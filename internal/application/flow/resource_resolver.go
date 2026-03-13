package flow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ResourceResolver fetches action input resources and deposits output resources.
// It bridges the ActionResource declarations with the ResourceProvider registry.
type ResourceResolver struct {
	store    ResourceResolverStore
	registry ResourceProviderRegistry
}

// ResourceResolverStore is the persistence port for resource resolution.
type ResourceResolverStore interface {
	core.ActionResourceStore
	core.ResourceLocatorStore
}

// ResourceProviderRegistry dispatches Fetch/Deposit to the correct provider.
type ResourceProviderRegistry interface {
	Fetch(ctx context.Context, locator *core.ResourceLocator, path string, destDir string) (string, error)
	Deposit(ctx context.Context, locator *core.ResourceLocator, path string, localPath string) error
}

// NewResourceResolver creates a ResourceResolver.
func NewResourceResolver(store ResourceResolverStore, registry ResourceProviderRegistry) *ResourceResolver {
	return &ResourceResolver{store: store, registry: registry}
}

// FetchInputs retrieves all input resources declared for the given action.
// Resources are downloaded to destDir. Returns resolved resource metadata
// for injection into the agent's input context.
func (r *ResourceResolver) FetchInputs(ctx context.Context, actionID int64, destDir string) ([]*core.ResolvedResource, error) {
	inputs, err := r.store.ListActionResourcesByDirection(ctx, actionID, core.ResourceInput)
	if err != nil {
		return nil, fmt.Errorf("list input resources for action %d: %w", actionID, err)
	}
	if len(inputs) == 0 {
		return nil, nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create resource dest dir: %w", err)
	}

	var resolved []*core.ResolvedResource
	for _, ar := range inputs {
		locator, err := r.store.GetResourceLocator(ctx, ar.LocatorID)
		if err != nil {
			if ar.Required {
				return nil, fmt.Errorf("required input resource %d: locator %d not found: %w", ar.ID, ar.LocatorID, err)
			}
			continue
		}

		localPath, err := r.registry.Fetch(ctx, locator, ar.Path, destDir)
		if err != nil {
			if ar.Required {
				return nil, fmt.Errorf("required input resource %d (%s): fetch failed: %w", ar.ID, ar.Path, err)
			}
			continue
		}

		resolved = append(resolved, &core.ResolvedResource{
			ActionResourceID: ar.ID,
			Direction:        core.ResourceInput,
			LocalPath:        localPath,
			RemoteURI:        locator.BaseURI + "/" + ar.Path,
			MediaType:        ar.MediaType,
			Description:      ar.Description,
		})
	}
	return resolved, nil
}

// DepositOutputs uploads all output resources declared for the given action.
// It looks for files matching the declared paths in sourceDir.
func (r *ResourceResolver) DepositOutputs(ctx context.Context, actionID int64, sourceDir string) error {
	outputs, err := r.store.ListActionResourcesByDirection(ctx, actionID, core.ResourceOutput)
	if err != nil {
		return fmt.Errorf("list output resources for action %d: %w", actionID, err)
	}
	if len(outputs) == 0 {
		return nil
	}

	for _, ar := range outputs {
		locator, err := r.store.GetResourceLocator(ctx, ar.LocatorID)
		if err != nil {
			if ar.Required {
				return fmt.Errorf("required output resource %d: locator %d not found: %w", ar.ID, ar.LocatorID, err)
			}
			continue
		}

		// Look for the file in the source directory.
		localPath := filepath.Join(sourceDir, ar.Path)
		if _, err := os.Stat(localPath); err != nil {
			// Also try just the basename.
			localPath = filepath.Join(sourceDir, filepath.Base(ar.Path))
			if _, err := os.Stat(localPath); err != nil {
				if ar.Required {
					return fmt.Errorf("required output resource %d (%s): file not found in %s", ar.ID, ar.Path, sourceDir)
				}
				continue
			}
		}

		if err := r.registry.Deposit(ctx, locator, ar.Path, localPath); err != nil {
			if ar.Required {
				return fmt.Errorf("required output resource %d (%s): deposit failed: %w", ar.ID, ar.Path, err)
			}
		}
	}
	return nil
}

// FormatInputResourceContext renders resolved input resources as a context block
// for inclusion in the agent's input/briefing.
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
