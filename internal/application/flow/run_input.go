package flow

import (
	"context"
	"log/slog"
	"strings"
	"text/template"

	"github.com/yoke233/zhanggui/internal/core"
)

// BuildRunInputFromSnapshot constructs the run input sent to an agent.
func BuildRunInputFromSnapshot(snapshot string, action *core.Action, hasActionContext bool) string {
	var sb strings.Builder
	sb.WriteString("# Task\n\n")
	sb.WriteString(snapshot)

	if hasActionContext {
		sb.WriteString("\n\n# Reference Materials\n\n")
		sb.WriteString("> Full details (work item body, upstream outputs, feature manifest) are pre-loaded\n")
		sb.WriteString("> in `skills/action-context/`. Read the `SKILL.md` there for an index of\n")
		sb.WriteString("> available files. Read individual files on demand — do not load everything.\n")
	}

	if action != nil && len(action.AcceptanceCriteria) > 0 {
		sb.WriteString("\n\n# Acceptance Criteria\n\n")
		for _, c := range action.AcceptanceCriteria {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
	}

	appendCompletionProtocol(&sb, action)

	return sb.String()
}

// BuildRunInputForAction chooses between a full prompt and a follow-up prompt
// depending on session reuse state and prior gate feedback.
// The feedback parameter is pre-resolved by the caller (via ResolveLatestFeedback).
// When hasActionContext is true, a "Reference Materials" section is appended directing
// the agent to read pre-loaded files from skills/action-context/.
func BuildRunInputForAction(profile *core.AgentProfile, snapshot string, action *core.Action, hasPriorTurns bool, feedback string, reworkTmpl string, continueTmpl string, hasActionContext bool) string {
	// Gate actions must always receive the full prompt to keep output deterministic.
	if action != nil && action.Type == core.ActionGate {
		return BuildRunInputFromSnapshot(snapshot, action, hasActionContext)
	}

	if profile != nil && profile.Session.Reuse && hasPriorTurns {
		if feedback != "" {
			return renderFollowupRunMessage(reworkTmpl, followupVars{
				Feedback: feedback,
				StepName: actionName(action),
			})
		}
		return renderFollowupRunMessage(continueTmpl, followupVars{
			StepName: actionName(action),
		})
	}

	base := BuildRunInputFromSnapshot(snapshot, action, hasActionContext)
	if feedback == "" {
		return base
	}
	return base + "\n\n# Gate Feedback (Rework)\n\n" + feedback + "\n"
}

// ResolveLatestFeedback reads the latest feedback/instruction signal for an action.
// Signals are the single source of truth for action interaction history.
func ResolveLatestFeedback(ctx context.Context, store core.ActionSignalStore, action *core.Action) string {
	if store == nil || action == nil {
		return ""
	}
	sig, _ := store.GetLatestActionSignal(ctx, action.ID, core.SignalFeedback, core.SignalInstruction)
	if sig == nil {
		return ""
	}
	var sb strings.Builder
	if sig.Summary != "" {
		sb.WriteString(sig.Summary)
	}
	if sig.Content != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(sig.Content)
	}
	return strings.TrimSpace(sb.String())
}

type followupVars struct {
	Feedback string
	StepName string
}

func actionName(action *core.Action) string {
	if action == nil {
		return ""
	}
	return strings.TrimSpace(action.Name)
}

func renderFollowupRunMessage(tmplText string, vars followupVars) string {
	if strings.TrimSpace(tmplText) == "" {
		if strings.TrimSpace(vars.Feedback) == "" {
			if vars.StepName == "" {
				return "# Continue\n\n请继续完成当前任务（复用已有上下文）。\n"
			}
			return "# Continue\n\n请继续完成本 action（复用已有上下文）： " + vars.StepName + "\n"
		}
		if vars.StepName == "" {
			return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
		}
		return "# Rework Requested\n\n(action: " + vars.StepName + ")\n\n反馈：\n" + vars.Feedback + "\n"
	}

	tmpl, err := template.New("runtime-followup").Parse(tmplText)
	if err != nil {
		slog.Warn("runtime followup run message: invalid template", "error", err)
		return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
	}

	var b strings.Builder
	if err := tmpl.Execute(&b, vars); err != nil {
		slog.Warn("runtime followup run message: render failed", "error", err)
		return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
	}
	return out
}

func appendCompletionProtocol(sb *strings.Builder, action *core.Action) {
	if sb == nil || action == nil {
		return
	}
	switch action.Type {
	case core.ActionExec, core.ActionGate:
	default:
		return
	}

	decision := "complete"
	if action.Type == core.ActionGate {
		decision = "approve"
	}

	sb.WriteString("\n\n# Completion Protocol\n\n")
	sb.WriteString("完成验收后，立刻结束本次 action，不要继续扩展范围，也不要继续追加可选工作。\n")
	sb.WriteString("在最终回复结束前，必须使用已注入的 `action-signal` skill 发送终态；如果脚本不可用，至少输出一行：\n\n")
	sb.WriteString("```text\n")
	sb.WriteString(`AI_WORKFLOW_SIGNAL: {"decision":"`)
	sb.WriteString(decision)
	sb.WriteString(`","reason":"<简短原因>"}`)
	sb.WriteString("\n```\n\n")
	sb.WriteString("发出终态后，把最终回复控制在很短范围内，只保留必要结果，不要继续探索或追加建议。\n")
}
