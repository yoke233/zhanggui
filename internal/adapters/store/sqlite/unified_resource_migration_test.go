package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func reopenTestStore(t *testing.T, dbPath string) *Store {
	t.Helper()
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store %s: %v", dbPath, err)
	}
	return s
}

func TestUnifiedResourceMigrationMigratesLegacyModelsOnRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "migration.db")

	seed := reopenTestStore(t, dbPath)

	projectID, err := seed.CreateProject(ctx, &core.Project{Name: "legacy-project", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workItemID, err := seed.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectID,
		Title:     "legacy-item",
		Status:    core.WorkItemOpen,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := seed.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "legacy-action",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	runID, err := seed.CreateRun(ctx, &core.Run{
		ActionID:   actionID,
		WorkItemID: workItemID,
		Status:     core.RunSucceeded,
		Attempt:    1,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	spaceBindingID, err := seed.CreateResourceBinding(ctx, &core.ResourceBinding{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		URI:       filepath.Join(t.TempDir(), "legacy-space"),
		Label:     "workspace",
		Config:    map[string]any{"branch": "main"},
	})
	if err != nil {
		t.Fatalf("create legacy space binding: %v", err)
	}
	attachmentBindingID, err := seed.CreateResourceBinding(ctx, &core.ResourceBinding{
		ProjectID: projectID,
		IssueID:   &workItemID,
		Kind:      core.ResourceKindAttachment,
		URI:       filepath.Join(t.TempDir(), "legacy-spec.md"),
		Label:     "legacy-spec.md",
		Config: map[string]any{
			"mime_type": "text/markdown",
			"size":      42,
		},
	})
	if err != nil {
		t.Fatalf("create legacy attachment binding: %v", err)
	}

	for _, ar := range []*core.ActionResource{
		{
			ActionID:          actionID,
			ResourceBindingID: spaceBindingID,
			Direction:         core.ResourceInput,
			Path:              "docs/spec.md",
			MediaType:         "text/markdown",
			Description:       "space input",
			Required:          true,
		},
		{
			ActionID:          actionID,
			ResourceBindingID: attachmentBindingID,
			Direction:         core.ResourceInput,
			Path:              "legacy-spec.md",
			MediaType:         "text/markdown",
			Description:       "attachment input",
			Required:          true,
		},
	} {
		if _, err := seed.CreateActionResource(ctx, ar); err != nil {
			t.Fatalf("create legacy action resource: %v", err)
		}
	}

	run, err := seed.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if err := seed.orm.WithContext(ctx).Model(&RunModel{}).Where("id = ?", run.ID).Update("result_assets", JSONField[[]core.Asset]{Data: []core.Asset{{
		Name:      "result.txt",
		URI:       filepath.Join(t.TempDir(), "result.txt"),
		MediaType: "text/plain",
	}}}).Error; err != nil {
		t.Fatalf("seed legacy result_assets: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	migrated := reopenTestStore(t, dbPath)
	defer func() { _ = migrated.Close() }()

	space, err := migrated.GetResourceSpace(ctx, spaceBindingID)
	if err != nil {
		t.Fatalf("get migrated space: %v", err)
	}
	if space.RootURI == "" || space.Config["legacy_binding_id"] == nil {
		t.Fatalf("unexpected migrated space: %+v", space)
	}

	workItemResources, err := migrated.ListResourcesByWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("list migrated work item resources: %v", err)
	}
	if len(workItemResources) != 1 {
		t.Fatalf("expected 1 migrated work item resource, got %d", len(workItemResources))
	}
	if workItemResources[0].FileName != "legacy-spec.md" || workItemResources[0].Metadata["legacy_source"] != "resource_bindings_attachment" {
		t.Fatalf("unexpected migrated attachment resource: %+v", workItemResources[0])
	}

	runResources, err := migrated.ListResourcesByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list migrated run resources: %v", err)
	}
	if len(runResources) != 1 {
		t.Fatalf("expected 1 migrated run resource, got %d", len(runResources))
	}
	if runResources[0].FileName != "result.txt" || runResources[0].Metadata["legacy_source"] != "executions.result_assets" {
		t.Fatalf("unexpected migrated run resource: %+v", runResources[0])
	}

	decls, err := migrated.ListActionIODecls(ctx, actionID)
	if err != nil {
		t.Fatalf("list migrated action io decls: %v", err)
	}
	if len(decls) != 2 {
		t.Fatalf("expected 2 migrated action io decls, got %d", len(decls))
	}

	var foundSpaceDecl, foundResourceDecl bool
	for _, decl := range decls {
		if decl.SpaceID != nil && *decl.SpaceID == spaceBindingID {
			foundSpaceDecl = true
		}
		if decl.ResourceID != nil && *decl.ResourceID == workItemResources[0].ID {
			foundResourceDecl = true
		}
	}
	if !foundSpaceDecl || !foundResourceDecl {
		t.Fatalf("expected both space/resource migrated decls, got %+v", decls)
	}
}

func TestUnifiedResourceMigrationIsIdempotentAcrossRestarts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "migration-idempotent.db")

	seed := reopenTestStore(t, dbPath)
	projectID, err := seed.CreateProject(ctx, &core.Project{Name: "legacy-project", Kind: core.ProjectDev})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workItemID, err := seed.CreateWorkItem(ctx, &core.WorkItem{ProjectID: &projectID, Title: "legacy-item", Status: core.WorkItemOpen})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := seed.CreateAction(ctx, &core.Action{WorkItemID: workItemID, Name: "legacy-action", Type: core.ActionExec, Status: core.ActionPending})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	runID, err := seed.CreateRun(ctx, &core.Run{ActionID: actionID, WorkItemID: workItemID, Status: core.RunSucceeded, Attempt: 1})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	bindingID, err := seed.CreateResourceBinding(ctx, &core.ResourceBinding{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		URI:       filepath.Join(t.TempDir(), "legacy-space"),
		Label:     "workspace",
	})
	if err != nil {
		t.Fatalf("create resource binding: %v", err)
	}
	if _, err := seed.CreateActionResource(ctx, &core.ActionResource{
		ActionID:          actionID,
		ResourceBindingID: bindingID,
		Direction:         core.ResourceOutput,
		Path:              "out/report.txt",
		Required:          true,
	}); err != nil {
		t.Fatalf("create action resource: %v", err)
	}

	run, err := seed.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if err := seed.orm.WithContext(ctx).Model(&RunModel{}).Where("id = ?", run.ID).Update("result_assets", JSONField[[]core.Asset]{Data: []core.Asset{{Name: "result.txt", URI: filepath.Join(t.TempDir(), "result.txt"), MediaType: "text/plain"}}}).Error; err != nil {
		t.Fatalf("seed legacy result_assets: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	first := reopenTestStore(t, dbPath)
	var spaceCount1, declCount1, resourceCount1 int64
	if err := first.orm.Model(&ResourceSpaceModel{}).Count(&spaceCount1).Error; err != nil {
		t.Fatalf("count spaces after first restart: %v", err)
	}
	if err := first.orm.Model(&ActionIODeclModel{}).Count(&declCount1).Error; err != nil {
		t.Fatalf("count decls after first restart: %v", err)
	}
	if err := first.orm.Model(&ResourceModel{}).Count(&resourceCount1).Error; err != nil {
		t.Fatalf("count resources after first restart: %v", err)
	}
	_ = first.Close()

	second := reopenTestStore(t, dbPath)
	defer func() { _ = second.Close() }()

	var spaceCount2, declCount2, resourceCount2 int64
	if err := second.orm.Model(&ResourceSpaceModel{}).Count(&spaceCount2).Error; err != nil {
		t.Fatalf("count spaces after second restart: %v", err)
	}
	if err := second.orm.Model(&ActionIODeclModel{}).Count(&declCount2).Error; err != nil {
		t.Fatalf("count decls after second restart: %v", err)
	}
	if err := second.orm.Model(&ResourceModel{}).Count(&resourceCount2).Error; err != nil {
		t.Fatalf("count resources after second restart: %v", err)
	}

	if spaceCount1 != 1 || declCount1 != 1 || resourceCount1 != 1 {
		t.Fatalf("unexpected first restart counts: spaces=%d decls=%d resources=%d", spaceCount1, declCount1, resourceCount1)
	}
	if spaceCount2 != spaceCount1 || declCount2 != declCount1 || resourceCount2 != resourceCount1 {
		t.Fatalf("migration duplicated data across restarts: first=(%d,%d,%d) second=(%d,%d,%d)", spaceCount1, declCount1, resourceCount1, spaceCount2, declCount2, resourceCount2)
	}
}
