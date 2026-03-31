package orchestrateapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/application/planning"
	"github.com/yoke233/zhanggui/internal/application/threadapp"
	"github.com/yoke233/zhanggui/internal/application/workitemapp"
	"github.com/yoke233/zhanggui/internal/core"
)

type Service struct {
	store           Store
	workItemCreator WorkItemCreator
	planner         Planner
	threads         ThreadCoordinator
	registry        core.AgentRegistry
	now             func() time.Time
}

type CreateTaskInput struct {
	Title               string
	Body                string
	ProjectID           *int64
	Priority            string
	Labels              []string
	DedupeKey           string
	SourceChatSessionID string
	SourceGoalRef       string
	ParentWorkItemID    *int64
	RootWorkItemID      *int64
	ExecutorProfile     string
	ReviewerProfile     string
	SponsorProfile      string
	ActorProfile        string
}

type CreateTaskResult struct {
	WorkItem *core.WorkItem
	Created  bool
}

type FollowUpTaskInput struct {
	WorkItemID int64
}

type FollowUpTaskResult struct {
	WorkItemID          int64
	Status              core.WorkItemStatus
	Blocked             bool
	LatestRunSummary    string
	RecommendedNextStep string
	ActiveProfileID     string
}

type ReassignTaskInput struct {
	WorkItemID    int64
	NewProfile    string
	Reason        string
	ActorProfile  string
	SourceSession string
}

type ReassignTaskResult struct {
	WorkItemID     int64
	OldProfile     string
	NewProfile     string
	JournalEntries []map[string]any
}

type DecomposeTaskInput struct {
	WorkItemID        int64
	Objective         string
	OverwriteExisting bool
}

type DecomposeTaskResult struct {
	WorkItemID  int64
	ActionCount int
}

type EscalateThreadInput struct {
	WorkItemID     int64
	Reason         string
	ThreadTitle    string
	ActorProfile   string
	SourceSession  string
	InviteProfiles []string
	InviteHumans   []string
	ForceNew       bool
}

type EscalateThreadResult struct {
	WorkItemID int64
	Thread     *core.Thread
	Created    bool
}

func New(cfg Config) *Service {
	return &Service{
		store:           cfg.Store,
		workItemCreator: cfg.WorkItemCreator,
		planner:         cfg.Planner,
		threads:         cfg.Threads,
		registry:        cfg.Registry,
		now:             time.Now,
	}
}

func (s *Service) CreateTask(ctx context.Context, input CreateTaskInput) (*CreateTaskResult, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, newError(CodeMissingTitle, "title is required", nil)
	}
	if existing, err := s.findExistingTask(ctx, strings.TrimSpace(input.DedupeKey), strings.TrimSpace(input.SourceGoalRef)); err != nil {
		return nil, err
	} else if existing != nil {
		return &CreateTaskResult{WorkItem: existing, Created: false}, nil
	}

	if s.workItemCreator == nil {
		return nil, fmt.Errorf("work item creator is not configured")
	}

	executorProfile := s.resolveExecutorProfile(ctx, input.ExecutorProfile)
	metadata := map[string]any{
		"orchestrate": map[string]any{
			"source_chat_session_id": strings.TrimSpace(input.SourceChatSessionID),
			"source_goal_ref":        strings.TrimSpace(input.SourceGoalRef),
			"dedupe_key":             strings.TrimSpace(input.DedupeKey),
		},
	}
	workItem, err := s.workItemCreator.CreateWorkItem(ctx, workitemapp.CreateWorkItemInput{
		ProjectID:          input.ProjectID,
		ParentWorkItemID:   input.ParentWorkItemID,
		RootWorkItemID:     input.RootWorkItemID,
		Title:              title,
		Body:               strings.TrimSpace(input.Body),
		Priority:           strings.TrimSpace(input.Priority),
		ExecutorProfileID:  executorProfile,
		ReviewerProfileID:  strings.TrimSpace(input.ReviewerProfile),
		ActiveProfileID:    executorProfile,
		SponsorProfileID:   strings.TrimSpace(input.SponsorProfile),
		CreatedByProfileID: defaultActorProfileID(input.ActorProfile),
		Labels:             cloneStrings(input.Labels),
		Metadata:           metadata,
	})
	if err != nil {
		return nil, err
	}
	return &CreateTaskResult{WorkItem: workItem, Created: true}, nil
}

func (s *Service) FollowUpTask(ctx context.Context, input FollowUpTaskInput) (*FollowUpTaskResult, error) {
	workItem, err := s.store.GetWorkItem(ctx, input.WorkItemID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}

	actions, err := s.store.ListActionsByWorkItem(ctx, workItem.ID)
	if err != nil {
		return nil, err
	}

	activeProfile := firstNonEmpty(workItem.ActiveProfileID, workItem.ExecutorProfileID)
	latestSummary := ""
	if workItem.FinalDeliverableID != nil {
		deliverable, deliverableErr := s.store.GetDeliverable(ctx, *workItem.FinalDeliverableID)
		if deliverableErr == nil {
			latestSummary = summarizeDeliverable(deliverable)
		}
	}
	for _, action := range actions {
		if latestSummary != "" {
			break
		}
		if action == nil {
			continue
		}
		run, err := s.store.GetLatestRunWithResult(ctx, action.ID)
		if err == nil && run != nil && strings.TrimSpace(run.ResultMarkdown) != "" {
			latestSummary = compactString(run.ResultMarkdown, 160)
		}
	}

	blocked := workItem.Status == core.WorkItemBlocked || hasActionStatus(actions, core.ActionBlocked, core.ActionFailed)
	nextStep := recommendedNextStep(workItem.Status, len(actions), blocked, activeProfile)

	return &FollowUpTaskResult{
		WorkItemID:          workItem.ID,
		Status:              workItem.Status,
		Blocked:             blocked,
		LatestRunSummary:    latestSummary,
		RecommendedNextStep: nextStep,
		ActiveProfileID:     activeProfile,
	}, nil
}

func (s *Service) ReassignTask(ctx context.Context, input ReassignTaskInput) (*ReassignTaskResult, error) {
	workItem, err := s.store.GetWorkItem(ctx, input.WorkItemID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}

	oldProfile := firstNonEmpty(workItem.ActiveProfileID, workItem.ExecutorProfileID)
	newProfile := strings.TrimSpace(input.NewProfile)
	if newProfile == "" {
		return nil, newError(CodeMissingProfile, "profile is required", nil)
	}
	if err := s.propagatePreferredProfile(ctx, workItem.ID, newProfile); err != nil {
		return nil, err
	}
	workItem.ExecutorProfileID = newProfile
	workItem.ActiveProfileID = newProfile
	if err := s.refreshResponsibilityFields(ctx, workItem); err != nil {
		return nil, err
	}
	if err := s.store.UpdateWorkItem(ctx, workItem); err != nil {
		return nil, err
	}
	entry := map[string]any{
		"reason":                 strings.TrimSpace(input.Reason),
		"source_chat_session_id": strings.TrimSpace(input.SourceSession),
		"from_profile_id":        oldProfile,
		"to_profile_id":          newProfile,
		"reviewer_profile_id":    workItem.ReviewerProfileID,
		"sponsor_profile_id":     workItem.SponsorProfileID,
		"escalation_path":        cloneStrings(workItem.EscalationPath),
	}
	if _, err := s.store.AppendJournal(ctx, &core.JournalEntry{
		WorkItemID: workItem.ID,
		Kind:       core.JournalAssignment,
		Source:     journalSourceForActor(input.ActorProfile),
		Summary:    summarizeReassignment(oldProfile, newProfile),
		Payload:    entry,
		Actor:      strings.TrimSpace(input.ActorProfile),
		CreatedAt:  s.now().UTC(),
	}); err != nil {
		return nil, err
	}

	return &ReassignTaskResult{
		WorkItemID:     workItem.ID,
		OldProfile:     oldProfile,
		NewProfile:     newProfile,
		JournalEntries: []map[string]any{entry},
	}, nil
}

func (s *Service) DecomposeTask(ctx context.Context, input DecomposeTaskInput) (*DecomposeTaskResult, error) {
	if s.planner == nil {
		return nil, fmt.Errorf("task decomposition service is not configured")
	}
	workItem, err := s.store.GetWorkItem(ctx, input.WorkItemID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}

	actions, err := s.store.ListActionsByWorkItem(ctx, workItem.ID)
	if err != nil {
		return nil, err
	}
	if input.OverwriteExisting && hasStartedActions(actions) {
		return nil, newError(CodeDecomposeConflict, "cannot overwrite active or completed actions", nil)
	}
	if len(actions) > 0 && !input.OverwriteExisting {
		return &DecomposeTaskResult{WorkItemID: workItem.ID, ActionCount: len(actions)}, nil
	}
	if input.OverwriteExisting {
		for _, action := range actions {
			if action == nil {
				continue
			}
			if err := s.store.DeleteAction(ctx, action.ID); err != nil {
				return nil, err
			}
		}
	}

	dag, err := s.planner.Generate(ctx, planning.GenerateInput{Description: strings.TrimSpace(input.Objective)})
	if err != nil {
		return nil, err
	}
	created, err := planning.MaterializeDAG(ctx, s.store, workItem.ID, dag)
	if err != nil {
		return nil, err
	}
	preferredProfile := firstNonEmpty(workItem.ActiveProfileID, workItem.ExecutorProfileID)
	if preferredProfile != "" {
		for _, action := range created {
			if action == nil || !isExecutableAction(action) {
				continue
			}
			if action.Config == nil {
				action.Config = map[string]any{}
			}
			action.Config["preferred_profile_id"] = preferredProfile
			if err := s.store.UpdateAction(ctx, action); err != nil {
				return nil, err
			}
		}
	}
	return &DecomposeTaskResult{WorkItemID: workItem.ID, ActionCount: len(created)}, nil
}

func (s *Service) EscalateThread(ctx context.Context, input EscalateThreadInput) (*EscalateThreadResult, error) {
	if s.threads == nil {
		return nil, fmt.Errorf("thread coordinator is not configured")
	}

	workItem, err := s.store.GetWorkItem(ctx, input.WorkItemID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}

	if !input.ForceNew {
		thread, err := s.threads.FindActiveThreadByWorkItem(ctx, workItem.ID)
		if err != nil {
			return nil, err
		}
		if thread != nil {
			if _, err := s.threads.EnsureAgentParticipants(ctx, thread.ID, input.InviteProfiles); err != nil {
				return nil, err
			}
			if _, err := s.threads.EnsureHumanParticipants(ctx, thread.ID, input.InviteHumans); err != nil {
				return nil, err
			}
			if err := s.appendEscalationJournal(ctx, workItem, input, thread.ID, false); err != nil {
				return nil, err
			}
			return &EscalateThreadResult{WorkItemID: workItem.ID, Thread: thread, Created: false}, nil
		}
	}

	threadTitle := strings.TrimSpace(input.ThreadTitle)
	if threadTitle == "" {
		threadTitle = workItem.Title
	}
	createdThread, err := s.threads.CreateThread(ctx, threadapp.CreateThreadInput{
		Title:              threadTitle,
		OwnerID:            strings.TrimSpace(input.ActorProfile),
		ParticipantUserIDs: cloneStrings(input.InviteHumans),
		Metadata: map[string]any{
			"source_type":            "ceo_escalation",
			"source_work_item_id":    workItem.ID,
			"source_chat_session_id": strings.TrimSpace(input.SourceSession),
			"escalation_reason":      strings.TrimSpace(input.Reason),
			"invite_profiles":        cloneStrings(input.InviteProfiles),
		},
	})
	if err != nil {
		return nil, err
	}

	_, err = s.threads.LinkThreadWorkItem(ctx, threadapp.LinkThreadWorkItemInput{
		ThreadID:     createdThread.Thread.ID,
		WorkItemID:   workItem.ID,
		RelationType: "drives",
		IsPrimary:    true,
	})
	if err != nil {
		_ = s.threads.DeleteThread(ctx, createdThread.Thread.ID)
		return nil, err
	}
	if _, err := s.threads.EnsureAgentParticipants(ctx, createdThread.Thread.ID, input.InviteProfiles); err != nil {
		_ = s.threads.DeleteThread(ctx, createdThread.Thread.ID)
		return nil, err
	}
	if err := s.appendEscalationJournal(ctx, workItem, input, createdThread.Thread.ID, true); err != nil {
		return nil, err
	}
	return &EscalateThreadResult{WorkItemID: workItem.ID, Thread: createdThread.Thread, Created: true}, nil
}

func (s *Service) findExistingTask(ctx context.Context, dedupeKey string, sourceGoalRef string) (*core.WorkItem, error) {
	if dedupeKey == "" && sourceGoalRef == "" {
		return nil, nil
	}
	archived := false
	items, err := s.store.ListWorkItems(ctx, core.WorkItemFilter{Archived: &archived})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item == nil || isClosedStatus(item.Status) {
			continue
		}
		if dedupeKey != "" && metadataValue(item.Metadata, "orchestrate", "dedupe_key") == dedupeKey {
			return item, nil
		}
		if sourceGoalRef != "" && metadataValue(item.Metadata, "orchestrate", "source_goal_ref") == sourceGoalRef {
			return item, nil
		}
	}
	return nil, nil
}

func (s *Service) appendEscalationJournal(ctx context.Context, workItem *core.WorkItem, input EscalateThreadInput, threadID int64, created bool) error {
	if workItem == nil {
		return nil
	}
	entry := map[string]any{
		"reason":                 strings.TrimSpace(input.Reason),
		"source_chat_session_id": strings.TrimSpace(input.SourceSession),
		"thread_id":              threadID,
		"thread_created":         created,
		"invite_profiles":        cloneStrings(input.InviteProfiles),
		"invite_humans":          cloneStrings(input.InviteHumans),
	}
	_, err := s.store.AppendJournal(ctx, &core.JournalEntry{
		WorkItemID: workItem.ID,
		Kind:       core.JournalSystem,
		Source:     journalSourceForActor(input.ActorProfile),
		Summary:    "escalated work item into coordination thread",
		Payload:    entry,
		Actor:      strings.TrimSpace(input.ActorProfile),
		CreatedAt:  s.now().UTC(),
	})
	return err
}

func metadataValue(metadata map[string]any, path ...string) string {
	current := any(metadata)
	for _, key := range path {
		next, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = next[key]
	}
	value, _ := current.(string)
	return strings.TrimSpace(value)
}

func recommendedNextStep(status core.WorkItemStatus, actionCount int, blocked bool, activeProfile string) string {
	switch {
	case actionCount == 0:
		return "decompose"
	case blocked && activeProfile != "":
		return "reassign_or_escalate"
	case blocked:
		return "investigate_blocker"
	case activeProfile == "":
		return "assign_profile"
	case status == core.WorkItemOpen || status == core.WorkItemAccepted:
		return "run_work_item"
	default:
		return "follow_up_with_" + activeProfile
	}
}

func hasActionStatus(actions []*core.Action, statuses ...core.ActionStatus) bool {
	if len(statuses) == 0 {
		return false
	}
	set := make(map[core.ActionStatus]struct{}, len(statuses))
	for _, status := range statuses {
		set[status] = struct{}{}
	}
	for _, action := range actions {
		if action == nil {
			continue
		}
		if _, ok := set[action.Status]; ok {
			return true
		}
	}
	return false
}

func hasStartedActions(actions []*core.Action) bool {
	for _, action := range actions {
		if action == nil {
			continue
		}
		switch action.Status {
		case core.ActionRunning, core.ActionDone, core.ActionWaitingGate, core.ActionBlocked, core.ActionFailed, core.ActionCancelled:
			return true
		}
	}
	return false
}

func isExecutableAction(action *core.Action) bool {
	if action == nil {
		return false
	}
	return action.Type == core.ActionExec || action.Type == core.ActionComposite
}

func shouldPropagatePreferredProfile(action *core.Action) bool {
	if !isExecutableAction(action) {
		return false
	}
	switch action.Status {
	case core.ActionPending, core.ActionReady, core.ActionBlocked, core.ActionFailed:
		return true
	default:
		return false
	}
}

func isClosedStatus(status core.WorkItemStatus) bool {
	switch status {
	case core.WorkItemCompleted, core.WorkItemCancelled:
		return true
	default:
		return false
	}
}

func compactString(raw string, limit int) string {
	trimmed := strings.TrimSpace(raw)
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed
	}
	if limit <= 3 {
		return trimmed[:limit]
	}
	return trimmed[:limit-3] + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultActorProfileID(profileID string) string {
	return firstNonEmpty(profileID, "ceo")
}

func summarizeDeliverable(deliverable *core.Deliverable) string {
	if deliverable == nil {
		return ""
	}
	if summary := strings.TrimSpace(deliverable.Summary); summary != "" {
		return compactString(summary, 160)
	}
	if title := strings.TrimSpace(deliverable.Title); title != "" {
		return compactString(title, 160)
	}
	return ""
}

func journalSourceForActor(actorProfile string) core.JournalSource {
	if strings.TrimSpace(actorProfile) != "" {
		return core.JournalSourceAgent
	}
	return core.JournalSourceSystem
}

func summarizeReassignment(oldProfile string, newProfile string) string {
	if strings.TrimSpace(oldProfile) == "" {
		return "assigned active owner to " + strings.TrimSpace(newProfile)
	}
	return fmt.Sprintf("reassigned active owner from %s to %s", strings.TrimSpace(oldProfile), strings.TrimSpace(newProfile))
}

func (s *Service) refreshResponsibilityFields(ctx context.Context, workItem *core.WorkItem) error {
	if s == nil || workItem == nil {
		return nil
	}
	if workItem.ActiveProfileID == "" {
		workItem.ActiveProfileID = firstNonEmpty(workItem.ExecutorProfileID, workItem.ReviewerProfileID)
	}
	if s.registry == nil || workItem.ExecutorProfileID == "" {
		return nil
	}
	reviewer, err := workitemapp.DefaultReviewerProfileID(ctx, workItem.ExecutorProfileID, s.registry)
	if err != nil {
		return err
	}
	workItem.ReviewerProfileID = strings.TrimSpace(reviewer)
	if workItem.ActiveProfileID != "" {
		path, pathErr := workitemapp.BuildEscalationPath(ctx, workItem.ActiveProfileID, s.registry)
		if pathErr != nil {
			return pathErr
		}
		workItem.EscalationPath = path
	}
	workItem.SponsorProfileID = sponsorProfileFromEscalationPath(workItem.EscalationPath, workItem.ReviewerProfileID)
	return nil
}

func sponsorProfileFromEscalationPath(path []string, reviewerProfileID string) string {
	for i := len(path) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(path[i])
		if candidate != "" && candidate != workitemapp.HumanEscalationTarget {
			return candidate
		}
	}
	return strings.TrimSpace(reviewerProfileID)
}

func (s *Service) resolveExecutorProfile(ctx context.Context, requestedProfile string) string {
	requestedProfile = strings.TrimSpace(requestedProfile)
	if requestedProfile != "" {
		return requestedProfile
	}
	if s == nil || s.registry == nil {
		return "lead"
	}
	for _, candidate := range []string{"lead", "ceo"} {
		if _, err := s.registry.ResolveByID(ctx, candidate); err == nil {
			return candidate
		}
	}
	return "lead"
}

func (s *Service) propagatePreferredProfile(ctx context.Context, workItemID int64, profile string) error {
	actions, err := s.store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}
	for _, action := range actions {
		if !shouldPropagatePreferredProfile(action) {
			continue
		}
		if action.Config == nil {
			action.Config = map[string]any{}
		}
		action.Config["preferred_profile_id"] = profile
		if err := s.store.UpdateAction(ctx, action); err != nil {
			return err
		}
	}
	return nil
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for k, v := range metadata {
		switch value := v.(type) {
		case map[string]any:
			out[k] = cloneMetadata(value)
		case []any:
			cloned := make([]any, len(value))
			copy(cloned, value)
			out[k] = cloned
		default:
			out[k] = value
		}
	}
	return out
}

func cloneMetadataMap(raw any) map[string]any {
	current, _ := raw.(map[string]any)
	return cloneMetadata(current)
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if nested, ok := v.(map[string]any); ok {
			out[k] = cloneAnyMap(nested)
			continue
		}
		out[k] = v
	}
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
