package teamleader

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
)

type RunRef struct {
	sessionID string
	issueID   string
}

type readyDispatch struct {
	sessionID string
	issueID   string
}

type runningSession struct {
	SessionID string
	ProjectID string
	Running   map[string]string
	IssueByID map[string]*core.Issue
	HaltNew   bool
	Recovered bool
}

func newRunningSession(sessionID, projectID string, issues []*core.Issue) *runningSession {
	issueByID := make(map[string]*core.Issue, len(issues))
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		issueByID[issue.ID] = issue
	}

	return &runningSession{
		SessionID: sessionID,
		ProjectID: projectID,
		Running:   make(map[string]string),
		IssueByID: issueByID,
	}
}

func (rs *runningSession) readyToDispatchIDs() []string {
	readyByProfile := map[core.WorkflowProfileType][]string{
		core.WorkflowProfileStrict:      {},
		core.WorkflowProfileNormal:      {},
		core.WorkflowProfileFastRelease: {},
	}
	for _, issueID := range sortedIssueIDs(rs.IssueByID) {
		issue := rs.IssueByID[issueID]
		if issue == nil {
			continue
		}
		if issue.Status != core.IssueStatusReady {
			continue
		}
		if _, running := rs.Running[issueID]; running {
			continue
		}
		profile := workflowProfileFromIssue(issue)
		readyByProfile[profile] = append(readyByProfile[profile], issueID)
	}
	ordered := make([]string, 0, len(rs.IssueByID))
	for _, profile := range workflowDispatchProfileOrder() {
		ids := readyByProfile[profile]
		sort.Strings(ids)
		ordered = append(ordered, ids...)
	}
	return ordered
}

// RunRecoveryEvent maps a terminal run's status+conclusion to an event for scheduler replay.
// All non-success conclusions (failure, timed_out, cancelled) map to EventRunFailed
// because the scheduler treats any non-success outcome identically: mark the issue failed
// and apply the session's fail policy.
func RunRecoveryEvent(status core.RunStatus, conclusion core.RunConclusion) (core.EventType, bool) {
	if status != core.StatusCompleted {
		return "", false
	}
	if conclusion == core.ConclusionSuccess {
		return core.EventRunDone, true
	}
	return core.EventRunFailed, true
}

func workflowDispatchProfileOrder() []core.WorkflowProfileType {
	return []core.WorkflowProfileType{
		core.WorkflowProfileStrict,
		core.WorkflowProfileNormal,
		core.WorkflowProfileFastRelease,
	}
}

func workflowProfileFromIssue(issue *core.Issue) core.WorkflowProfileType {
	if issue == nil {
		return core.WorkflowProfileNormal
	}
	for _, label := range issue.Labels {
		trimmed := strings.TrimSpace(strings.ToLower(label))
		if !strings.HasPrefix(trimmed, "profile:") {
			continue
		}
		candidate := core.WorkflowProfileType(strings.TrimSpace(strings.TrimPrefix(trimmed, "profile:")))
		if candidate.Validate() == nil {
			return candidate
		}
	}
	if candidate := core.WorkflowProfileType(strings.TrimSpace(strings.ToLower(issue.Template))); candidate.Validate() == nil {
		return candidate
	}
	return core.WorkflowProfileNormal
}

func buildRunFromIssue(issue *core.Issue, profile core.WorkflowProfileType, stageRoles map[core.StageID]string) (*core.Run, error) {
	if issue == nil {
		return nil, errors.New("issue cannot be nil")
	}

	template := strings.TrimSpace(issue.Template)
	if template == "" {
		template = "standard"
	}
	stages, err := buildSchedulerStages(template, stageRoles)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(issue.Title)
	if name == "" {
		name = issue.ID
	}

	now := time.Now()
	config := map[string]any{
		"workflow_profile": string(profile),
	}
	if issue.MergeRetries > 0 {
		config["merge_conflict_hint"] = "上一次实现与主干产生合并冲突，请先 rebase 解决冲突后再实现需求。"
	}
	return &core.Run{
		ID:              engine.NewRunID(),
		ProjectID:       issue.ProjectID,
		Name:            name,
		Description:     issue.Body,
		Template:        template,
		Status:          core.StatusQueued,
		Stages:          stages,
		Artifacts:       map[string]string{},
		Config:          config,
		IssueID:         issue.ID,
		MaxTotalRetries: 5,
		QueuedAt:        now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func buildSchedulerStages(template string, stageRoles map[core.StageID]string) ([]core.StageConfig, error) {
	stageIDs, ok := engine.Templates[template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", template)
	}

	stages := make([]core.StageConfig, len(stageIDs))
	for i, stageID := range stageIDs {
		stages[i] = schedulerDefaultStageConfig(stageID)
		if role, ok := stageRoles[stageID]; ok {
			stages[i].Role = role
		}
	}
	return stages, nil
}

func schedulerDefaultStageConfig(id core.StageID) core.StageConfig {
	cfg := core.StageConfig{
		Name:           id,
		PromptTemplate: string(id),
		Timeout:        30 * time.Minute,
		MaxRetries:     1,
		OnFailure:      core.OnFailureHuman,
	}

	switch id {
	case core.StageRequirements, core.StageReview:
		cfg.Agent = "codex"
	case core.StageImplement:
		cfg.Agent = "codex"
	case core.StageFixup:
		cfg.Agent = "codex"
		cfg.ReuseSessionFrom = core.StageImplement
	case core.StageTest:
		cfg.Agent = "codex"
		cfg.Timeout = 15 * time.Minute
	case core.StageSetup, core.StageMerge, core.StageCleanup:
		cfg.Timeout = 2 * time.Minute
	}
	return cfg
}

func isIssueTerminal(status core.IssueStatus) bool {
	switch status {
	case core.IssueStatusDone, core.IssueStatusFailed, core.IssueStatusSuperseded, core.IssueStatusAbandoned:
		return true
	default:
		return false
	}
}

func makeSessionID(projectID, sessionID string) string {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID != "" {
		return trimmedSessionID
	}
	return "project:" + strings.TrimSpace(projectID)
}

func groupIssuesBySession(issues []*core.Issue) (map[string][]*core.Issue, error) {
	grouped := make(map[string][]*core.Issue)
	sessionProject := make(map[string]string)

	for _, issue := range issues {
		if issue == nil {
			continue
		}
		issueID := strings.TrimSpace(issue.ID)
		projectID := strings.TrimSpace(issue.ProjectID)
		if issueID == "" {
			return nil, errors.New("issue id is required")
		}
		if projectID == "" {
			return nil, fmt.Errorf("issue %s project id is required", issueID)
		}

		issue.ID = issueID
		issue.ProjectID = projectID
		issue.SessionID = strings.TrimSpace(issue.SessionID)

		sessionID := makeSessionID(projectID, issue.SessionID)
		if existingProjectID, ok := sessionProject[sessionID]; ok && existingProjectID != projectID {
			return nil, fmt.Errorf("session %s has mixed project ids: %s vs %s", sessionID, existingProjectID, projectID)
		}
		sessionProject[sessionID] = projectID
		grouped[sessionID] = append(grouped[sessionID], issue)
	}

	if len(grouped) == 0 {
		return nil, errors.New("no issues provided")
	}
	return grouped, nil
}

func sortedSessionIDs(grouped map[string][]*core.Issue) []string {
	sessionIDs := make([]string, 0, len(grouped))
	for sessionID := range grouped {
		sessionIDs = append(sessionIDs, sessionID)
	}
	sort.Strings(sessionIDs)
	return sessionIDs
}

func sortedIssueIDs(issueByID map[string]*core.Issue) []string {
	ids := make([]string, 0, len(issueByID))
	for issueID := range issueByID {
		ids = append(ids, issueID)
	}
	sort.Strings(ids)
	return ids
}
