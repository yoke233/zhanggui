package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
)

const defaultRunTemplate = "standard"

type RunCreateFn func(projectID, name, description, template string) (*core.Run, error)

// RunTrigger creates Runs from issue and slash command events.
type RunTrigger struct {
	store     core.Store
	createRun RunCreateFn
	now       func() time.Time
}

type IssueTriggerInput struct {
	ProjectID            string
	IssueNumber          int
	IssueTitle           string
	IssueBody            string
	Labels               []string
	LabelTemplateMapping map[string]string
	TraceID              string
}

type CommandTriggerInput struct {
	ProjectID            string
	IssueNumber          int
	Message              string
	Template             string
	DefaultTemplate      string
	LabelTemplateMapping map[string]string
	Labels               []string
	TraceID              string
}

func NewRunTrigger(store core.Store, create RunCreateFn) *RunTrigger {
	return &RunTrigger{
		store:     store,
		createRun: create,
		now:       time.Now,
	}
}

func (t *RunTrigger) TriggerFromIssue(ctx context.Context, input IssueTriggerInput) (*core.Run, error) {
	template := pickTemplate(input.Labels, input.LabelTemplateMapping, defaultRunTemplate)
	name := strings.TrimSpace(input.IssueTitle)
	if name == "" {
		name = fmt.Sprintf("Issue #%d", input.IssueNumber)
	}
	return t.trigger(ctx, triggerInput{
		ProjectID:   input.ProjectID,
		IssueNumber: input.IssueNumber,
		Name:        name,
		Description: input.IssueBody,
		Template:    template,
		TraceID:     input.TraceID,
		Source:      "issue_opened",
	})
}

func (t *RunTrigger) TriggerFromCommand(ctx context.Context, input CommandTriggerInput) (*core.Run, error) {
	template := strings.TrimSpace(input.Template)
	if template == "" {
		defaultTemplate := input.DefaultTemplate
		if strings.TrimSpace(defaultTemplate) == "" {
			defaultTemplate = defaultRunTemplate
		}
		template = pickTemplate(input.Labels, input.LabelTemplateMapping, defaultTemplate)
	}
	description := strings.TrimSpace(input.Message)
	if description == "" {
		description = "triggered by slash command"
	}

	return t.trigger(ctx, triggerInput{
		ProjectID:   input.ProjectID,
		IssueNumber: input.IssueNumber,
		Name:        fmt.Sprintf("Issue #%d command run", input.IssueNumber),
		Description: description,
		Template:    template,
		TraceID:     input.TraceID,
		Source:      "slash_run",
	})
}

type triggerInput struct {
	ProjectID   string
	IssueNumber int
	Name        string
	Description string
	Template    string
	TraceID     string
	Source      string
}

func (t *RunTrigger) trigger(ctx context.Context, input triggerInput) (*core.Run, error) {
	if t == nil || t.store == nil {
		return nil, errors.New("Run trigger store is required")
	}
	if t.createRun == nil {
		return nil, errors.New("Run trigger create function is required")
	}
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return nil, errors.New("project id is required")
	}
	if input.IssueNumber <= 0 {
		return nil, errors.New("issue number must be positive")
	}

	existing, err := t.findExistingIssueRun(projectID, input.IssueNumber)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	template := strings.TrimSpace(input.Template)
	if template == "" {
		template = defaultRunTemplate
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = fmt.Sprintf("Issue #%d", input.IssueNumber)
	}
	Run, err := t.createRun(projectID, name, input.Description, template)
	if err != nil {
		return nil, err
	}
	if Run == nil {
		return nil, errors.New("create Run returned nil")
	}
	if Run.Config == nil {
		Run.Config = map[string]any{}
	}
	Run.Config["issue_number"] = input.IssueNumber
	Run.Config["trigger_source"] = input.Source
	if traceID := strings.TrimSpace(input.TraceID); traceID != "" {
		Run.Config["trace_id"] = traceID
	}
	if Run.QueuedAt.IsZero() {
		Run.QueuedAt = t.now()
	}
	if Run.CreatedAt.IsZero() {
		Run.CreatedAt = t.now()
	}
	Run.UpdatedAt = t.now()

	if err := t.store.SaveRun(Run); err != nil {
		return nil, err
	}
	return Run, nil
}

func (t *RunTrigger) findExistingIssueRun(projectID string, issueNumber int) (*core.Run, error) {
	return engine.FindRunByIssueNumber(t.store, projectID, issueNumber)
}

func pickTemplate(labels []string, mapping map[string]string, fallback string) string {
	if len(mapping) == 0 {
		if strings.TrimSpace(fallback) == "" {
			return defaultRunTemplate
		}
		return strings.TrimSpace(fallback)
	}
	for _, label := range labels {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if normalized == "" {
			continue
		}
		for pattern, template := range mapping {
			if strings.EqualFold(strings.TrimSpace(pattern), normalized) {
				if trimmed := strings.TrimSpace(template); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	if strings.TrimSpace(fallback) == "" {
		return defaultRunTemplate
	}
	return strings.TrimSpace(fallback)
}
