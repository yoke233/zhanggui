package executor

import (
	"context"
	"fmt"
	"strings"

	flowapp "github.com/yoke233/zhanggui/internal/application/flow"
	"github.com/yoke233/zhanggui/internal/core"
)

// CompositeStepExecutorConfig wires builtin actions with an ACP fallback executor.
type CompositeStepExecutorConfig struct {
	Store core.Store
	Bus   core.EventBus

	SCMTokens flowapp.SCMTokens

	// UpgradeFunc is called by the self_upgrade builtin to trigger a restart
	// with a newly built binary. If nil, self_upgrade is disabled.
	UpgradeFunc UpgradeFunc

	ACPExecutor flowapp.ActionExecutor
}

// NewCompositeActionExecutor returns an ActionExecutor that routes certain exec actions to builtin
// implementations (git commit/push, open PR), and falls back to ACP for everything else.
//
// Builtin routing is controlled by action.Config["builtin"].
func NewCompositeActionExecutor(cfg CompositeStepExecutorConfig) flowapp.ActionExecutor {
	return func(ctx context.Context, action *core.Action, run *core.Run) error {
		if action == nil {
			return fmt.Errorf("action is nil")
		}

		builtin := ""
		if action.Config != nil {
			if v, ok := action.Config["builtin"].(string); ok {
				builtin = strings.TrimSpace(v)
			}
		}

		switch builtin {
		case "":
			// fallthrough to ACP
		case "git_commit_push":
			return runBuiltinGitCommitPush(ctx, cfg.Store, cfg.Bus, cfg.SCMTokens, action, run)
		case "scm_open_pr", "github_open_pr":
			return runBuiltinSCMOpenPR(ctx, cfg.Store, cfg.Bus, cfg.SCMTokens, action, run)
		case "self_upgrade":
			return runBuiltinSelfUpgrade(ctx, cfg.Store, cfg.Bus, action, run, cfg.UpgradeFunc)
		default:
			return fmt.Errorf("unknown builtin executor: %s", builtin)
		}

		if cfg.ACPExecutor == nil {
			return fmt.Errorf("ACP executor is not configured")
		}
		return cfg.ACPExecutor(ctx, action, run)
	}
}
