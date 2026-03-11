package flow

import (
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestRecordGateRework_AppendsHistoryAndLastFeedback(t *testing.T) {
	step := &core.Step{
		ID:         10,
		Name:       "implement",
		RetryCount: 0,
		Config:     map[string]any{},
	}
	recordGateRework(step, 99, "tests failing", map[string]any{"pr_number": 12, "pr_url": "https://example/pr/12"})

	if step.Config["last_gate_feedback"] == nil {
		t.Fatalf("expected last_gate_feedback to be set")
	}
	if _, ok := step.Config["rework_history"]; !ok {
		t.Fatalf("expected rework_history to be set")
	}
	arr, ok := step.Config["rework_history"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("expected rework_history length 1, got %T len=%d", step.Config["rework_history"], len(arr))
	}
	last, ok := step.Config["last_gate_feedback"].(map[string]any)
	if !ok {
		t.Fatalf("expected last_gate_feedback to be map, got %T", step.Config["last_gate_feedback"])
	}
	if got, _ := last["reason"].(string); strings.TrimSpace(got) != "tests failing" {
		t.Fatalf("reason=%q, want %q", got, "tests failing")
	}
}

func TestBuildExecutionInputForStep_ReusedSessionUsesFollowupTemplates(t *testing.T) {
	step := &core.Step{
		Name:   "implement",
		Type:   core.StepExec,
		Config: map[string]any{},
	}
	recordGateRework(step, 1, "please add unit tests", map[string]any(nil))

	profile := &core.AgentProfile{
		ID: "worker",
		Session: core.ProfileSession{
			Reuse: true,
		},
	}

	reworkTmpl := "REWORK {{.StepName}}: {{.Feedback}}"
	continueTmpl := "CONTINUE {{.StepName}}"

	out := BuildExecutionInputForStep(profile, "ignored", step, true, reworkTmpl, continueTmpl)
	if !strings.Contains(out, "REWORK implement") {
		t.Fatalf("expected rework followup template, got: %q", out)
	}
	if !strings.Contains(out, "please add unit tests") {
		t.Fatalf("expected feedback to appear in followup, got: %q", out)
	}

	// If no feedback, should use continue template (and not re-send base snapshot).
	step.Config["last_gate_feedback"] = nil
	step.Config["rework_history"] = []any{}
	out2 := BuildExecutionInputForStep(profile, "BASE-SNAPSHOT", step, true, reworkTmpl, continueTmpl)
	if strings.Contains(out2, "BASE-SNAPSHOT") {
		t.Fatalf("expected not to include base snapshot when reusing session, got: %q", out2)
	}
	if !strings.Contains(out2, "CONTINUE implement") {
		t.Fatalf("expected continue template, got: %q", out2)
	}
}

func TestBuildExecutionInputForStep_GateAlwaysFullPrompt(t *testing.T) {
	step := &core.Step{
		Name: "review_merge_gate",
		Type: core.StepGate,
		AcceptanceCriteria: []string{
			"must output AI_WORKFLOW_GATE_JSON",
		},
	}
	profile := &core.AgentProfile{Session: core.ProfileSession{Reuse: true}}
	out := BuildExecutionInputForStep(profile, "SNAP", step, true, "REWORK", "CONTINUE")
	if !strings.Contains(out, "SNAP") {
		t.Fatalf("expected full execution input to include snapshot, got: %q", out)
	}
	if !strings.Contains(out, "Acceptance Criteria") {
		t.Fatalf("expected full execution input to include acceptance criteria, got: %q", out)
	}
}

func TestFormatMergeFailureFeedback_GitHubConflict(t *testing.T) {
	reason, metadata := (&FlowEngine{}).formatMergeFailureFeedback(&core.Step{ID: 1, Name: "review_merge_gate"}, &MergeError{
		Provider:       "github",
		Number:         12,
		URL:            "https://github.com/acme/repo/pull/12",
		Message:        "PUT https://api.github.com/...: 405 Pull Request is not mergeable []",
		MergeableState: "dirty",
	})

	if !strings.Contains(reason, "PR #12") {
		t.Fatalf("expected PR number in reason, got: %q", reason)
	}
	if !strings.Contains(reason, "git fetch origin") {
		t.Fatalf("expected actionable hint in reason, got: %q", reason)
	}
	if got, _ := metadata["mergeable_state"].(string); got != "dirty" {
		t.Fatalf("expected mergeable_state=dirty, got %#v", metadata["mergeable_state"])
	}
	if !strings.Contains(metadata["merge_action_hint"].(string), "git diff origin/main...HEAD") {
		t.Fatalf("expected merge action hint, got %#v", metadata["merge_action_hint"])
	}
}

func TestFormatMergeFailureFeedback_UsesConfiguredTemplate(t *testing.T) {
	eng := &FlowEngine{
		prPrompts: func() PRFlowPrompts {
			return PRFlowPrompts{
				Global: PRProviderPrompts{
					MergeReworkFeedback: "PR={{.PRNumber}} STATE={{.MergeableState}} HINT={{.Hint}}",
				},
			}
		},
	}
	reason, _ := eng.formatMergeFailureFeedback(&core.Step{ID: 1}, &MergeError{
		Provider:       "github",
		Number:         7,
		MergeableState: "dirty",
		Message:        "conflict",
	})
	if !strings.Contains(reason, "PR=7") || !strings.Contains(reason, "STATE=dirty") {
		t.Fatalf("expected configured template output, got %q", reason)
	}
}

func TestPRFlowPrompts_ProviderCodeUpUsesOverride(t *testing.T) {
	prompts := MergePRFlowPrompts(PRFlowPrompts{
		CodeUp: PRProviderPrompts{
			MergeReworkFeedback: "CODEUP {{.PRNumber}}",
			MergeStates: PRMergeStatePrompts{
				Dirty: "codeup dirty",
			},
		},
	})
	got := prompts.Provider("codeup")
	if got.MergeReworkFeedback != "CODEUP {{.PRNumber}}" {
		t.Fatalf("unexpected codeup feedback template: %q", got.MergeReworkFeedback)
	}
	if got.MergeStates.Dirty != "codeup dirty" {
		t.Fatalf("unexpected codeup dirty state: %q", got.MergeStates.Dirty)
	}
}

func TestPRFlowPrompts_ProviderDefaultsStayGeneric(t *testing.T) {
	prompts := DefaultPRFlowPrompts()
	for _, provider := range []string{"github", "codeup"} {
		got := prompts.Provider(provider)
		if strings.Contains(got.MergeReworkFeedback, "GitHub") || strings.Contains(got.MergeReworkFeedback, "Codeup") {
			t.Fatalf("expected generic feedback for %s, got %q", provider, got.MergeReworkFeedback)
		}
		if strings.Contains(got.MergeStates.Dirty, "GitHub") || strings.Contains(got.MergeStates.Dirty, "Codeup") {
			t.Fatalf("expected generic dirty hint for %s, got %q", provider, got.MergeStates.Dirty)
		}
	}
}
