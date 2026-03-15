package flow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	resourceprovider "github.com/yoke233/ai-workflow/internal/adapters/resource/provider"
	"github.com/yoke233/ai-workflow/internal/core"
)

type stubWorkspaceProvider struct {
	path string
}

func (p stubWorkspaceProvider) Prepare(context.Context, *core.Project, []*core.ResourceSpace, int64) (*core.Workspace, error) {
	return &core.Workspace{Path: p.path}, nil
}

func (p stubWorkspaceProvider) Release(context.Context, *core.Workspace) error { return nil }

func TestE2E_ActionIOResolverFetchesInputsAndDepositsOutputs(t *testing.T) {
	store, bus := setup(t)
	ctx := context.Background()

	projectID, err := store.CreateProject(ctx, &core.Project{Name: "io-project", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	workspaceRoot := t.TempDir()
	inputSpaceRoot := t.TempDir()
	outputSpaceRoot := t.TempDir()

	if err := os.WriteFile(filepath.Join(inputSpaceRoot, "spec.md"), []byte("# spec\nresource input"), 0o644); err != nil {
		t.Fatalf("write input space file: %v", err)
	}

	attachedPath := filepath.Join(t.TempDir(), "brief.txt")
	if err := os.WriteFile(attachedPath, []byte("direct resource input"), 0o644); err != nil {
		t.Fatalf("write direct resource file: %v", err)
	}

	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectID,
		Title:     "io work item",
		Status:    core.WorkItemOpen,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "implement",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}

	inputSpaceID, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   inputSpaceRoot,
		Role:      "input",
		Label:     "spec-space",
	})
	if err != nil {
		t.Fatalf("create input space: %v", err)
	}
	outputSpaceID, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   outputSpaceRoot,
		Role:      "output",
		Label:     "artifact-space",
	})
	if err != nil {
		t.Fatalf("create output space: %v", err)
	}

	inputResourceID, err := store.CreateResource(ctx, &core.Resource{
		ProjectID:   projectID,
		StorageKind: "local",
		URI:         attachedPath,
		Role:        "attachment",
		FileName:    "brief.txt",
		MimeType:    "text/plain",
	})
	if err != nil {
		t.Fatalf("create direct input resource: %v", err)
	}

	for _, decl := range []*core.ActionIODecl{
		{
			ActionID:    actionID,
			Direction:   core.IOInput,
			SpaceID:     &inputSpaceID,
			Path:        "spec.md",
			MediaType:   "text/markdown",
			Description: "spec from space",
			Required:    true,
		},
		{
			ActionID:    actionID,
			Direction:   core.IOInput,
			ResourceID:  &inputResourceID,
			Path:        "brief.txt",
			MediaType:   "text/plain",
			Description: "brief from resource",
			Required:    true,
		},
		{
			ActionID:    actionID,
			Direction:   core.IOOutput,
			SpaceID:     &outputSpaceID,
			Path:        "reports/result.txt",
			MediaType:   "text/plain",
			Description: "execution result",
			Required:    true,
		},
	} {
		if _, err := store.CreateActionIODecl(ctx, decl); err != nil {
			t.Fatalf("create action io decl: %v", err)
		}
	}

	executor := func(ctx context.Context, action *core.Action, run *core.Run) error {
		ws := WorkspaceFromContext(ctx)
		if ws == nil {
			t.Fatal("expected workspace in context")
		}
		resourcesDir := filepath.Join(ws.Path, ".resources")
		for _, name := range []string{"spec.md", "brief.txt"} {
			raw, err := os.ReadFile(filepath.Join(resourcesDir, name))
			if err != nil {
				t.Fatalf("read fetched resource %s: %v", name, err)
			}
			if len(strings.TrimSpace(string(raw))) == 0 {
				t.Fatalf("expected fetched resource %s to have content", name)
			}
		}

		outputPath := filepath.Join(ws.Path, "reports", "result.txt")
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			t.Fatalf("mkdir output: %v", err)
		}
		if err := os.WriteFile(outputPath, []byte("pipeline output"), 0o644); err != nil {
			t.Fatalf("write output: %v", err)
		}
		run.ResultMarkdown = "done"
		return nil
	}

	engine := New(store, bus, executor,
		WithConcurrency(1),
		WithWorkspaceProvider(stubWorkspaceProvider{path: workspaceRoot}),
		WithResourceResolver(NewActionIOResolver(store, resourceprovider.NewDefaultRegistry())),
	)

	if err := engine.Run(ctx, workItemID); err != nil {
		t.Fatalf("run engine: %v", err)
	}

	run, err := store.GetLatestRunWithResult(ctx, actionID)
	if err != nil {
		t.Fatalf("get latest run: %v", err)
	}
	if !strings.Contains(run.BriefingSnapshot, "spec.md") || !strings.Contains(run.BriefingSnapshot, "brief.txt") {
		t.Fatalf("expected briefing snapshot to include fetched resources, got %q", run.BriefingSnapshot)
	}

	depositedPath := filepath.Join(outputSpaceRoot, "reports", "result.txt")
	got, err := os.ReadFile(depositedPath)
	if err != nil {
		t.Fatalf("read deposited output: %v", err)
	}
	if strings.TrimSpace(string(got)) != "pipeline output" {
		t.Fatalf("unexpected deposited output: %q", string(got))
	}

	runResources, err := store.ListResourcesByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list run resources: %v", err)
	}
	if len(runResources) != 1 {
		t.Fatalf("expected 1 run resource, got %d", len(runResources))
	}
	if runResources[0].FileName != "result.txt" || runResources[0].Role != "output" {
		t.Fatalf("unexpected run resource: %+v", runResources[0])
	}
	if _, err := os.Stat(runResources[0].URI); err != nil {
		t.Fatalf("expected stored local output to exist at %s: %v", runResources[0].URI, err)
	}
}
