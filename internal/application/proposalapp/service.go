package proposalapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
)

type Service struct {
	store Store
	tx    Tx
	bus   core.EventBus
}

func New(cfg Config) *Service {
	return &Service{store: cfg.Store, tx: cfg.Tx, bus: cfg.Bus}
}

type CreateProposalInput struct {
	ThreadID        int64
	Title           string
	Summary         string
	Content         string
	ProposedBy      string
	WorkItemDrafts  []core.ProposalWorkItemDraft
	SourceMessageID *int64
	Metadata        map[string]any
}

type UpdateProposalInput struct {
	ID              int64
	Title           *string
	Summary         *string
	Content         *string
	Metadata        *map[string]any
	SourceMessageID *int64
}

type ReviewInput struct {
	ReviewedBy string
	ReviewNote string
}

type ReviseInput struct {
	ReviewedBy string
	ReviewNote string
}

func (s *Service) CreateProposal(ctx context.Context, input CreateProposalInput) (*core.ThreadProposal, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("proposal service is not configured")
	}
	if input.ThreadID <= 0 {
		return nil, fmt.Errorf("thread_id is required")
	}
	if _, err := s.store.GetThread(ctx, input.ThreadID); err != nil {
		return nil, err
	}

	drafts, err := normalizeDrafts(input.WorkItemDrafts, false)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	proposal := &core.ThreadProposal{
		ThreadID:        input.ThreadID,
		Title:           title,
		Summary:         strings.TrimSpace(input.Summary),
		Content:         strings.TrimSpace(input.Content),
		ProposedBy:      strings.TrimSpace(input.ProposedBy),
		Status:          core.ProposalDraft,
		WorkItemDrafts:  drafts,
		SourceMessageID: input.SourceMessageID,
		Metadata:        cloneAnyMap(input.Metadata),
	}

	var systemMsg *core.ThreadMessage
	run := func(ctx context.Context, store Store) error {
		id, err := store.CreateThreadProposal(ctx, proposal)
		if err != nil {
			return err
		}
		proposal.ID = id
		systemMsg, err = persistSystemMessage(ctx, store, proposal.ThreadID, buildProposalCreatedMessage(proposal), map[string]any{
			"type":        "proposal_created",
			"proposal_id": proposal.ID,
			"status":      string(proposal.Status),
		})
		return err
	}
	if err := s.withTx(ctx, run); err != nil {
		return nil, err
	}
	s.publishThreadMessageEvent(ctx, systemMsg)
	s.publishProposalEvent(ctx, core.EventThreadProposalCreated, proposal, nil)
	return proposal, nil
}

func (s *Service) ListThreadProposals(ctx context.Context, threadID int64, status *core.ProposalStatus) ([]*core.ThreadProposal, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("proposal service is not configured")
	}
	if threadID <= 0 {
		return nil, fmt.Errorf("thread_id is required")
	}
	if _, err := s.store.GetThread(ctx, threadID); err != nil {
		return nil, err
	}
	return s.store.ListThreadProposals(ctx, core.ProposalFilter{
		ThreadID: &threadID,
		Status:   status,
		Limit:    100,
	})
}

func (s *Service) GetProposal(ctx context.Context, proposalID int64) (*core.ThreadProposal, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("proposal service is not configured")
	}
	return s.store.GetThreadProposal(ctx, proposalID)
}

func (s *Service) UpdateProposal(ctx context.Context, input UpdateProposalInput) (*core.ThreadProposal, error) {
	proposal, err := s.GetProposal(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if proposal.Status != core.ProposalDraft && proposal.Status != core.ProposalRevised {
		return nil, core.ErrInvalidTransition
	}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return nil, fmt.Errorf("title is required")
		}
		proposal.Title = title
	}
	if input.Summary != nil {
		proposal.Summary = strings.TrimSpace(*input.Summary)
	}
	if input.Content != nil {
		proposal.Content = strings.TrimSpace(*input.Content)
	}
	if input.Metadata != nil {
		proposal.Metadata = cloneAnyMap(*input.Metadata)
	}
	if input.SourceMessageID != nil {
		proposal.SourceMessageID = input.SourceMessageID
	}
	if err := s.store.UpdateThreadProposal(ctx, proposal); err != nil {
		return nil, err
	}
	return proposal, nil
}

func (s *Service) ReplaceDrafts(ctx context.Context, proposalID int64, drafts []core.ProposalWorkItemDraft) (*core.ThreadProposal, error) {
	proposal, err := s.GetProposal(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	if proposal.Status != core.ProposalDraft && proposal.Status != core.ProposalRevised {
		return nil, core.ErrInvalidTransition
	}
	normalized, err := normalizeDrafts(drafts, false)
	if err != nil {
		return nil, err
	}
	proposal.WorkItemDrafts = normalized
	if err := s.store.UpdateThreadProposal(ctx, proposal); err != nil {
		return nil, err
	}
	return proposal, nil
}

func (s *Service) DeleteProposal(ctx context.Context, proposalID int64) error {
	proposal, err := s.GetProposal(ctx, proposalID)
	if err != nil {
		return err
	}
	if proposal.Status != core.ProposalDraft {
		return core.ErrInvalidTransition
	}
	return s.store.DeleteThreadProposal(ctx, proposalID)
}

func (s *Service) Submit(ctx context.Context, proposalID int64) (*core.ThreadProposal, error) {
	proposal, err := s.GetProposal(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	if !core.CanTransitionProposalStatus(proposal.Status, core.ProposalOpen) {
		return nil, core.ErrInvalidTransition
	}
	drafts, err := normalizeDrafts(proposal.WorkItemDrafts, true)
	if err != nil {
		return nil, err
	}
	if err := validateDraftProjects(ctx, s.store, drafts); err != nil {
		return nil, err
	}
	proposal.Status = core.ProposalOpen

	var systemMsg *core.ThreadMessage
	run := func(ctx context.Context, store Store) error {
		if err := store.UpdateThreadProposal(ctx, proposal); err != nil {
			return err
		}
		var err error
		systemMsg, err = persistSystemMessage(ctx, store, proposal.ThreadID, buildProposalSubmittedMessage(proposal), map[string]any{
			"type":        "proposal_submitted",
			"proposal_id": proposal.ID,
			"status":      string(proposal.Status),
		})
		return err
	}
	if err := s.withTx(ctx, run); err != nil {
		return nil, err
	}
	s.publishThreadMessageEvent(ctx, systemMsg)
	s.publishProposalEvent(ctx, core.EventThreadProposalSubmitted, proposal, nil)
	return proposal, nil
}

func (s *Service) Approve(ctx context.Context, proposalID int64, input ReviewInput) (*core.ThreadProposal, error) {
	proposal, err := s.GetProposal(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	if !core.CanTransitionProposalStatus(proposal.Status, core.ProposalApproved) {
		return nil, core.ErrInvalidTransition
	}
	drafts, err := normalizeDrafts(proposal.WorkItemDrafts, true)
	if err != nil {
		return nil, err
	}
	if err := validateDraftProjects(ctx, s.store, drafts); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	reviewedBy := ptr(strings.TrimSpace(input.ReviewedBy))
	proposal.ReviewedBy = reviewedBy
	proposal.ReviewedAt = &now
	proposal.ReviewNote = strings.TrimSpace(input.ReviewNote)

	var initiative *core.Initiative
	var systemMsg *core.ThreadMessage
	run := func(ctx context.Context, store Store) error {
		if _, err := store.GetThread(ctx, proposal.ThreadID); err != nil {
			return err
		}
		var err error
		initiative, err = materializeProposal(ctx, store, proposal, drafts)
		if err != nil {
			return err
		}
		proposal.Status = core.ProposalMerged
		proposal.InitiativeID = &initiative.ID
		if err := store.UpdateThreadProposal(ctx, proposal); err != nil {
			return err
		}
		systemMsg, err = persistSystemMessage(ctx, store, proposal.ThreadID, buildProposalMergedMessage(proposal, initiative.ID), map[string]any{
			"type":          "proposal_merged",
			"proposal_id":   proposal.ID,
			"initiative_id": initiative.ID,
			"status":        string(proposal.Status),
		})
		return err
	}
	if err := s.withTx(ctx, run); err != nil {
		return nil, err
	}
	s.publishThreadMessageEvent(ctx, systemMsg)
	s.publishProposalEvent(ctx, core.EventThreadProposalApproved, proposal, map[string]any{
		"initiative_id": initiative.ID,
		"status":        string(core.ProposalApproved),
	})
	s.publishProposalEvent(ctx, core.EventThreadProposalMerged, proposal, map[string]any{
		"initiative_id": initiative.ID,
	})
	return proposal, nil
}

func (s *Service) Reject(ctx context.Context, proposalID int64, input ReviewInput) (*core.ThreadProposal, error) {
	proposal, err := s.GetProposal(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	if !core.CanTransitionProposalStatus(proposal.Status, core.ProposalRejected) {
		return nil, core.ErrInvalidTransition
	}
	now := time.Now().UTC()
	proposal.Status = core.ProposalRejected
	proposal.ReviewedBy = ptr(strings.TrimSpace(input.ReviewedBy))
	proposal.ReviewedAt = &now
	proposal.ReviewNote = strings.TrimSpace(input.ReviewNote)

	var systemMsg *core.ThreadMessage
	run := func(ctx context.Context, store Store) error {
		if err := store.UpdateThreadProposal(ctx, proposal); err != nil {
			return err
		}
		var err error
		systemMsg, err = persistSystemMessage(ctx, store, proposal.ThreadID, buildProposalRejectedMessage(proposal), map[string]any{
			"type":        "proposal_rejected",
			"proposal_id": proposal.ID,
			"status":      string(proposal.Status),
			"review_note": proposal.ReviewNote,
		})
		return err
	}
	if err := s.withTx(ctx, run); err != nil {
		return nil, err
	}
	s.publishThreadMessageEvent(ctx, systemMsg)
	s.publishProposalEvent(ctx, core.EventThreadProposalRejected, proposal, nil)
	return proposal, nil
}

func (s *Service) Revise(ctx context.Context, proposalID int64, input ReviseInput) (*core.ThreadProposal, error) {
	proposal, err := s.GetProposal(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	if !core.CanTransitionProposalStatus(proposal.Status, core.ProposalRevised) {
		return nil, core.ErrInvalidTransition
	}
	now := time.Now().UTC()
	proposal.Status = core.ProposalRevised
	proposal.ReviewedBy = ptr(strings.TrimSpace(input.ReviewedBy))
	proposal.ReviewedAt = &now
	proposal.ReviewNote = strings.TrimSpace(input.ReviewNote)

	var systemMsg *core.ThreadMessage
	run := func(ctx context.Context, store Store) error {
		if err := store.UpdateThreadProposal(ctx, proposal); err != nil {
			return err
		}
		var err error
		systemMsg, err = persistSystemMessage(ctx, store, proposal.ThreadID, buildProposalRevisedMessage(proposal), map[string]any{
			"type":        "proposal_revised",
			"proposal_id": proposal.ID,
			"status":      string(proposal.Status),
		})
		return err
	}
	if err := s.withTx(ctx, run); err != nil {
		return nil, err
	}
	s.publishThreadMessageEvent(ctx, systemMsg)
	s.publishProposalEvent(ctx, core.EventThreadProposalRevised, proposal, nil)
	return proposal, nil
}

func (s *Service) withTx(ctx context.Context, fn func(ctx context.Context, store Store) error) error {
	if s.tx != nil {
		return s.tx.InTx(ctx, fn)
	}
	return fn(ctx, s.store)
}

func materializeProposal(ctx context.Context, store Store, proposal *core.ThreadProposal, drafts []core.ProposalWorkItemDraft) (*core.Initiative, error) {
	initiative := &core.Initiative{
		Title:       proposal.Title,
		Description: buildInitiativeDescription(proposal),
		Status:      core.InitiativeDraft,
		CreatedBy:   fallbackNonEmpty(strings.TrimSpace(proposal.ProposedBy), "system"),
		Metadata: map[string]any{
			"source_thread_id":   proposal.ThreadID,
			"source_proposal_id": proposal.ID,
			"source_type":        "proposal_materialize",
		},
	}
	if proposal.SourceMessageID != nil {
		initiative.Metadata["source_message_id"] = *proposal.SourceMessageID
	}
	initiativeID, err := store.CreateInitiative(ctx, initiative)
	if err != nil {
		return nil, err
	}
	initiative.ID = initiativeID

	workItems := make(map[string]*core.WorkItem, len(drafts))
	for _, draft := range drafts {
		workItem := &core.WorkItem{
			ProjectID: draft.ProjectID,
			Title:     strings.TrimSpace(draft.Title),
			Body:      strings.TrimSpace(draft.Body),
			Priority:  draft.Priority,
			Labels:    append([]string(nil), draft.Labels...),
			Status:    core.WorkItemOpen,
			Metadata: map[string]any{
				"source_thread_id":   proposal.ThreadID,
				"source_proposal_id": proposal.ID,
				"source_type":        "proposal_materialize",
				"proposal_temp_id":   draft.TempID,
			},
		}
		workItemID, err := store.CreateWorkItem(ctx, workItem)
		if err != nil {
			return nil, err
		}
		workItem.ID = workItemID
		workItems[draft.TempID] = workItem
		if _, err := store.CreateInitiativeItem(ctx, &core.InitiativeItem{
			InitiativeID: initiative.ID,
			WorkItemID:   workItemID,
		}); err != nil {
			return nil, err
		}
	}

	for _, draft := range drafts {
		if len(draft.DependsOn) == 0 {
			continue
		}
		workItem := workItems[draft.TempID]
		dependsOn := make([]int64, 0, len(draft.DependsOn))
		for _, depTempID := range draft.DependsOn {
			dependsOn = append(dependsOn, workItems[depTempID].ID)
		}
		workItem.DependsOn = dependsOn
		if err := store.UpdateWorkItem(ctx, workItem); err != nil {
			return nil, err
		}
	}

	if _, err := store.CreateThreadInitiativeLink(ctx, &core.ThreadInitiativeLink{
		ThreadID:     proposal.ThreadID,
		InitiativeID: initiative.ID,
		RelationType: "source",
	}); err != nil {
		return nil, err
	}
	return initiative, nil
}

func persistSystemMessage(ctx context.Context, store Store, threadID int64, content string, metadata map[string]any) (*core.ThreadMessage, error) {
	msg := &core.ThreadMessage{
		ThreadID: threadID,
		SenderID: "system",
		Role:     "system",
		Content:  strings.TrimSpace(content),
		Metadata: cloneAnyMap(metadata),
	}
	id, err := store.CreateThreadMessage(ctx, msg)
	if err != nil {
		return nil, err
	}
	msg.ID = id
	return msg, nil
}

func (s *Service) publishThreadMessageEvent(ctx context.Context, msg *core.ThreadMessage) {
	if s == nil || s.bus == nil || msg == nil {
		return
	}
	data := map[string]any{
		"thread_id":  msg.ThreadID,
		"message_id": msg.ID,
		"message":    msg.Content,
		"content":    msg.Content,
		"sender_id":  msg.SenderID,
		"role":       msg.Role,
	}
	if len(msg.Metadata) > 0 {
		data["metadata"] = cloneAnyMap(msg.Metadata)
	}
	s.bus.Publish(ctx, core.Event{
		Type:      core.EventThreadMessage,
		Data:      data,
		Timestamp: time.Now().UTC(),
	})
}

func (s *Service) publishProposalEvent(ctx context.Context, eventType core.EventType, proposal *core.ThreadProposal, extra map[string]any) {
	if s == nil || s.bus == nil || proposal == nil {
		return
	}
	data := map[string]any{
		"thread_id":   proposal.ThreadID,
		"proposal_id": proposal.ID,
		"title":       proposal.Title,
		"status":      string(proposal.Status),
	}
	if proposal.InitiativeID != nil {
		data["initiative_id"] = *proposal.InitiativeID
	}
	if proposal.ReviewNote != "" {
		data["review_note"] = proposal.ReviewNote
	}
	for key, value := range extra {
		data[key] = value
	}
	s.bus.Publish(ctx, core.Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	})
}

func normalizeDrafts(drafts []core.ProposalWorkItemDraft, requireNonEmpty bool) ([]core.ProposalWorkItemDraft, error) {
	if len(drafts) == 0 {
		if requireNonEmpty {
			return nil, fmt.Errorf("at least one work item draft is required")
		}
		return []core.ProposalWorkItemDraft{}, nil
	}
	normalized := make([]core.ProposalWorkItemDraft, len(drafts))
	seen := make(map[string]struct{}, len(drafts))
	for i, draft := range drafts {
		tempID := strings.TrimSpace(draft.TempID)
		title := strings.TrimSpace(draft.Title)
		if tempID == "" {
			return nil, fmt.Errorf("draft temp_id is required")
		}
		if title == "" {
			return nil, fmt.Errorf("draft %s title is required", tempID)
		}
		if _, ok := seen[tempID]; ok {
			return nil, fmt.Errorf("duplicate draft temp_id %s", tempID)
		}
		seen[tempID] = struct{}{}
		priority, err := normalizePriority(draft.Priority)
		if err != nil {
			return nil, fmt.Errorf("draft %s priority: %w", tempID, err)
		}
		normalized[i] = core.ProposalWorkItemDraft{
			TempID:    tempID,
			ProjectID: draft.ProjectID,
			Title:     title,
			Body:      strings.TrimSpace(draft.Body),
			Priority:  priority,
			DependsOn: append([]string(nil), draft.DependsOn...),
			Labels:    append([]string(nil), draft.Labels...),
		}
	}
	for i, draft := range normalized {
		depSeen := make(map[string]struct{}, len(draft.DependsOn))
		for _, dep := range draft.DependsOn {
			depID := strings.TrimSpace(dep)
			if depID == "" {
				return nil, fmt.Errorf("draft %s has empty dependency", draft.TempID)
			}
			if depID == draft.TempID {
				return nil, fmt.Errorf("draft %s cannot depend on itself", draft.TempID)
			}
			if _, ok := depSeen[depID]; ok {
				return nil, fmt.Errorf("draft %s has duplicate dependency %s", draft.TempID, depID)
			}
			if _, ok := seen[depID]; !ok {
				return nil, fmt.Errorf("draft %s references unknown dependency %s", draft.TempID, depID)
			}
			depSeen[depID] = struct{}{}
		}
		normalized[i].DependsOn = normalizeStringSlice(draft.DependsOn)
		normalized[i].Labels = normalizeStringSlice(draft.Labels)
	}
	if err := validateDraftDependencyGraph(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func validateDraftProjects(ctx context.Context, store Store, drafts []core.ProposalWorkItemDraft) error {
	if store == nil {
		return fmt.Errorf("proposal store is not configured")
	}
	for _, draft := range drafts {
		if draft.ProjectID == nil {
			continue
		}
		if *draft.ProjectID <= 0 {
			return fmt.Errorf("draft %s has invalid project_id", draft.TempID)
		}
		if _, err := store.GetProject(ctx, *draft.ProjectID); err != nil {
			return fmt.Errorf("draft %s project_id %d: %w", draft.TempID, *draft.ProjectID, err)
		}
	}
	return nil
}

func validateDraftDependencyGraph(drafts []core.ProposalWorkItemDraft) error {
	edges := make(map[string][]string, len(drafts))
	for _, draft := range drafts {
		edges[draft.TempID] = draft.DependsOn
	}
	const (
		stateVisiting = 1
		stateDone     = 2
	)
	seen := make(map[string]int, len(edges))
	var visit func(string) error
	visit = func(node string) error {
		switch seen[node] {
		case stateVisiting:
			return fmt.Errorf("draft dependency cycle detected at %s", node)
		case stateDone:
			return nil
		}
		seen[node] = stateVisiting
		for _, dep := range edges[node] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		seen[node] = stateDone
		return nil
	}
	for node := range edges {
		if err := visit(node); err != nil {
			return err
		}
	}
	return nil
}

func normalizePriority(priority core.WorkItemPriority) (core.WorkItemPriority, error) {
	switch priority {
	case "":
		return core.PriorityMedium, nil
	case core.PriorityLow, core.PriorityMedium, core.PriorityHigh, core.PriorityUrgent:
		return priority, nil
	default:
		return "", fmt.Errorf("invalid priority %q", priority)
	}
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func buildInitiativeDescription(proposal *core.ThreadProposal) string {
	if proposal == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(proposal.Summary) != "" && strings.TrimSpace(proposal.Content) != "":
		return strings.TrimSpace(proposal.Summary) + "\n\n" + strings.TrimSpace(proposal.Content)
	case strings.TrimSpace(proposal.Content) != "":
		return strings.TrimSpace(proposal.Content)
	default:
		return strings.TrimSpace(proposal.Summary)
	}
}

func buildProposalCreatedMessage(proposal *core.ThreadProposal) string {
	return fmt.Sprintf("系统已创建提案 #%d《%s》草稿。", proposal.ID, proposal.Title)
}

func buildProposalSubmittedMessage(proposal *core.ThreadProposal) string {
	return fmt.Sprintf("提案 #%d《%s》已提交审批。", proposal.ID, proposal.Title)
}

func buildProposalRejectedMessage(proposal *core.ThreadProposal) string {
	if strings.TrimSpace(proposal.ReviewNote) == "" {
		return fmt.Sprintf("提案 #%d《%s》已被驳回。", proposal.ID, proposal.Title)
	}
	return fmt.Sprintf("提案 #%d《%s》已被驳回：%s", proposal.ID, proposal.Title, strings.TrimSpace(proposal.ReviewNote))
}

func buildProposalRevisedMessage(proposal *core.ThreadProposal) string {
	return fmt.Sprintf("提案 #%d《%s》已进入修订状态。", proposal.ID, proposal.Title)
}

func buildProposalMergedMessage(proposal *core.ThreadProposal, initiativeID int64) string {
	return fmt.Sprintf("提案 #%d《%s》已审批并物化为 Initiative #%d。", proposal.ID, proposal.Title, initiativeID)
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func ptr(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	value := strings.TrimSpace(v)
	return &value
}

func fallbackNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
