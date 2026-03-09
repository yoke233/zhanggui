package engine

import (
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed prompt_templates/*.tmpl
var promptFS embed.FS

type PromptVars struct {
	ProjectName       string
	RepoPath          string
	WorktreePath      string
	Requirements      string
	ExecutionContext  string
	PreviousReview    string
	HumanFeedback     string
	RetryError        string
	MergeConflictHint string
	RetryCount        int
	ColdContext       string
	WarmContext       string
	HotContext        string
}

func RenderPrompt(stage string, vars PromptVars) (string, error) {
	data, tmplName, err := readPromptTemplate(stage)
	if err != nil {
		return fmt.Sprintf("Execute stage: %s\nRequirements: %s", stage, vars.Requirements), nil
	}

	tmpl, err := template.New(tmplName).Parse(string(data))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, vars); err != nil {
		return "", err
	}
	return b.String(), nil
}

func readPromptTemplate(stage string) ([]byte, string, error) {
	candidates := promptTemplateCandidates(stage)
	var lastErr error
	for _, candidate := range candidates {
		data, err := promptFS.ReadFile("prompt_templates/" + candidate + ".tmpl")
		if err == nil {
			return data, candidate, nil
		}
		lastErr = err
	}
	return nil, "", lastErr
}

func promptTemplateCandidates(stage string) []string {
	trimmed := strings.TrimSpace(stage)
	switch trimmed {
	case "review":
		return []string{"review", "code_review"}
	default:
		return []string{trimmed}
	}
}
