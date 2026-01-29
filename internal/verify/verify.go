package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yoke233/zhanggui/internal/gateway"
)

type Issue struct {
	Severity string `json:"severity"`
	Where    string `json:"where"`
	What     string `json:"what"`
	Action   string `json:"action,omitempty"`
}

type IssuesFile struct {
	SchemaVersion int     `json:"schema_version"`
	TaskID        string  `json:"task_id"`
	Rev           string  `json:"rev"`
	Issues        []Issue `json:"issues"`
}

type Result struct {
	HasBlocker bool
	Issues     []Issue
}

func VerifyTaskRev(gw *gateway.Gateway, taskID, rev string) (Result, error) {
	if gw == nil {
		return Result{}, fmt.Errorf("gw 不能为空")
	}
	if strings.TrimSpace(taskID) == "" {
		return Result{}, fmt.Errorf("taskID 不能为空")
	}
	if strings.TrimSpace(rev) == "" {
		return Result{}, fmt.Errorf("rev 不能为空")
	}

	taskDir := gw.Root()
	revDir := filepath.Join(taskDir, "revs", rev)
	if _, err := os.Stat(revDir); err != nil {
		return Result{}, err
	}

	var issues []Issue

	summaryPath := filepath.Join(revDir, "summary.md")
	if _, err := os.Stat(summaryPath); err != nil {
		issues = append(issues, Issue{
			Severity: "blocker",
			Where:    "verify",
			What:     "缺少必需文件 summary.md",
			Action:   "在 rev 目录生成 summary.md",
		})
	}

	issuesPath := filepath.Join(revDir, "issues.json")
	var existing IssuesFile
	if b, err := os.ReadFile(issuesPath); err == nil {
		if err := json.Unmarshal(b, &existing); err != nil {
			issues = append(issues, Issue{
				Severity: "blocker",
				Where:    "verify",
				What:     "issues.json 不是合法 JSON",
				Action:   "修复 issues.json 格式",
			})
		}
	} else {
		issues = append(issues, Issue{
			Severity: "blocker",
			Where:    "verify",
			What:     "缺少必需文件 issues.json",
			Action:   "生成 issues.json（允许为空数组）",
		})
	}

	var merged []Issue
	if existing.SchemaVersion != 0 {
		merged = append(merged, existing.Issues...)
	}
	merged = append(merged, issues...)

	merged = normalizeIssues(merged)

	res := Result{Issues: merged}
	for _, it := range merged {
		if strings.EqualFold(it.Severity, "blocker") {
			res.HasBlocker = true
			break
		}
	}

	out := IssuesFile{
		SchemaVersion: 1,
		TaskID:        taskID,
		Rev:           rev,
		Issues:        merged,
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return Result{}, err
	}
	b = append(b, '\n')
	relIssues := filepath.ToSlash(filepath.Join("revs", rev, "issues.json"))
	if err := gw.ReplaceFile(relIssues, b, 0o644, "verify: write issues.json"); err != nil {
		return Result{}, err
	}

	return res, nil
}

func normalizeIssues(in []Issue) []Issue {
	out := make([]Issue, 0, len(in))
	for _, it := range in {
		it.Severity = strings.ToLower(strings.TrimSpace(it.Severity))
		it.Where = strings.TrimSpace(it.Where)
		it.What = strings.TrimSpace(it.What)
		it.Action = strings.TrimSpace(it.Action)
		if it.Severity == "" {
			it.Severity = "warn"
		}
		switch it.Severity {
		case "blocker", "warn", "info":
		default:
			out = append(out, Issue{
				Severity: "blocker",
				Where:    "verify",
				What:     fmt.Sprintf("非法 severity: %s", it.Severity),
				Action:   "severity 仅允许 blocker|warn|info",
			})
			continue
		}
		if it.What == "" {
			continue
		}
		out = append(out, it)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity < out[j].Severity
		}
		if out[i].Where != out[j].Where {
			return out[i].Where < out[j].Where
		}
		return out[i].What < out[j].What
	})
	return out
}
