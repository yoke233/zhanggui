package github

import (
	"context"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func TestRunTrigger_LabelMapping_SelectsTemplate(t *testing.T) {
	store := newRunTriggerTestStore(t)
	defer store.Close()
	projectID := seedRunTriggerProject(t, store)

	createCalls := 0
	trigger := NewRunTrigger(store, func(projectID, name, description, template string) (*core.Run, error) {
		createCalls++
		return &core.Run{
			ID:              "pipe-trigger-1",
			ProjectID:       projectID,
			Name:            name,
			Description:     description,
			Template:        template,
			Status:          core.StatusQueued,
			Stages:          []core.StageConfig{},
			Artifacts:       map[string]string{},
			Config:          map[string]any{},
			MaxTotalRetries: 5,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}, nil
	})

	Run, err := trigger.TriggerFromIssue(context.Background(), IssueTriggerInput{
		ProjectID:            projectID,
		IssueNumber:          201,
		IssueTitle:           "issue trigger",
		IssueBody:            "from label mapping",
		Labels:               []string{"type:feature"},
		LabelTemplateMapping: map[string]string{"type:feature": "feature"},
	})
	if err != nil {
		t.Fatalf("TriggerFromIssue() error = %v", err)
	}
	if Run.Template != "feature" {
		t.Fatalf("expected template feature, got %q", Run.Template)
	}
	if createCalls != 1 {
		t.Fatalf("expected create called once, got %d", createCalls)
	}
}

func TestRunTrigger_Idempotent_NoDuplicateRunForSameIssue(t *testing.T) {
	store := newRunTriggerTestStore(t)
	defer store.Close()
	projectID := seedRunTriggerProject(t, store)

	existing := &core.Run{
		ID:              "pipe-existing",
		ProjectID:       projectID,
		Name:            "existing",
		Description:     "existing",
		Template:        "standard",
		Status:          core.StatusQueued,
		Stages:          []core.StageConfig{},
		Artifacts:       map[string]string{"issue_number": "202"},
		Config:          map[string]any{"issue_number": 202},
		MaxTotalRetries: 5,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := store.SaveRun(existing); err != nil {
		t.Fatalf("SaveRun(existing) error = %v", err)
	}

	createCalls := 0
	trigger := NewRunTrigger(store, func(projectID, name, description, template string) (*core.Run, error) {
		createCalls++
		return &core.Run{
			ID:              "pipe-new",
			ProjectID:       projectID,
			Name:            name,
			Description:     description,
			Template:        template,
			Status:          core.StatusQueued,
			Stages:          []core.StageConfig{},
			Artifacts:       map[string]string{},
			Config:          map[string]any{},
			MaxTotalRetries: 5,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}, nil
	})

	Run, err := trigger.TriggerFromIssue(context.Background(), IssueTriggerInput{
		ProjectID:   projectID,
		IssueNumber: 202,
		IssueTitle:  "same issue",
	})
	if err != nil {
		t.Fatalf("TriggerFromIssue() error = %v", err)
	}
	if Run.ID != "pipe-existing" {
		t.Fatalf("expected existing Run, got %q", Run.ID)
	}
	if createCalls != 0 {
		t.Fatalf("expected create not called, got %d", createCalls)
	}
}

func TestRunTrigger_CommandRun_UsesExplicitTemplate(t *testing.T) {
	store := newRunTriggerTestStore(t)
	defer store.Close()
	projectID := seedRunTriggerProject(t, store)

	trigger := NewRunTrigger(store, func(projectID, name, description, template string) (*core.Run, error) {
		return &core.Run{
			ID:              "pipe-command-1",
			ProjectID:       projectID,
			Name:            name,
			Description:     description,
			Template:        template,
			Status:          core.StatusQueued,
			Stages:          []core.StageConfig{},
			Artifacts:       map[string]string{},
			Config:          map[string]any{},
			MaxTotalRetries: 5,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}, nil
	})

	Run, err := trigger.TriggerFromCommand(context.Background(), CommandTriggerInput{
		ProjectID:   projectID,
		IssueNumber: 203,
		Template:    "hotfix",
		Message:     "/run hotfix",
	})
	if err != nil {
		t.Fatalf("TriggerFromCommand() error = %v", err)
	}
	if Run.Template != "hotfix" {
		t.Fatalf("expected template hotfix, got %q", Run.Template)
	}
}

func newRunTriggerTestStore(t *testing.T) *storesqlite.SQLiteStore {
	t.Helper()
	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	return store
}

func seedRunTriggerProject(t *testing.T, store core.Store) string {
	t.Helper()
	project := &core.Project{
		ID:       "proj-Run-trigger",
		Name:     "proj-Run-trigger",
		RepoPath: t.TempDir(),
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	return project.ID
}
