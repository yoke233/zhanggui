package api

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestDefaultPRTemplates(t *testing.T) {
	prompts := flowapp.DefaultPRFlowPrompts()
	if got := prompts.Global.ImplementObjective; !strings.Contains(got, "不要自行 git commit/push") {
		t.Fatalf("implement objective missing commit/push guard: %q", got)
	}
	if got := prompts.Global.ImplementObjective; !strings.Contains(got, "实际执行过的检查命令") {
		t.Fatalf("implement objective missing validation requirement: %q", got)
	}
	if got := prompts.Global.GateObjective; !strings.Contains(got, "merge 失败时") {
		t.Fatalf("gate objective missing merge retry guidance: %q", got)
	}
	if got := prompts.Global.GateObjective; !strings.Contains(got, "AI_WORKFLOW_GATE_JSON") {
		t.Fatalf("gate objective missing deterministic output requirement: %q", got)
	}
	if got := prompts.Global.MergeReworkFeedback; !strings.Contains(got, "{{.Hint}}") {
		t.Fatalf("merge rework prompt missing hint variable: %q", got)
	}
}

func TestDefaultPRCommitMessage(t *testing.T) {
	if got := defaultPRCommitMessage(42); got != "chore(pr-issue): apply issue 42 updates" {
		t.Fatalf("unexpected commit message: %q", got)
	}
}

func TestBootstrapPRDefaultTimeoutsAreSane(t *testing.T) {
	if 15*time.Minute <= 0 || 10*time.Minute <= 0 || 5*time.Minute <= 0 {
		t.Fatal("expected positive default timeouts")
	}
}

func TestCurrentPRFlowPrompts_UsesProviderOverrides(t *testing.T) {
	h := &Handler{
		prPrompts: func() flowapp.PRFlowPrompts {
			return flowapp.PRFlowPrompts{
				Global: flowapp.PRProviderPrompts{
					ImplementObjective:  "impl",
					GateObjective:       "gate",
					MergeReworkFeedback: "merge",
				},
			}
		},
	}
	got := h.currentPRFlowPrompts()
	if got.Global.ImplementObjective != "impl" || got.Global.GateObjective != "gate" || got.Global.MergeReworkFeedback != "merge" {
		t.Fatalf("unexpected prompts: %+v", got)
	}
}

func TestBindingDefaultBranch_PrefersBaseBranch(t *testing.T) {
	binding := &core.ResourceBinding{
		Kind: "git",
		Config: map[string]any{
			"default_branch": "main",
			"base_branch":    "release/1.0",
		},
	}
	if got := bindingDefaultBranch(binding); got != "release/1.0" {
		t.Fatalf("bindingDefaultBranch = %q, want %q", got, "release/1.0")
	}
}

func TestBindingProvider_DetectsCodeup(t *testing.T) {
	binding := &core.ResourceBinding{Kind: "git"}
	if got := bindingProvider(binding, "codeup.aliyun.com"); got != "codeup" {
		t.Fatalf("bindingProvider = %q, want codeup", got)
	}
}

func TestBindingSCMFlowEnabled_RequiresExplicitOptIn(t *testing.T) {
	binding := &core.ResourceBinding{
		Kind: "git",
		Config: map[string]any{
			"provider": "github",
		},
	}
	if bindingSCMFlowEnabled(binding) {
		t.Fatal("expected binding without enable_scm_flow to be disabled")
	}
	binding.Config["enable_scm_flow"] = true
	if !bindingSCMFlowEnabled(binding) {
		t.Fatal("expected binding with enable_scm_flow to be enabled")
	}
}

func TestResolveEnabledSCMRepoFromBindings_GitHub(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme/demo.git")

	info, ok := resolveEnabledSCMRepoFromBindings(context.Background(), []*core.ResourceBinding{{
		Kind: "git",
		URI:  dir,
		Config: map[string]any{
			"default_branch":  "main",
			"enable_scm_flow": true,
		},
	}})
	if !ok {
		t.Fatal("expected binding to resolve")
	}
	if info.Provider != "github" {
		t.Fatalf("provider = %q", info.Provider)
	}
	if info.DefaultBranch != "main" {
		t.Fatalf("default branch = %q", info.DefaultBranch)
	}
}

func TestResolveEnabledSCMRepoFromBindings_RequiresEnabledFlow(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme/demo.git")

	if _, ok := resolveEnabledSCMRepoFromBindings(context.Background(), []*core.ResourceBinding{{
		Kind: "git",
		URI:  dir,
		Config: map[string]any{
			"default_branch": "main",
		},
	}}); ok {
		t.Fatal("expected binding resolution to skip disabled scm flow binding")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if out, err := cmd.Output(); err != nil {
		t.Fatalf("git %v failed: %v stderr=%s stdout=%s", args, err, stderr.String(), string(out))
	}
}
