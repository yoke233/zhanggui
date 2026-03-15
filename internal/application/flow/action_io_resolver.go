package flow

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ActionIOResolver fetches declared action inputs and materializes output resources.
type ActionIOResolver struct {
	store    ActionIOResolverStore
	registry SpaceProviderRegistry
}

// ActionIOResolverStore is the persistence port for action I/O resolution.
type ActionIOResolverStore interface {
	core.ActionIODeclStore
	core.ResourceSpaceStore
	core.ResourceStore
	core.WorkItemStore
}

// SpaceProviderRegistry dispatches Fetch/Deposit to the correct provider.
type SpaceProviderRegistry interface {
	Fetch(ctx context.Context, space *core.ResourceSpace, path string, destDir string) (string, error)
	Deposit(ctx context.Context, space *core.ResourceSpace, path string, localPath string) error
}

// NewActionIOResolver creates an ActionIOResolver.
func NewActionIOResolver(store ActionIOResolverStore, registry SpaceProviderRegistry) *ActionIOResolver {
	return &ActionIOResolver{store: store, registry: registry}
}

// ResourceResolver is kept as a compatibility alias while call sites transition.
type ResourceResolver = ActionIOResolver

// NewResourceResolver is kept as a compatibility constructor alias.
func NewResourceResolver(store ActionIOResolverStore, registry SpaceProviderRegistry) *ActionIOResolver {
	return NewActionIOResolver(store, registry)
}

func (r *ActionIOResolver) FetchInputs(ctx context.Context, actionID int64, destDir string) ([]*core.ResolvedResource, error) {
	inputs, err := r.store.ListActionIODeclsByDirection(ctx, actionID, core.IOInput)
	if err != nil {
		return nil, fmt.Errorf("list input io decls for action %d: %w", actionID, err)
	}
	if len(inputs) == 0 {
		return nil, nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create resource dest dir: %w", err)
	}

	resolved := make([]*core.ResolvedResource, 0, len(inputs))
	for _, decl := range inputs {
		item, err := r.fetchInputDecl(ctx, decl, destDir)
		if err != nil {
			if decl.Required {
				return nil, err
			}
			continue
		}
		if item != nil {
			resolved = append(resolved, item)
		}
	}
	return resolved, nil
}

func (r *ActionIOResolver) DepositOutputs(ctx context.Context, action *core.Action, run *core.Run, sourceDir string) error {
	if action == nil || run == nil {
		return fmt.Errorf("action and run are required")
	}
	outputs, err := r.store.ListActionIODeclsByDirection(ctx, action.ID, core.IOOutput)
	if err != nil {
		return fmt.Errorf("list output io decls for action %d: %w", action.ID, err)
	}
	if len(outputs) == 0 {
		return nil
	}

	projectID := int64(0)
	if workItem, err := r.store.GetWorkItem(ctx, action.WorkItemID); err == nil && workItem.ProjectID != nil {
		projectID = *workItem.ProjectID
	}

	for _, decl := range outputs {
		localPath, err := locateDeclaredOutput(sourceDir, decl.Path)
		if err != nil {
			if decl.Required {
				return fmt.Errorf("required output %d (%s): %w", decl.ID, decl.Path, err)
			}
			continue
		}

		if decl.SpaceID != nil {
			space, err := r.store.GetResourceSpace(ctx, *decl.SpaceID)
			if err != nil {
				if decl.Required {
					return fmt.Errorf("required output %d: load space %d: %w", decl.ID, *decl.SpaceID, err)
				}
				continue
			}
			if err := r.registry.Deposit(ctx, space, decl.Path, localPath); err != nil && decl.Required {
				return fmt.Errorf("required output %d (%s): deposit failed: %w", decl.ID, decl.Path, err)
			}
			if projectID == 0 {
				projectID = space.ProjectID
			}
		}

		resource := &core.Resource{
			ProjectID:   projectID,
			RunID:       &run.ID,
			StorageKind: "local",
			URI:         localPath,
			Role:        "output",
			FileName:    filepath.Base(localPath),
			MimeType:    decl.MediaType,
			Metadata: map[string]any{
				"action_id":   action.ID,
				"io_decl_id":  decl.ID,
				"declared_path": decl.Path,
			},
		}
		if _, err := r.store.CreateResource(ctx, resource); err != nil {
			return fmt.Errorf("create run output resource for action %d: %w", action.ID, err)
		}
	}
	return nil
}

func (r *ActionIOResolver) fetchInputDecl(ctx context.Context, decl *core.ActionIODecl, destDir string) (*core.ResolvedResource, error) {
	if decl == nil {
		return nil, nil
	}
	if decl.SpaceID != nil {
		space, err := r.store.GetResourceSpace(ctx, *decl.SpaceID)
		if err != nil {
			return nil, fmt.Errorf("required input %d: load space %d: %w", decl.ID, *decl.SpaceID, err)
		}
		localPath, err := r.registry.Fetch(ctx, space, decl.Path, destDir)
		if err != nil {
			return nil, fmt.Errorf("required input %d (%s): fetch failed: %w", decl.ID, decl.Path, err)
		}
		return &core.ResolvedResource{
			ActionResourceID: decl.ID,
			Direction:        core.ActionResourceDirection(decl.Direction),
			LocalPath:        localPath,
			RemoteURI:        strings.TrimRight(space.RootURI, "/") + "/" + strings.TrimLeft(decl.Path, "/"),
			MediaType:        decl.MediaType,
			Description:      decl.Description,
		}, nil
	}

	resource, err := r.store.GetResource(ctx, *decl.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("required input %d: load resource %d: %w", decl.ID, *decl.ResourceID, err)
	}
	localPath, err := materializeStoredResource(resource, destDir)
	if err != nil {
		return nil, fmt.Errorf("required input %d (%s): materialize failed: %w", decl.ID, resource.FileName, err)
	}
	return &core.ResolvedResource{
		ActionResourceID: decl.ID,
		Direction:        core.ActionResourceDirection(decl.Direction),
		LocalPath:        localPath,
		RemoteURI:        resource.URI,
		MediaType:        coalesceString(decl.MediaType, resource.MimeType),
		Description:      decl.Description,
	}, nil
}

func materializeStoredResource(resource *core.Resource, destDir string) (string, error) {
	if resource == nil {
		return "", fmt.Errorf("resource is nil")
	}
	if resource.StorageKind != "local" {
		return "", fmt.Errorf("storage kind %q is not yet supported for direct resource inputs", resource.StorageKind)
	}
	src, err := os.Open(resource.URI)
	if err != nil {
		return "", err
	}
	defer src.Close()

	destPath := filepath.Join(destDir, filepath.Base(coalesceString(resource.FileName, resource.URI)))
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", err
	}
	dst, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return destPath, dst.Close()
}

func locateDeclaredOutput(sourceDir, declaredPath string) (string, error) {
	localPath := filepath.Join(sourceDir, declaredPath)
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}
	localPath = filepath.Join(sourceDir, filepath.Base(declaredPath))
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}
	return "", fmt.Errorf("file not found in %s", sourceDir)
}

func coalesceString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func FormatInputResourceContext(resolved []*core.ResolvedResource) string {
	if len(resolved) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("The following input resources have been fetched to your workspace:\n\n")
	for _, r := range resolved {
		sb.WriteString("- **")
		sb.WriteString(filepath.Base(r.LocalPath))
		sb.WriteString("**")
		if r.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(r.Description)
		}
		sb.WriteString("\n  Local path: `")
		sb.WriteString(r.LocalPath)
		sb.WriteString("`\n")
		if r.MediaType != "" {
			sb.WriteString("  Type: ")
			sb.WriteString(r.MediaType)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
