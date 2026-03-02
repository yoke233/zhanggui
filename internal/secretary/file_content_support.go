package secretary

import (
	"strings"

	"github.com/user/ai-workflow/internal/core"
)

const waitReasonParseFailed = core.WaitParseFailed

func hasPendingFileContents(plan *core.TaskPlan, fallback map[string]string) bool {
	if plan == nil {
		return false
	}
	if plan.HasPendingFileContents() {
		return true
	}
	if !isFileBasedPlan(plan, fallback) {
		return false
	}
	if len(plan.FileContents) > 0 {
		return true
	}
	return len(fallback) > 0
}

func isFileBasedPlan(plan *core.TaskPlan, fallback map[string]string) bool {
	if plan == nil {
		return false
	}
	if len(plan.Tasks) > 0 {
		return false
	}
	if fallback != nil {
		return true
	}
	if plan.HasPendingFileContents() {
		return true
	}
	if len(plan.SourceFiles) > 0 {
		return true
	}
	if len(plan.FileContents) > 0 {
		return true
	}
	// 保留非 nil 语义：调用方显式提供空 map 也视作 file-based 上下文。
	return fallback != nil
}

func isWaitParseFailed(reason core.WaitReason) bool {
	return strings.EqualFold(strings.TrimSpace(string(reason)), string(waitReasonParseFailed))
}

func copyPlanOptionalFileFields(dst, src *core.TaskPlan) {
	if dst == nil || src == nil {
		return
	}
	dst.SourceFiles = cloneStringSlice(src.SourceFiles)
	dst.FileContents = cloneStringMap(src.FileContents)
}

func loadPlanSourceFiles(plan *core.TaskPlan) []string {
	if plan == nil {
		return nil
	}
	return cloneStringSlice(plan.SourceFiles)
}

func setPlanSourceFiles(plan *core.TaskPlan, sourceFiles []string) {
	if plan == nil {
		return
	}
	plan.SourceFiles = cloneStringSlice(sourceFiles)
}

func loadPlanFileContents(plan *core.TaskPlan) map[string]string {
	if plan == nil {
		return nil
	}
	return cloneStringMap(plan.FileContents)
}

func setPlanFileContents(plan *core.TaskPlan, fileContents map[string]string) {
	if plan == nil {
		return
	}
	plan.FileContents = cloneStringMap(fileContents)
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
