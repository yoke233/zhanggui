package api

import (
	"context"
	"errors"
	"fmt"
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
		if (m.Kind == core.ThreadMemberKindAgent || strings.EqualFold(strings.TrimSpace(m.Role), core.ThreadMemberKindAgent)) && threadAgentSessionIsActive(m.Status) {
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

func (h *Handler) resolveThreadMessageRecipients(ctx context.Context, thread *core.Thread, message string, targetAgentID string) ([]string, error) {
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
		matched := h.autoRouteMessage(ctx, strings.TrimSpace(message), activeProfileIDs, agentParticipants, useParticipantFilter)
		if len(matched) > 0 {
			return matched, nil
		}
		// Fallback: broadcast to all active agents if no match found.
		routingMode = "broadcast"
	}

	if routingMode != "broadcast" {
		return nil, nil
	}

	// Broadcast: send to all agents that are known participants (DB status
	// joining/booting/active). We use the DB participant list rather than
	// the in-memory session pool so that agents that just finished booting
	// (async) are not missed. If a session doesn't exist yet, SendMessage
	// will fail gracefully and publish EventThreadAgentFailed.
	if useParticipantFilter {
		recipients := make([]string, 0, len(agentParticipants))
		for profileID := range agentParticipants {
			recipients = append(recipients, profileID)
		}
		return recipients, nil
	}
	return activeProfileIDs, nil
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
		isBroadcast, _ := input.Metadata["broadcast"].(bool)
		if isBroadcast && h.threadPool != nil {
			recipients = h.threadPool.ActiveAgentProfileIDs(thread.ID)
		} else {
			recipients, err = h.resolveThreadMessageRecipients(ctx, thread, content, input.TargetAgentID)
			if err != nil {
				return nil, nil, err
			}
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
		"content":    message.Content,
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
	if len(message.Metadata) > 0 {
		eventData["metadata"] = cloneAnyMap(message.Metadata)
	}

	h.bus.Publish(ctx, core.Event{
		Type:      core.EventThreadMessage,
		Data:      eventData,
		Timestamp: time.Now().UTC(),
	})

	if message.Role == "human" && h.threadPool != nil {
		h.dispatchThreadAgentWork(thread, message, recipients, targetAgentID)
	}

	return thread, message, nil
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

// enrichMessageWithFileRefs prepends a structured file reference section to
// the message content when file_refs exist in metadata. This gives the agent
// clear, actionable file paths relative to its cwd.
func enrichMessageWithFileRefs(content string, metadata map[string]any) string {
	if metadata == nil {
		return content
	}
	refsRaw, ok := metadata["file_refs"]
	if !ok {
		return content
	}
	refs, ok := refsRaw.([]any)
	if !ok || len(refs) == 0 {
		return content
	}

	var b strings.Builder
	b.WriteString("引用文件：\n")
	for _, r := range refs {
		ref, ok := r.(map[string]any)
		if !ok {
			continue
		}
		name, _ := ref["name"].(string)
		path, _ := ref["path"].(string)
		if name == "" && path == "" {
			continue
		}
		if name == "" {
			name = path
		}
		fmt.Fprintf(&b, "- %s → %s\n", name, path)
	}
	b.WriteString("\n")
	b.WriteString(content)
	return b.String()
}

func threadAgentSessionIsActive(status core.ThreadAgentStatus) bool {
	switch status {
	case core.ThreadAgentJoining, core.ThreadAgentBooting, core.ThreadAgentActive:
		return true
	default:
		return false
	}
}
