package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/yoke233/zhanggui/internal/application/orchestrateapp"
	"github.com/yoke233/zhanggui/internal/core"
)

type fakeOrchestrateService struct {
	createTaskInput     orchestrateapp.CreateTaskInput
	followUpTaskInput   orchestrateapp.FollowUpTaskInput
	reassignTaskInput   orchestrateapp.ReassignTaskInput
	decomposeTaskInput  orchestrateapp.DecomposeTaskInput
	escalateThreadInput orchestrateapp.EscalateThreadInput
}

func (f *fakeOrchestrateService) CreateTask(_ context.Context, input orchestrateapp.CreateTaskInput) (*orchestrateapp.CreateTaskResult, error) {
	f.createTaskInput = input
	return &orchestrateapp.CreateTaskResult{
		WorkItem: &core.WorkItem{ID: 12, Title: input.Title},
		Created:  true,
	}, nil
}

func (f *fakeOrchestrateService) FollowUpTask(_ context.Context, input orchestrateapp.FollowUpTaskInput) (*orchestrateapp.FollowUpTaskResult, error) {
	f.followUpTaskInput = input
	return &orchestrateapp.FollowUpTaskResult{
		WorkItemID:          input.WorkItemID,
		Status:              core.WorkItemBlocked,
		Blocked:             true,
		AssignedProfile:     "lead",
		RecommendedNextStep: "reassign_or_escalate",
		LatestRunSummary:    "waiting on integration fix",
	}, nil
}

func (f *fakeOrchestrateService) ReassignTask(_ context.Context, input orchestrateapp.ReassignTaskInput) (*orchestrateapp.ReassignTaskResult, error) {
	f.reassignTaskInput = input
	return &orchestrateapp.ReassignTaskResult{
		WorkItemID: input.WorkItemID,
		OldProfile: "lead",
		NewProfile: input.NewProfile,
	}, nil
}

func (f *fakeOrchestrateService) DecomposeTask(_ context.Context, input orchestrateapp.DecomposeTaskInput) (*orchestrateapp.DecomposeTaskResult, error) {
	f.decomposeTaskInput = input
	return &orchestrateapp.DecomposeTaskResult{
		WorkItemID:  input.WorkItemID,
		ActionCount: 3,
	}, nil
}

func (f *fakeOrchestrateService) EscalateThread(_ context.Context, input orchestrateapp.EscalateThreadInput) (*orchestrateapp.EscalateThreadResult, error) {
	f.escalateThreadInput = input
	return &orchestrateapp.EscalateThreadResult{
		WorkItemID: input.WorkItemID,
		Thread:     &core.Thread{ID: 44, Title: input.ThreadTitle, Status: core.ThreadActive},
		Created:    true,
	}, nil
}

func TestParseOrchestrateArgsTaskCreate(t *testing.T) {
	t.Parallel()

	opts, err := parseOrchestrateArgs([]string{
		"task", "create",
		"--title", "CEO bootstrap",
		"--project-id", "12",
		"--dedupe-key", "chat:42:goal:bootstrap",
		"--json",
	})
	if err != nil {
		t.Fatalf("parseOrchestrateArgs() error = %v", err)
	}
	if opts.Action != "task.create" {
		t.Fatalf("Action = %q, want task.create", opts.Action)
	}
	if opts.Title != "CEO bootstrap" {
		t.Fatalf("Title = %q, want CEO bootstrap", opts.Title)
	}
	if opts.ProjectID == nil || *opts.ProjectID != 12 {
		t.Fatalf("ProjectID = %v, want 12", opts.ProjectID)
	}
	if opts.DedupeKey != "chat:42:goal:bootstrap" {
		t.Fatalf("DedupeKey = %q, want chat:42:goal:bootstrap", opts.DedupeKey)
	}
	if !opts.JSON {
		t.Fatal("expected JSON to be true")
	}
}

func TestParseOrchestrateArgsRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	_, err := parseOrchestrateArgs([]string{"task", "create", "--nope"})
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestRunOrchestrateToWriterEmitsCreateJSON(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	service := &fakeOrchestrateService{}
	err := runOrchestrateToWriter(&out, service, []string{
		"task", "create",
		"--title", "CEO bootstrap",
		"--project-id", "12",
		"--dedupe-key", "chat:42:goal:bootstrap",
		"--json",
	})
	if err != nil {
		t.Fatalf("runOrchestrateToWriter() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("ok = %v, want true", payload["ok"])
	}
	if payload["action"] != "task.create" {
		t.Fatalf("action = %v, want task.create", payload["action"])
	}
	if payload["created"] != true {
		t.Fatalf("created = %v, want true", payload["created"])
	}
	if payload["work_item_id"] != float64(12) {
		t.Fatalf("work_item_id = %v, want 12", payload["work_item_id"])
	}
	if service.createTaskInput.DedupeKey != "chat:42:goal:bootstrap" {
		t.Fatalf("service.createTaskInput.DedupeKey = %q, want chat:42:goal:bootstrap", service.createTaskInput.DedupeKey)
	}
}

func TestRunOrchestrateToWriterEmitsFollowUpJSON(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	service := &fakeOrchestrateService{}
	err := runOrchestrateToWriter(&out, service, []string{
		"task", "follow-up",
		"--work-item-id", "21",
		"--json",
	})
	if err != nil {
		t.Fatalf("runOrchestrateToWriter() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["action"] != "task.follow-up" {
		t.Fatalf("action = %v, want task.follow-up", payload["action"])
	}
	if payload["work_item_id"] != float64(21) {
		t.Fatalf("work_item_id = %v, want 21", payload["work_item_id"])
	}
	if payload["assigned_profile"] != "lead" {
		t.Fatalf("assigned_profile = %v, want lead", payload["assigned_profile"])
	}
}

func TestRunOrchestrateTaskCreateThenFollowUp(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)

	configToml := []byte("[server]\nauth_required = false\n")
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), configToml, 0o644); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	var createOut bytes.Buffer
	if err := RunOrchestrateWithWriters(&createOut, []string{
		"task", "create",
		"--title", "CEO smoke",
		"--dedupe-key", "chat:smoke",
		"--json",
	}); err != nil {
		t.Fatalf("RunOrchestrateWithWriters(create) error = %v", err)
	}
	createResp := decodeOrchestrateJSON(t, createOut.Bytes())
	workItemID := int64(createResp["work_item_id"].(float64))

	var followOut bytes.Buffer
	if err := RunOrchestrateWithWriters(&followOut, []string{
		"task", "follow-up",
		"--work-item-id", strconv.FormatInt(workItemID, 10),
		"--json",
	}); err != nil {
		t.Fatalf("RunOrchestrateWithWriters(follow-up) error = %v", err)
	}
	followResp := decodeOrchestrateJSON(t, followOut.Bytes())
	if followResp["work_item_id"] != float64(workItemID) {
		t.Fatalf("unexpected follow-up response: %+v", followResp)
	}
}

func RunOrchestrateWithWriters(out io.Writer, args []string) error {
	runtime, err := defaultNewOrchestrateRuntime()
	if err != nil {
		return err
	}
	if runtime != nil && runtime.close != nil {
		defer runtime.close()
	}
	return runOrchestrateToWriter(out, runtime.service, args)
}

func decodeOrchestrateJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw=%s)", err, string(raw))
	}
	return payload
}
