package github

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	"github.com/yoke233/ai-workflow/internal/eventbus"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func TestE2E_GitHub_ScenarioA_IssueOpened_RunCreate_StatusSync(t *testing.T) {
	store := newGitHubE2EStore(t)
	defer store.Close()
	projectID := seedGitHubE2EProject(t, store)

	payload := readGitHubFixture(t, "issues_opened.json")
	issue := parseE2EIssuePayload(t, payload)

	trigger := NewRunTrigger(store, func(projectID, name, description, template string) (*core.Run, error) {
		now := time.Now()
		return &core.Run{
			ID:              "pipe-e2e-a",
			ProjectID:       projectID,
			Name:            name,
			Description:     description,
			Template:        template,
			Status:          core.StatusCreated,
			Stages:          []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
			Artifacts:       map[string]string{},
			Config:          map[string]any{},
			MaxTotalRetries: 5,
			CreatedAt:       now,
			UpdatedAt:       now,
		}, nil
	})

	Run, err := trigger.TriggerFromIssue(context.Background(), IssueTriggerInput{
		ProjectID:   projectID,
		IssueNumber: issue.Issue.Number,
		IssueTitle:  issue.Issue.Title,
		IssueBody:   issue.Issue.Body,
		Labels:      issue.LabelNames(),
	})
	if err != nil {
		t.Fatalf("TriggerFromIssue() error = %v", err)
	}
	if Run == nil {
		t.Fatal("expected Run created from issue")
	}

	labelClient := &fakeRunIssueSyncClient{}
	syncer := NewRunStatusSyncer(labelClient)
	if err := syncer.SyncRunEvent(context.Background(), core.Event{
		Type: core.EventRunDone,
		Data: map[string]string{
			"issue_number": "201",
		},
	}); err != nil {
		t.Fatalf("SyncRunEvent() error = %v", err)
	}

	if len(labelClient.updatedLabels) == 0 {
		t.Fatal("expected status sync label update")
	}
	last := labelClient.updatedLabels[len(labelClient.updatedLabels)-1]
	if len(last.labels) == 0 || last.labels[0] != "status: run_done" {
		t.Fatalf("expected done status label, got %#v", last.labels)
	}
}

func TestE2E_GitHub_ScenarioB_SlashReject_ApplyRunAction(t *testing.T) {
	store := newGitHubE2EStore(t)
	defer store.Close()
	projectID := seedGitHubE2EProject(t, store)

	Run := seedGitHubE2ERun(t, store, projectID, "pipe-e2e-b", map[string]any{
		"issue_number": 201,
	})
	Run.Status = core.StatusWaitingReview
	Run.CurrentStage = core.StageImplement
	Run.Stages = []core.StageConfig{
		{Name: core.StageImplement, Agent: "codex"},
		{Name: core.StageCodeReview, Agent: "claude"},
	}
	if err := store.SaveRun(Run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	now := time.Now()
	if err := store.SaveCheckpoint(&core.Checkpoint{
		RunID:      Run.ID,
		StageName:  core.StageImplement,
		Status:     core.CheckpointSuccess,
		StartedAt:  now,
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("SaveCheckpoint(implement) error = %v", err)
	}
	if err := store.SaveCheckpoint(&core.Checkpoint{
		RunID:      Run.ID,
		StageName:  core.StageCodeReview,
		Status:     core.CheckpointSuccess,
		StartedAt:  now,
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("SaveCheckpoint(code_review) error = %v", err)
	}

	payload := readGitHubFixture(t, "issue_comment_created.json")
	comment := parseE2ECommentPayload(t, payload)

	command, ok, err := ParseSlashCommand(comment.Comment.Body)
	if err != nil {
		t.Fatalf("ParseSlashCommand() error = %v", err)
	}
	if !ok {
		t.Fatal("expected slash command parsed")
	}
	if !IsSlashCommandAllowed(comment.Sender.Login, comment.Comment.AuthorAssociation, command.Type, SlashACLConfig{}) {
		t.Fatal("expected slash command ACL allow")
	}

	bus := eventbus.New()
	defer bus.Close()
	executor := engine.NewExecutor(store, bus, map[string]core.AgentPlugin{}, nil, nil)
	if err := executor.ApplyAction(context.Background(), core.RunAction{
		RunID:   Run.ID,
		Type:    core.ActionReject,
		Stage:   command.Stage,
		Message: command.Reason,
	}); err != nil {
		t.Fatalf("ApplyAction(reject) error = %v", err)
	}

	after, err := store.GetRun(Run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if after.Status != core.StatusWaitingReview {
		t.Fatalf("expected waiting_review after slash reject, got %s", after.Status)
	}
	if after.ErrorMessage == "" {
		t.Fatal("expected reject reason persisted")
	}
}

func TestE2E_GitHub_ScenarioC_ImplementComplete_DraftPR_MergedRunDone(t *testing.T) {
	store := newGitHubE2EStore(t)
	defer store.Close()
	projectID := seedGitHubE2EProject(t, store)

	Run := seedGitHubE2ERun(t, store, projectID, "pipe-e2e-c", map[string]any{
		"base_branch": "main",
	})
	Run.Status = core.StatusRunning
	Run.BranchName = "ai-flow/pipe-e2e-c"
	if err := store.SaveRun(Run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	scm := &fakePRLifecycleSCM{
		createPRURL: "https://github.com/acme/ai-workflow/pull/301",
	}
	lifecycle := NewPRLifecycle(store, scm)

	if _, err := lifecycle.OnImplementComplete(context.Background(), Run.ID); err != nil {
		t.Fatalf("OnImplementComplete() error = %v", err)
	}
	if err := lifecycle.OnPullRequestClosed(context.Background(), projectID, 301, true); err != nil {
		t.Fatalf("OnPullRequestClosed() error = %v", err)
	}

	updated, err := store.GetRun(Run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if updated.Status != core.StatusDone {
		t.Fatalf("expected done after merged webhook, got %s", updated.Status)
	}
}

func TestE2E_GitHub_ScenarioD_OutageDegradeRecover(t *testing.T) {
	base := &fakeRunIssueSyncClientWithError{
		updateErr: &net.OpError{Op: "dial", Err: &net.DNSError{IsTimeout: true}},
	}
	resilient := NewResilientClient(base)

	if err := resilient.UpdateIssueLabels(context.Background(), 301, []string{"status: run_active:implement"}); err != nil {
		t.Fatalf("UpdateIssueLabels() error = %v", err)
	}
	if !resilient.IsDegraded() {
		t.Fatal("expected degraded mode after outage")
	}

	publisher := &fakeReconnectPublisher{}
	syncer := &fakeRunEventSyncer{}
	reconnect := NewReconnectSync(publisher, syncer)
	reconnect.MarkDegraded(errors.New("dial tcp timeout"))

	events := []core.Event{
		{
			Type:      core.EventStageStart,
			Timestamp: time.Now().Add(-2 * time.Minute),
			Data:      map[string]string{"issue_number": "301"},
		},
		{
			Type:      core.EventRunDone,
			Timestamp: time.Now().Add(-1 * time.Minute),
			Data:      map[string]string{"issue_number": "301"},
		},
	}
	if err := reconnect.OnRecovered(context.Background(), events); err != nil {
		t.Fatalf("OnRecovered() error = %v", err)
	}

	if len(publisher.events) != 1 || publisher.events[0].Type != core.EventGitHubReconnected {
		t.Fatalf("expected github_reconnected event, got %#v", publisher.events)
	}
	if len(syncer.events) != 1 || syncer.events[0].Type != core.EventRunDone {
		t.Fatalf("expected latest Run state replay, got %#v", syncer.events)
	}
}

type e2eIssuePayload struct {
	Issue struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	} `json:"issue"`
}

func (p e2eIssuePayload) LabelNames() []string {
	if len(p.Issue.Labels) == 0 {
		return nil
	}
	labels := make([]string, 0, len(p.Issue.Labels))
	for _, label := range p.Issue.Labels {
		labels = append(labels, label.Name)
	}
	return labels
}

type e2eCommentPayload struct {
	Comment struct {
		Body              string `json:"body"`
		AuthorAssociation string `json:"author_association"`
	} `json:"comment"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

func parseE2EIssuePayload(t *testing.T, payload []byte) e2eIssuePayload {
	t.Helper()
	var body e2eIssuePayload
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("unmarshal issue payload: %v", err)
	}
	return body
}

func parseE2ECommentPayload(t *testing.T, payload []byte) e2eCommentPayload {
	t.Helper()
	var body e2eCommentPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("unmarshal comment payload: %v", err)
	}
	return body
}

func readGitHubFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return content
}

func newGitHubE2EStore(t *testing.T) *storesqlite.SQLiteStore {
	t.Helper()
	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	return store
}

func seedGitHubE2EProject(t *testing.T, store core.Store) string {
	t.Helper()
	project := &core.Project{
		ID:          "proj-github-e2e",
		Name:        "proj-github-e2e",
		RepoPath:    t.TempDir(),
		GitHubOwner: "acme",
		GitHubRepo:  "ai-workflow",
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	return project.ID
}

func seedGitHubE2ERun(t *testing.T, store core.Store, projectID, RunID string, config map[string]any) *core.Run {
	t.Helper()
	if config == nil {
		config = map[string]any{}
	}
	Run := &core.Run{
		ID:              RunID,
		ProjectID:       projectID,
		Name:            RunID,
		Description:     "github e2e Run",
		Template:        "standard",
		Status:          core.StatusCreated,
		CurrentStage:    core.StageImplement,
		Stages:          []core.StageConfig{{Name: core.StageImplement, Agent: "codex"}},
		Artifacts:       map[string]string{},
		Config:          config,
		MaxTotalRetries: 5,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := store.SaveRun(Run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}
	return Run
}
