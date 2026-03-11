package flow

import (
	"fmt"
	"log/slog"
	"strings"
	"text/template"

	"github.com/yoke233/ai-workflow/internal/core"
)

// BuildExecutionInputFromBriefing constructs the execution input sent to an agent.
func BuildExecutionInputFromBriefing(snapshot string, step *core.Step) string {
	var sb strings.Builder
	sb.WriteString("# Task\n\n")
	sb.WriteString(snapshot)

	if step != nil && len(step.AcceptanceCriteria) > 0 {
		sb.WriteString("\n\n# Acceptance Criteria\n\n")
		for _, c := range step.AcceptanceCriteria {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// BuildExecutionInputForStep chooses between a full prompt and a follow-up prompt
// depending on session reuse state and prior gate feedback.
func BuildExecutionInputForStep(profile *core.AgentProfile, snapshot string, step *core.Step, hasPriorTurns bool, reworkTmpl string, continueTmpl string) string {
	// Gate steps must always receive the full prompt to keep output deterministic.
	if step != nil && step.Type == core.StepGate {
		return BuildExecutionInputFromBriefing(snapshot, step)
	}

	feedback := latestGateFeedback(step)
	if profile != nil && profile.Session.Reuse && hasPriorTurns {
		if feedback != "" {
			return renderFollowupExecutionMessage(reworkTmpl, followupVars{
				Feedback: feedback,
				StepName: stepName(step),
			})
		}
		return renderFollowupExecutionMessage(continueTmpl, followupVars{
			StepName: stepName(step),
		})
	}

	base := BuildExecutionInputFromBriefing(snapshot, step)
	if feedback == "" {
		return base
	}
	return base + "\n\n# Gate Feedback (Rework)\n\n" + feedback + "\n"
}

func latestGateFeedback(step *core.Step) string {
	if step == nil || step.Config == nil {
		return ""
	}

	last, _ := step.Config["last_gate_feedback"].(map[string]any)
	if last == nil {
		if arr, ok := step.Config["rework_history"].([]any); ok && len(arr) > 0 {
			if m, ok := arr[len(arr)-1].(map[string]any); ok {
				last = m
			}
		}
	}
	if last == nil {
		return ""
	}

	reason, _ := last["reason"].(string)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Reason: ")
	sb.WriteString(reason)

	if prURL, ok := last["pr_url"].(string); ok && strings.TrimSpace(prURL) != "" {
		sb.WriteString("\nPR: ")
		sb.WriteString(strings.TrimSpace(prURL))
	}
	if n, ok := last["pr_number"]; ok {
		sb.WriteString("\nPR Number: ")
		sb.WriteString(fmt.Sprint(n))
	}
	return sb.String()
}

type followupVars struct {
	Feedback string
	StepName string
}

func stepName(step *core.Step) string {
	if step == nil {
		return ""
	}
	return strings.TrimSpace(step.Name)
}

func renderFollowupExecutionMessage(tmplText string, vars followupVars) string {
	if strings.TrimSpace(tmplText) == "" {
		if strings.TrimSpace(vars.Feedback) == "" {
			if vars.StepName == "" {
				return "# Continue\n\n请继续完成当前任务（复用已有上下文）。\n"
			}
			return "# Continue\n\n请继续完成本 step（复用已有上下文）： " + vars.StepName + "\n"
		}
		if vars.StepName == "" {
			return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
		}
		return "# Rework Requested\n\n(step: " + vars.StepName + ")\n\n反馈：\n" + vars.Feedback + "\n"
	}

	tmpl, err := template.New("runtime-followup").Parse(tmplText)
	if err != nil {
		slog.Warn("runtime followup execution message: invalid template", "error", err)
		return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
	}

	var b strings.Builder
	if err := tmpl.Execute(&b, vars); err != nil {
		slog.Warn("runtime followup execution message: render failed", "error", err)
		return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
	}
	return out
}
