package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type threadMessageLookupStore interface {
	GetThreadMessage(ctx context.Context, id int64) (*core.ThreadMessage, error)
}

type threadMessageInput struct {
	ThreadID         int64
	SenderID         string
	Role             string
	Content          string
	ReplyToMessageID *int64
	Metadata         map[string]any
	TargetAgentID    string
}

type threadMessageAPIError struct {
	Code    string
	Message string
}

func (e *threadMessageAPIError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (h *Handler) ensureThreadParticipant(ctx context.Context, threadID int64, userID string, role string) (*core.ThreadMember, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}

	members, err := h.store.ListThreadMembers(ctx, threadID)
	if err != nil {
		return nil, err
	}
	for _, m := range members {
		if m != nil && m.UserID == userID {
			return m, nil
		}
	}

	member := &core.ThreadMember{
		ThreadID: threadID,
		Kind:     core.ThreadMemberKindHuman,
		UserID:   userID,
		Role:     strings.TrimSpace(role),
	}
	if member.Role == "" {
		member.Role = "member"
	}

	id, err := h.store.AddThreadMember(ctx, member)
	if err != nil {
		return nil, err
	}
	member.ID = id
	return member, nil
}

func (h *Handler) activeThreadAgentParticipantIDs(ctx context.Context, threadID int64) (map[string]bool, error) {
	members, err := h.store.ListThreadMembers(ctx, threadID)
	if err != nil {
		return nil, err
	}

	active := make(map[string]bool)
	for _, m := range members {
		if m == nil {
			continue
		}
		if m.Kind == core.ThreadMemberKindAgent || strings.EqualFold(strings.TrimSpace(m.Role), core.ThreadMemberKindAgent) {
			active[m.UserID] = true
		}
	}
	return active, nil
}

func (h *Handler) validateReplyToThreadMessage(ctx context.Context, threadID int64, replyToMessageID *int64) error {
	if replyToMessageID == nil || *replyToMessageID <= 0 {
		return nil
	}

	if lookupStore, ok := h.store.(threadMessageLookupStore); ok {
		msg, err := lookupStore.GetThreadMessage(ctx, *replyToMessageID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return &threadMessageAPIError{Code: "REPLY_TO_NOT_FOUND", Message: "reply_to_msg_id not found"}
			}
			return err
		}
		if msg.ThreadID != threadID {
			return &threadMessageAPIError{Code: "REPLY_TO_THREAD_MISMATCH", Message: "reply_to_msg_id belongs to another thread"}
		}
		return nil
	}

	offset := 0
	for {
		msgs, err := h.store.ListThreadMessages(ctx, threadID, 200, offset)
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			break
		}
		for _, msg := range msgs {
			if msg != nil && msg.ID == *replyToMessageID {
				return nil
			}
		}
		offset += len(msgs)
	}

	return &threadMessageAPIError{Code: "REPLY_TO_NOT_FOUND", Message: "reply_to_msg_id not found"}
}

func (h *Handler) resolveThreadMessageRecipients(ctx context.Context, thread *core.Thread, targetAgentID string) ([]string, error) {
	targetAgentID = strings.TrimSpace(targetAgentID)
	if h.threadPool == nil {
		if targetAgentID != "" {
			return nil, &threadMessageAPIError{Code: "TARGET_AGENT_UNAVAILABLE", Message: "thread agent runtime is not configured"}
		}
		return nil, nil
	}

	activeProfileIDs := h.threadPool.ActiveAgentProfileIDs(thread.ID)
	activeSet := make(map[string]bool, len(activeProfileIDs))
	for _, profileID := range activeProfileIDs {
		activeSet[profileID] = true
	}

	agentParticipants, err := h.activeThreadAgentParticipantIDs(ctx, thread.ID)
	if err != nil {
		return nil, err
	}
	useParticipantFilter := len(agentParticipants) > 0

	if targetAgentID != "" {
		if !activeSet[targetAgentID] {
			return nil, &threadMessageAPIError{Code: "TARGET_AGENT_UNAVAILABLE", Message: "target agent is not active in this thread"}
		}
		if useParticipantFilter && !agentParticipants[targetAgentID] {
			return nil, &threadMessageAPIError{Code: "TARGET_AGENT_UNAVAILABLE", Message: "target agent is not active in this thread"}
		}
		return []string{targetAgentID}, nil
	}

	routingMode := readThreadAgentRoutingMode(thread)

	if routingMode == "auto" {
		// Auto-routing: match message content against agent capabilities/name/role.
		matched := h.autoRouteMessage(ctx, strings.TrimSpace(targetAgentID), activeProfileIDs, agentParticipants, useParticipantFilter)
		if len(matched) > 0 {
			return matched, nil
		}
		// Fallback: broadcast to all active agents if no match found.
		routingMode = "broadcast"
	}

	if routingMode != "broadcast" {
		return nil, nil
	}

	if !useParticipantFilter {
		return activeProfileIDs, nil
	}

	recipients := make([]string, 0, len(activeProfileIDs))
	for _, profileID := range activeProfileIDs {
		if agentParticipants[profileID] {
			recipients = append(recipients, profileID)
		}
	}
	return recipients, nil
}

// autoRouteMessage picks the best-fit agent(s) based on keyword matching
// between message content and agent profile capabilities, name, and role.
func (h *Handler) autoRouteMessage(ctx context.Context, message string, activeProfileIDs []string, agentParticipants map[string]bool, useParticipantFilter bool) []string {
	if h.registry == nil || message == "" {
		return nil
	}

	messageLower := strings.ToLower(message)
	type scored struct {
		profileID string
		score     int
	}

	var candidates []scored
	for _, profileID := range activeProfileIDs {
		if useParticipantFilter && !agentParticipants[profileID] {
			continue
		}

		profile, err := h.registry.ResolveByID(ctx, profileID)
		if err != nil || profile == nil {
			continue
		}

		score := 0
		// Match against capabilities.
		for _, cap := range profile.Capabilities {
			if strings.Contains(messageLower, strings.ToLower(cap)) {
				score += 3
			}
		}
		// Match against skills.
		for _, skill := range profile.Skills {
			if strings.Contains(messageLower, strings.ToLower(skill)) {
				score += 2
			}
		}
		// Match against agent name.
		if profile.Name != "" && strings.Contains(messageLower, strings.ToLower(profile.Name)) {
			score += 5
		}
		// Match against profile ID.
		if strings.Contains(messageLower, strings.ToLower(profileID)) {
			score += 4
		}
		// Match against role.
		if string(profile.Role) != "" && strings.Contains(messageLower, strings.ToLower(string(profile.Role))) {
			score += 1
		}

		if score > 0 {
			candidates = append(candidates, scored{profileID: profileID, score: score})
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort by score descending, pick the best match(es).
	// If there's a clear winner (score > second place), pick only that one.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	var result []string
	for _, c := range candidates {
		if c.score == best.score {
			result = append(result, c.profileID)
		}
	}
	return result
}

func (h *Handler) createThreadMessageAndRoute(ctx context.Context, input threadMessageInput) (*core.Thread, *core.ThreadMessage, error) {
	thread, err := h.store.GetThread(ctx, input.ThreadID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil, &threadMessageAPIError{Code: "THREAD_NOT_FOUND", Message: "thread not found"}
		}
		return nil, nil, err
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, nil, &threadMessageAPIError{Code: "MISSING_CONTENT", Message: "content is required"}
	}

	if err := h.validateReplyToThreadMessage(ctx, input.ThreadID, input.ReplyToMessageID); err != nil {
		return nil, nil, err
	}

	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = "human"
	}

	var recipients []string
	if role == "human" {
		recipients, err = h.resolveThreadMessageRecipients(ctx, thread, input.TargetAgentID)
		if err != nil {
			return nil, nil, err
		}
	}

	metadata := cloneAnyMap(input.Metadata)
	targetAgentID := strings.TrimSpace(input.TargetAgentID)
	if targetAgentID != "" {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["target_agent_id"] = targetAgentID
	}

	// For auto-routing, record which agents were selected so the frontend can show routing tags.
	isAutoRouted := targetAgentID == "" && len(recipients) > 0 && readThreadAgentRoutingMode(thread) == "auto"
	if isAutoRouted {
		if metadata == nil {
			metadata = map[string]any{}
		}
		routedIDs := make([]any, len(recipients))
		for i, pid := range recipients {
			routedIDs[i] = pid
		}
		metadata["auto_routed_to"] = routedIDs
	}

	message := &core.ThreadMessage{
		ThreadID:         input.ThreadID,
		SenderID:         strings.TrimSpace(input.SenderID),
		Role:             role,
		Content:          content,
		ReplyToMessageID: input.ReplyToMessageID,
		Metadata:         metadata,
	}

	id, err := h.store.CreateThreadMessage(ctx, message)
	if err != nil {
		return nil, nil, err
	}
	message.ID = id

	eventData := map[string]any{
		"thread_id":  message.ThreadID,
		"message_id": message.ID,
		"message":    message.Content,
		"sender_id":  message.SenderID,
		"role":       message.Role,
	}
	if message.ReplyToMessageID != nil {
		eventData["reply_to_msg_id"] = *message.ReplyToMessageID
	}
	if targetAgentID != "" {
		eventData["target_agent_id"] = targetAgentID
	}
	if isAutoRouted {
		routedIDs := make([]any, len(recipients))
		for i, pid := range recipients {
			routedIDs[i] = pid
		}
		eventData["auto_routed_to"] = routedIDs
	}

	h.bus.Publish(ctx, core.Event{
		Type:      core.EventThreadMessage,
		Data:      eventData,
		Timestamp: time.Now().UTC(),
	})

	if message.Role == "human" && h.threadPool != nil {
		for _, profileID := range recipients {
			go func(pid string) {
				// Emit thinking event so frontend can show typing indicator.
				h.bus.Publish(context.Background(), core.Event{
					Type: core.EventThreadAgentThinking,
					Data: map[string]any{
						"thread_id":  message.ThreadID,
						"profile_id": pid,
						"message_id": message.ID,
					},
					Timestamp: time.Now().UTC(),
				})

				routedMessage := stripLeadingThreadMention(message.Content, pid, targetAgentID)
				if sendErr := h.threadPool.SendMessage(context.Background(), message.ThreadID, pid, routedMessage); sendErr != nil {
					h.bus.Publish(context.Background(), core.Event{
						Type: core.EventThreadAgentFailed,
						Data: map[string]any{
							"thread_id":  message.ThreadID,
							"profile_id": pid,
							"error":      sendErr.Error(),
						},
						Timestamp: time.Now().UTC(),
					})
				}
			}(profileID)
		}
	}

	return thread, message, nil
}

func createWorkItemFromThreadDataWithStore(store Store, ctx context.Context, thread *core.Thread, title string, body string, projectID *int64) (*core.WorkItem, error) {
	if thread == nil {
		return nil, errors.New("thread is required")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return nil, &threadMessageAPIError{Code: "MISSING_TITLE", Message: "title is required"}
	}

	body = strings.TrimSpace(body)
	summary := strings.TrimSpace(thread.Summary)
	bodyFromSummary := false
	if body == "" {
		if summary == "" {
			return nil, &threadMessageAPIError{Code: "MISSING_THREAD_SUMMARY", Message: "please generate or fill in summary first"}
		}
		body = summary
		bodyFromSummary = true
	}

	sourceType := "thread_manual"
	if bodyFromSummary {
		sourceType = "thread_summary"
	}

	workItem := &core.WorkItem{
		Title:     title,
		Body:      body,
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
		ProjectID: projectID,
		Metadata: map[string]any{
			"source_thread_id":  thread.ID,
			"source_type":       sourceType,
			"body_from_summary": bodyFromSummary,
		},
	}

	id, err := store.CreateWorkItem(ctx, workItem)
	if err != nil {
		return nil, err
	}
	workItem.ID = id

	link := &core.ThreadWorkItemLink{
		ThreadID:     thread.ID,
		WorkItemID:   id,
		RelationType: "drives",
		IsPrimary:    true,
	}
	if _, err := store.CreateThreadWorkItemLink(ctx, link); err != nil {
		if rollbackErr := store.DeleteWorkItem(ctx, id); rollbackErr != nil {
			return nil, errors.New(err.Error() + "; rollback failed: " + rollbackErr.Error())
		}
		return nil, err
	}

	return workItem, nil
}

func (h *Handler) createWorkItemFromThreadData(ctx context.Context, thread *core.Thread, title string, body string, projectID *int64) (*core.WorkItem, error) {
	return createWorkItemFromThreadDataWithStore(h.store, ctx, thread, title, body, projectID)
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func buildThreadParticipants(ownerID string, memberIDs []string) []*core.ThreadMember {
	participants := make([]*core.ThreadMember, 0, len(memberIDs)+1)
	seen := make(map[string]bool)

	ownerID = strings.TrimSpace(ownerID)
	if ownerID != "" {
		participants = append(participants, &core.ThreadMember{
			Kind:   core.ThreadMemberKindHuman,
			UserID: ownerID,
			Role:   "owner",
		})
		seen[ownerID] = true
	}

	for _, participantID := range memberIDs {
		participantID = strings.TrimSpace(participantID)
		if participantID == "" || seen[participantID] {
			continue
		}
		participants = append(participants, &core.ThreadMember{
			Kind:   core.ThreadMemberKindHuman,
			UserID: participantID,
			Role:   "member",
		})
		seen[participantID] = true
	}

	return participants
}

func threadAgentSessionIsActive(status core.ThreadAgentStatus) bool {
	switch status {
	case core.ThreadAgentJoining, core.ThreadAgentBooting, core.ThreadAgentActive:
		return true
	default:
		return false
	}
}
