package engine

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/template"
)

type completenessPromptData struct {
	Conversation     string
	TasksJSON        string
	PlanFileContents string
}

type dependencyPromptData struct {
	TasksJSON        string
	PlanFileContents string
}

type feasibilityPromptData struct {
	TasksJSON        string
	ProjectContext   string
	PlanFileContents string
}

type aggregatorPromptData struct {
	CompletenessVerdict string
	DependencyVerdict   string
	FeasibilityVerdict  string
	TasksJSON           string
	PlanFileContents    string
}

func TestReviewPromptTemplatesParseFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		templateName   string
		data           any
		requiredFields []string
		requiredText   []string
	}{
		{
			templateName: "review_completeness.tmpl",
			data: completenessPromptData{
				Conversation:     "用户希望支持导入、导出并补充异常处理",
				TasksJSON:        `{"tasks":[{"id":"t1","title":"实现导入"}]}`,
				PlanFileContents: "",
			},
			requiredFields: []string{"Conversation", "TasksJSON", "PlanFileContents"},
			requiredText: []string{
				`"status": "pass"|"issues_found"`,
				`"issues": [...]`,
				`"score": 0-100`,
				`检查项（file-based）`,
				`检查项（tasks-based）`,
			},
		},
		{
			templateName: "review_dependency.tmpl",
			data: dependencyPromptData{
				TasksJSON:        `{"tasks":[{"id":"t1","deps":["t0"]}]}`,
				PlanFileContents: "",
			},
			requiredFields: []string{"TasksJSON", "PlanFileContents"},
			requiredText: []string{
				`"status": "pass"|"issues_found"`,
				`"issues": [...]`,
				`"score": 0-100`,
				`检查项（file-based）`,
				`检查项（tasks-based）`,
			},
		},
		{
			templateName: "review_feasibility.tmpl",
			data: feasibilityPromptData{
				TasksJSON:        `{"tasks":[{"id":"t1","acceptance":"go test ./..."}]}`,
				ProjectContext:   "Go 单仓库，含 engine/config/store 包",
				PlanFileContents: "",
			},
			requiredFields: []string{"TasksJSON", "ProjectContext", "PlanFileContents"},
			requiredText: []string{
				`"status": "pass"|"issues_found"`,
				`"issues": [...]`,
				`"score": 0-100`,
				`检查项（file-based）`,
				`检查项（tasks-based）`,
			},
		},
		{
			templateName: "review_aggregator.tmpl",
			data: aggregatorPromptData{
				CompletenessVerdict: `{"status":"issues_found","issues":[{"severity":"warning"}],"score":78}`,
				DependencyVerdict:   `{"status":"pass","issues":[],"score":92}`,
				FeasibilityVerdict:  `{"status":"pass","issues":[],"score":90}`,
				TasksJSON:           `{"tasks":[{"id":"t1","title":"实现审查循环"}]}`,
				PlanFileContents:    "",
			},
			requiredFields: []string{"CompletenessVerdict", "DependencyVerdict", "FeasibilityVerdict", "TasksJSON", "PlanFileContents"},
			requiredText: []string{
				`"decision": "approve"`,
				`"decision": "fix"`,
				`"decision": "escalate"`,
				`"suggestions"`,
				`"revised_tasks"`,
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.templateName, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("..", "..", "configs", "prompts", tc.templateName)
			tmpl, err := template.ParseFiles(path)
			if err != nil {
				t.Fatalf("template.ParseFiles(%q) failed: %v", path, err)
			}

			contentBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("os.ReadFile(%q) failed: %v", path, err)
			}
			content := string(contentBytes)

			for _, field := range tc.requiredFields {
				assertTemplateReferencesField(t, tc.templateName, content, field)
			}

			for _, expected := range tc.requiredText {
				if !strings.Contains(content, expected) {
					t.Fatalf("template %q must include expected text %q", tc.templateName, expected)
				}
			}

			if err := tmpl.Execute(io.Discard, tc.data); err != nil {
				t.Fatalf("execute template %q failed: %v", tc.templateName, err)
			}
		})
	}
}

func TestReviewPromptTemplatesExecuteWithFileBasedInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		templateName string
		data         any
	}{
		{
			templateName: "review_completeness.tmpl",
			data: completenessPromptData{
				Conversation:     "用户希望补齐 parser + review 流程",
				TasksJSON:        `{"tasks":[]}`,
				PlanFileContents: "## Plan\n- parser\n- review",
			},
		},
		{
			templateName: "review_dependency.tmpl",
			data: dependencyPromptData{
				TasksJSON:        `{"tasks":[]}`,
				PlanFileContents: "step1 -> step2 -> step3",
			},
		},
		{
			templateName: "review_feasibility.tmpl",
			data: feasibilityPromptData{
				TasksJSON:        `{"tasks":[]}`,
				ProjectContext:   "Go + SQLite",
				PlanFileContents: "需要先改 parser 再补测试",
			},
		},
		{
			templateName: "review_aggregator.tmpl",
			data: aggregatorPromptData{
				CompletenessVerdict: `{"status":"issues_found","issues":[{"severity":"warning"}],"score":78}`,
				DependencyVerdict:   `{"status":"pass","issues":[],"score":92}`,
				FeasibilityVerdict:  `{"status":"pass","issues":[],"score":90}`,
				TasksJSON:           `{"tasks":[]}`,
				PlanFileContents:    "## raw plan file",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.templateName, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("..", "..", "configs", "prompts", tc.templateName)
			tmpl, err := template.ParseFiles(path)
			if err != nil {
				t.Fatalf("template.ParseFiles(%q) failed: %v", path, err)
			}
			if err := tmpl.Execute(io.Discard, tc.data); err != nil {
				t.Fatalf("execute template %q with file-based input failed: %v", tc.templateName, err)
			}
		})
	}
}

func assertTemplateReferencesField(t *testing.T, templateName, content, field string) {
	t.Helper()

	pattern := regexp.MustCompile(`\{\{[^}]*\.` + regexp.QuoteMeta(field) + `[^}]*\}\}`)
	if !pattern.MatchString(content) {
		t.Fatalf("template %q must reference field %q", templateName, field)
	}
}
