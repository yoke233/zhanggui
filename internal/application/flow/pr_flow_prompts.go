package flow

import "strings"

// PRFlowPrompts groups configurable prompt text used by the built-in PR flow.
type PRFlowPrompts struct {
	Global PRProviderPrompts
	GitHub PRProviderPrompts
	CodeUp PRProviderPrompts
	GitLab PRProviderPrompts
}

type PRProviderPrompts struct {
	ImplementObjective  string
	GateObjective       string
	MergeReworkFeedback string
	MergeStates         PRMergeStatePrompts
}

type PRMergeStatePrompts struct {
	Default  string
	Dirty    string
	Blocked  string
	Behind   string
	Unstable string
	Draft    string
}

// PRFlowPromptsProvider returns the latest prompt set for PR automation flows.
type PRFlowPromptsProvider func() PRFlowPrompts

func DefaultPRFlowPrompts() PRFlowPrompts {
	return PRFlowPrompts{
		Global: PRProviderPrompts{
			ImplementObjective:  "进行实现并在当前 worktree 中完成必要验证。必须直接修改代码/README，并执行与改动相关的实际检查（不要只做 smoke）。不要自行 git commit/push；后续步骤会处理。若 gate 或 merge 打回，请继续在当前分支修复，并在最终回复中明确列出你实际执行过的检查命令、结果，以及是否还存在阻塞。",
			GateObjective:       "你是代码审查员。你会收到上游 implement / commit / PR action 的输出。请综合 worktree 当前状态与上游输出评审本次提交是否可合并。至少检查：1) 改动是否满足目标；2) implement 输出里是否明确给出实际执行过的验证命令与结果；3) 当前分支是否存在明显冲突或阻塞。若不通过，必须给出可执行的返工原因；若通过，系统会自动尝试 merge，merge 失败时你的反馈会回流给上游继续修复。最后必须输出一行：AI_WORKFLOW_GATE_JSON: {\"verdict\":\"pass|reject\",\"reason\":\"...\"}",
			MergeReworkFeedback: "自动合并失败。{{if .PRNumber}}PR #{{.PRNumber}}{{if .PRURL}}（{{.PRURL}}）{{end}} 当前未能合并。{{end}}{{if .MergeableState}} mergeable_state={{.MergeableState}}。{{end}}\n{{.Hint}}\n不要新开 PR；继续在当前分支修复，并保留实际执行过的检查命令与结果。",
			MergeStates: PRMergeStatePrompts{
				Default:  "请在当前 worktree 中同步 base 分支变化，检查与 origin/main 的差异并修复冲突后重新提交。",
				Dirty:    "PR 当前存在冲突。请在当前 worktree 中执行 `git fetch origin`，检查 `git diff origin/main...HEAD`，修复与 base 分支的冲突后重新提交。",
				Blocked:  "PR 当前被阻塞。请检查仓库分支保护、必需状态检查或 review 条件，并在实现输出中说明阻塞原因。",
				Behind:   "PR 分支落后于 base。请同步 `origin/main` 的最新变化，并验证修改仍然正确后重新提交。",
				Unstable: "PR 当前状态不允许合并。请检查远端 PR 状态、必要检查与分支策略，并在输出里说明处理结果。",
				Draft:    "PR 当前仍是 draft。请检查是否需要 ready for review，并确认远端状态允许合并。",
			},
		},
		GitHub: PRProviderPrompts{},
		CodeUp: PRProviderPrompts{},
		GitLab: PRProviderPrompts{},
	}
}

func MergePRFlowPrompts(override PRFlowPrompts) PRFlowPrompts {
	base := DefaultPRFlowPrompts()
	base.Global = mergePRProviderPrompts(base.Global, override.Global)
	base.GitHub = mergePRProviderPrompts(base.GitHub, override.GitHub)
	base.CodeUp = mergePRProviderPrompts(base.CodeUp, override.CodeUp)
	base.GitLab = mergePRProviderPrompts(base.GitLab, override.GitLab)
	return base
}

func mergePRProviderPrompts(base PRProviderPrompts, override PRProviderPrompts) PRProviderPrompts {
	if override.ImplementObjective != "" {
		base.ImplementObjective = override.ImplementObjective
	}
	if override.GateObjective != "" {
		base.GateObjective = override.GateObjective
	}
	if override.MergeReworkFeedback != "" {
		base.MergeReworkFeedback = override.MergeReworkFeedback
	}
	base.MergeStates = mergePRMergeStatePrompts(base.MergeStates, override.MergeStates)
	return base
}

func mergePRMergeStatePrompts(base PRMergeStatePrompts, override PRMergeStatePrompts) PRMergeStatePrompts {
	if override.Default != "" {
		base.Default = override.Default
	}
	if override.Dirty != "" {
		base.Dirty = override.Dirty
	}
	if override.Blocked != "" {
		base.Blocked = override.Blocked
	}
	if override.Behind != "" {
		base.Behind = override.Behind
	}
	if override.Unstable != "" {
		base.Unstable = override.Unstable
	}
	if override.Draft != "" {
		base.Draft = override.Draft
	}
	return base
}

func (p PRFlowPrompts) Provider(kind string) PRProviderPrompts {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "github":
		return mergePRProviderPrompts(p.Global, p.GitHub)
	case "codeup":
		return mergePRProviderPrompts(p.Global, p.CodeUp)
	case "gitlab":
		return mergePRProviderPrompts(p.Global, p.GitLab)
	default:
		return p.Global
	}
}
