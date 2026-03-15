package sqlite

import (
	"context"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestResourceSpaceCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	projectID, err := s.CreateProject(ctx, &core.Project{Name: "proj", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	space := &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      "git",
		RootURI:   "https://github.com/org/repo.git",
		Role:      "primary_repo",
		Label:     "主仓库",
		Config:    map[string]any{"branch": "main"},
	}
	id, err := s.CreateResourceSpace(ctx, space)
	if err != nil {
		t.Fatalf("create resource space: %v", err)
	}

	got, err := s.GetResourceSpace(ctx, id)
	if err != nil {
		t.Fatalf("get resource space: %v", err)
	}
	if got.RootURI != space.RootURI || got.Config["branch"] != "main" {
		t.Fatalf("unexpected resource space: %+v", got)
	}

	space.RootURI = "https://github.com/org/repo2.git"
	space.Role = "reference"
	if err := s.UpdateResourceSpace(ctx, space); err != nil {
		t.Fatalf("update resource space: %v", err)
	}

	list, err := s.ListResourceSpaces(ctx, projectID)
	if err != nil {
		t.Fatalf("list resource spaces: %v", err)
	}
	if len(list) != 1 || list[0].RootURI != "https://github.com/org/repo2.git" {
		t.Fatalf("unexpected resource spaces: %+v", list)
	}

	if err := s.DeleteResourceSpace(ctx, id); err != nil {
		t.Fatalf("delete resource space: %v", err)
	}
	if _, err := s.GetResourceSpace(ctx, id); err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestResourceCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	projectID, err := s.CreateProject(ctx, &core.Project{Name: "proj", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workItemID, err := s.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectID,
		Title:     "item",
		Status:    core.WorkItemOpen,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}

	resource := &core.Resource{
		ProjectID:   projectID,
		WorkItemID:  &workItemID,
		StorageKind: "local",
		URI:         "/tmp/spec.md",
		Role:        "input",
		FileName:    "spec.md",
		MimeType:    "text/markdown",
		SizeBytes:   128,
		Metadata:    map[string]any{"source": "upload"},
	}
	id, err := s.CreateResource(ctx, resource)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	got, err := s.GetResource(ctx, id)
	if err != nil {
		t.Fatalf("get resource: %v", err)
	}
	if got.FileName != "spec.md" || got.Metadata["source"] != "upload" {
		t.Fatalf("unexpected resource: %+v", got)
	}

	list, err := s.ListResourcesByWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("list resources by work item: %v", err)
	}
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("unexpected resources: %+v", list)
	}

	if err := s.DeleteResource(ctx, id); err != nil {
		t.Fatalf("delete resource: %v", err)
	}
	if _, err := s.GetResource(ctx, id); err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateResourceRejectsMultipleOwners(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	projectID, err := s.CreateProject(ctx, &core.Project{Name: "proj", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workItemID := int64(1)
	runID := int64(2)
	resource := &core.Resource{
		ProjectID:   projectID,
		WorkItemID:  &workItemID,
		RunID:       &runID,
		StorageKind: "local",
		URI:         "/tmp/file.txt",
		FileName:    "file.txt",
	}
	if _, err := s.CreateResource(ctx, resource); err == nil {
		t.Fatal("expected create resource to fail")
	}
}

func TestActionIODeclCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	projectID, err := s.CreateProject(ctx, &core.Project{Name: "proj", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workItemID, err := s.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectID,
		Title:     "item",
		Status:    core.WorkItemOpen,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := s.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "write",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	spaceID, err := s.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      "local_fs",
		RootURI:   "/data/project",
		Role:      "shared_drive",
	})
	if err != nil {
		t.Fatalf("create resource space: %v", err)
	}

	decl := &core.ActionIODecl{
		ActionID:    actionID,
		Direction:   core.IOInput,
		SpaceID:     &spaceID,
		Path:        "docs/spec.md",
		MediaType:   "text/markdown",
		Description: "需求规格",
		Required:    true,
	}
	id, err := s.CreateActionIODecl(ctx, decl)
	if err != nil {
		t.Fatalf("create action io decl: %v", err)
	}

	got, err := s.GetActionIODecl(ctx, id)
	if err != nil {
		t.Fatalf("get action io decl: %v", err)
	}
	if got.Path != "docs/spec.md" || got.Direction != core.IOInput {
		t.Fatalf("unexpected decl: %+v", got)
	}

	list, err := s.ListActionIODeclsByDirection(ctx, actionID, core.IOInput)
	if err != nil {
		t.Fatalf("list action io decls: %v", err)
	}
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("unexpected decl list: %+v", list)
	}

	if err := s.DeleteActionIODecl(ctx, id); err != nil {
		t.Fatalf("delete action io decl: %v", err)
	}
	if _, err := s.GetActionIODecl(ctx, id); err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateActionIODeclRejectsAmbiguousReference(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	decl := &core.ActionIODecl{
		ActionID:  1,
		Direction: core.IOInput,
	}
	if _, err := s.CreateActionIODecl(ctx, decl); err == nil {
		t.Fatal("expected create action io decl to fail")
	}
}
