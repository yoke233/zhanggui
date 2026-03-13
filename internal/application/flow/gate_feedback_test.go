package flow

import (
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestBuildRunInputForAction_ReusedSessionUsesFollowupTemplates(t *testing.T) {
	profile := &core.AgentProfile{
		ID: "worker",
		Session: core.ProfileSession{
			Reuse: true,
		},
	}
	action := &core.Action{
		Name: "implement",
		Type: core.ActionExec,
	}

	reworkTmpl := "REWORK {{.StepName}}: {{.Feedback}}"
	continueTmpl := "CONTINUE {{.StepName}}"

	// With feedback (pre-resolved), should use rework template.
	feedback := "please add unit tests"
	out := BuildRunInputForAction(profile, "ignored", action, true, feedback, reworkTmpl, continueTmpl, false)
	if !strings.Contains(out, "REWORK implement") {
		t.Fatalf("expected rework followup template, got: %q", out)
	}
	if !strings.Contains(out, "please add unit tests") {
		t.Fatalf("expected feedback to appear in followup, got: %q", out)
	}

	// Without feedback, should use continue template (no base snapshot re-send).
	out2 := BuildRunInputForAction(profile, "BASE-SNAPSHOT", action, true, "", reworkTmpl, continueTmpl, false)
	if strings.Contains(out2, "BASE-SNAPSHOT") {
		t.Fatalf("expected not to include base snapshot when reusing session, got: %q", out2)
	}
	if !strings.Contains(out2, "CONTINUE implement") {
		t.Fatalf("expected continue template, got: %q", out2)
	}
}

func TestBuildRunInputForAction_GateAlwaysFullPrompt(t *testing.T) {
	action := &core.Action{
		Name: "review_merge_gate",
		Type: core.ActionGate,
		AcceptanceCriteria: []string{
			"must output AI_WORKFLOW_GATE_JSON",
		},
	}
	profile := &core.AgentProfile{Session: core.ProfileSession{Reuse: true}}
	out := BuildRunInputForAction(profile, "SNAP", action, true, "some-feedback", "REWORK", "CONTINUE", false)
	if !strings.Contains(out, "SNAP") {
		t.Fatalf("expected full run input to include snapshot, got: %q", out)
	}
	if !strings.Contains(out, "Acceptance Criteria") {
		t.Fatalf("expected full run input to include acceptance criteria, got: %q", out)
	}
}

func TestFormatMergeFailureFeedback_GitHubConflict(t *testing.T) {
	reason, metadata := (&WorkItemEngine{}).formatMergeFailureFeedback(&core.Action{ID: 1, Name: "review_merge_gate"}, &MergeError{
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
	eng := &WorkItemEngine{
		prPrompts: func() PRFlowPrompts {
			return PRFlowPrompts{
				Global: PRProviderPrompts{
					MergeReworkFeedback: "PR={{.PRNumber}} STATE={{.MergeableState}} HINT={{.Hint}}",
				},
			}
		},
	}
	reason, _ := eng.formatMergeFailureFeedback(&core.Action{ID: 1}, &MergeError{
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
