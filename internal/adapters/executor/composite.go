package executor

import (
	"context"
	"fmt"
	"strings"

	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

// CompositeStepExecutorConfig wires builtin steps with an ACP fallback executor.
type CompositeStepExecutorConfig struct {
	Store core.Store
	Bus   core.EventBus

	SCMTokens flowapp.SCMTokens

	// UpgradeFunc is called by the self_upgrade builtin to trigger a restart
	// with a newly built binary. If nil, self_upgrade is disabled.
	UpgradeFunc UpgradeFunc

	ACPExecutor flowapp.ActionExecutor
}

// NewCompositeActionExecutor returns a ActionExecutor that routes certain exec steps to builtin
// implementations (git commit/push, open PR), and falls back to ACP for everything else.
//
// Builtin routing is controlled by step.Config["builtin"].
func NewCompositeActionExecutor(cfg CompositeStepExecutorConfig) flowapp.ActionExecutor {
	return func(ctx context.Context, step *core.Action, exec *core.Run) error {
		if step == nil {
			return fmt.Errorf("step is nil")
		}

		builtin := ""
		if step.Config != nil {
			if v, ok := step.Config["builtin"].(string); ok {
				builtin = strings.TrimSpace(v)
			}
		}

		switch builtin {
		case "":
			// fallthrough to ACP
		case "git_commit_push":
			return runBuiltinGitCommitPush(ctx, cfg.Store, cfg.Bus, cfg.SCMTokens, step, exec)
		case "scm_open_pr", "github_open_pr":
			return runBuiltinSCMOpenPR(ctx, cfg.Store, cfg.Bus, cfg.SCMTokens, step, exec)
		case "self_upgrade":
			return runBuiltinSelfUpgrade(ctx, cfg.Store, cfg.Bus, step, exec, cfg.UpgradeFunc)
		default:
			return fmt.Errorf("unknown builtin executor: %s", builtin)
		}

		if cfg.ACPExecutor == nil {
			return fmt.Errorf("ACP executor is not configured")
		}
		return cfg.ACPExecutor(ctx, step, exec)
	}
}
