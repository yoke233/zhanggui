package core

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestNewChatSessionID(t *testing.T) {
	id := NewChatSessionID()
	pat := regexp.MustCompile(`^chat-\d{8}-[0-9a-f]{8}$`)
	if !pat.MatchString(id) {
		t.Fatalf("invalid chat session id: %s", id)
	}
}

func TestNewTaskPlanID(t *testing.T) {
	id := NewTaskPlanID()
	pat := regexp.MustCompile(`^plan-\d{8}-[0-9a-f]{8}$`)
	if !pat.MatchString(id) {
		t.Fatalf("invalid task plan id: %s", id)
	}
}

func TestNewTaskItemID(t *testing.T) {
	id := NewTaskItemID("plan-20260301-a3f1b2c0", 1)
	if id != "task-a3f1b2c0-1" {
		t.Fatalf("unexpected task item id: %s", id)
	}
}

func TestTaskItemValidate(t *testing.T) {
	err := (TaskItem{Description: "   "}).Validate()
	if err == nil {
		t.Fatal("expected validation error for empty description")
	}
}

func TestTaskItemValidate_RequiresAcceptanceWhenStructuredEnabled(t *testing.T) {
	base := TaskItem{
		ID:          "task-a3f1b2c0-1",
		PlanID:      "plan-20260301-a3f1b2c0",
		Title:       "实现 OAuth 登录",
		Description: "实现 OAuth 登录接口并补齐测试",
	}

	if err := base.Validate(); err != nil {
		t.Fatalf("default validate should not require structured contract: %v", err)
	}

	err := base.Validate(true)
	if err == nil {
		t.Fatal("expected structured validation error when acceptance is missing")
	}
	if !strings.Contains(err.Error(), "acceptance") {
		t.Fatalf("unexpected error for missing acceptance: %v", err)
	}

	base.Acceptance = []string{"OAuth callback succeeds with valid state"}
	if err := base.Validate(true); err != nil {
		t.Fatalf("structured validation should pass when acceptance exists: %v", err)
	}
}

func TestNewTaskItemID_StableWithPlanPrefix(t *testing.T) {
	planID := "plan-20260301-a3f1b2c0"
	id1 := NewTaskItemID(planID, 2)
	id2 := NewTaskItemID(planID, 2)
	if id1 != id2 {
		t.Fatalf("task item id should be stable, got %q and %q", id1, id2)
	}
	if id1 != "task-a3f1b2c0-2" {
		t.Fatalf("unexpected task item id with plan prefix: %s", id1)
	}
}

func TestTaskPlanJSON_RoundTrip_WithContractFields(t *testing.T) {
	plan := TaskPlan{
		ID:               "plan-20260301-a3f1b2c0",
		ProjectID:        "proj-1",
		Name:             "oauth rollout",
		Status:           PlanDraft,
		WaitReason:       WaitNone,
		FailPolicy:       FailBlock,
		SpecProfile:      "default",
		ContractVersion:  "v1",
		ContractChecksum: "sha256:abcdef",
		Tasks: []TaskItem{
			{
				ID:          "task-a3f1b2c0-1",
				PlanID:      "plan-20260301-a3f1b2c0",
				Title:       "接口开发",
				Description: "开发接口并补齐测试",
				Inputs:      []string{"OAuth provider config"},
				Outputs:     []string{"OAuth login API"},
				Acceptance:  []string{"OAuth callback with valid state returns 200"},
				Constraints: []string{"Must keep backward compatible response"},
				Template:    "standard",
				Status:      ItemPending,
			},
		},
	}

	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}

	var got TaskPlan
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}

	if got.SpecProfile != plan.SpecProfile || got.ContractVersion != plan.ContractVersion || got.ContractChecksum != plan.ContractChecksum {
		t.Fatalf("contract metadata mismatch after round-trip: got %+v", got)
	}
	if len(got.Tasks) != 1 {
		t.Fatalf("expected one task after round-trip, got %d", len(got.Tasks))
	}
	if len(got.Tasks[0].Inputs) != 1 || len(got.Tasks[0].Outputs) != 1 || len(got.Tasks[0].Acceptance) != 1 || len(got.Tasks[0].Constraints) != 1 {
		t.Fatalf("structured contract fields missing after round-trip: %+v", got.Tasks[0])
	}
}

func TestTaskPlan_HasPendingFileContents(t *testing.T) {
	plan := TaskPlan{
		FileContents: map[string]string{
			"internal/core/taskplan.go": "package core",
		},
		Tasks: nil,
	}
	if !plan.HasPendingFileContents() {
		t.Fatal("expected HasPendingFileContents to be true when file contents exist and tasks are empty")
	}
}

func TestTaskPlan_HasPendingFileContents_FalseWhenTasksExist(t *testing.T) {
	plan := TaskPlan{
		FileContents: map[string]string{
			"internal/core/taskplan.go": "package core",
		},
		Tasks: []TaskItem{
			{ID: "task-a3f1b2c0-1", Description: "has task"},
		},
	}
	if plan.HasPendingFileContents() {
		t.Fatal("expected HasPendingFileContents to be false when tasks exist")
	}
}

func TestTaskPlanJSON_RoundTrip_WithWaitParseFailed(t *testing.T) {
	plan := TaskPlan{
		ID:         "plan-20260302-a1b2c3d4",
		ProjectID:  "proj-parse",
		Name:       "parse-failed-plan",
		Status:     PlanWaitingHuman,
		WaitReason: WaitParseFailed,
		SourceFiles: []string{
			"internal/core/taskplan.go",
		},
		FileContents: map[string]string{
			"internal/core/taskplan.go": "package core",
		},
	}

	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}

	var got TaskPlan
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}

	if got.WaitReason != WaitParseFailed {
		t.Fatalf("wait reason mismatch after round-trip: got=%q want=%q", got.WaitReason, WaitParseFailed)
	}
	if len(got.SourceFiles) != 1 || got.SourceFiles[0] != "internal/core/taskplan.go" {
		t.Fatalf("source_files mismatch after round-trip: %#v", got.SourceFiles)
	}
	if got.FileContents["internal/core/taskplan.go"] != "package core" {
		t.Fatalf("file_contents mismatch after round-trip: %#v", got.FileContents)
	}
}
