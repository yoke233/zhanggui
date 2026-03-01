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
	ProjectName      string
	RepoPath         string
	WorktreePath     string
	Requirements     string
	ExecutionContext string
	PreviousReview   string
	HumanFeedback    string
	RetryError       string
	RetryCount       int
}

func RenderPrompt(stage string, vars PromptVars) (string, error) {
	data, err := promptFS.ReadFile("prompt_templates/" + stage + ".tmpl")
	if err != nil {
		return fmt.Sprintf("Execute stage: %s\nRequirements: %s", stage, vars.Requirements), nil
	}

	tmpl, err := template.New(stage).Parse(string(data))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, vars); err != nil {
		return "", err
	}
	return b.String(), nil
}
