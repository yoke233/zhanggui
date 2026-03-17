package api

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type threadMeetingMode string

const (
	threadMeetingModeDirect     threadMeetingMode = "direct"
	threadMeetingModeConcurrent threadMeetingMode = "concurrent"
	threadMeetingModeGroupChat  threadMeetingMode = "group_chat"
)

type threadMeetingTurn struct {
	ProfileID string
	Content   string
	Round     int
}

type threadConcurrentReply struct {
	profileID string
	content   string
	err       error
}

func readThreadMeetingMode(thread *core.Thread) threadMeetingMode {
	if thread == nil || thread.Metadata == nil {
		return threadMeetingModeDirect
	}
	value, _ := thread.Metadata["meeting_mode"].(string)
	switch strings.TrimSpace(value) {
	case string(threadMeetingModeConcurrent):
		return threadMeetingModeConcurrent
	case string(threadMeetingModeGroupChat):
		return threadMeetingModeGroupChat
	default:
		return threadMeetingModeDirect
	}
}

func readThreadMeetingMaxRounds(thread *core.Thread) int {
	if thread == nil || thread.Metadata == nil {
		return 3
	}
	switch value := thread.Metadata["meeting_max_rounds"].(type) {
	case int:
		if value > 0 {
			return minInt(value, 12)
		}
	case int64:
		if value > 0 {
			return minInt(int(value), 12)
		}
	case float64:
		if value > 0 {
			return minInt(int(value), 12)
		}
	}
	return 3
}

func readThreadMeetingSelector(thread *core.Thread) string {
	if thread == nil || thread.Metadata == nil {
		return "round_robin"
	}
	value, _ := thread.Metadata["meeting_selector"].(string)
	value = strings.TrimSpace(value)
	if value == "" {
		return "round_robin"
	}
	return value
}

func (h *Handler) dispatchThreadAgentWork(thread *core.Thread, message *core.ThreadMessage, recipients []string, targetAgentID string) {
	if h == nil || h.threadPool == nil || thread == nil || message == nil || len(recipients) == 0 {
		return
	}

	normalized := sortedProfileIDs(recipients)
	mode := readThreadMeetingMode(thread)
	if targetAgentID != "" || len(normalized) <= 1 || mode == threadMeetingModeDirect {
		for _, profileID := range normalized {
			go h.runDirectThreadDispatch(thread, message, profileID, targetAgentID)
		}
		return
	}

	go func(profileIDs []string) {
		switch mode {
		case threadMeetingModeConcurrent:
			h.runConcurrentMeeting(context.Background(), thread, message, profileIDs)
		case threadMeetingModeGroupChat:
			h.runGroupChatMeeting(context.Background(), thread, message, profileIDs)
		default:
			for _, profileID := range profileIDs {
				h.runDirectThreadDispatch(thread, message, profileID, targetAgentID)
			}
		}
	}(normalized)
}

func (h *Handler) runDirectThreadDispatch(thread *core.Thread, message *core.ThreadMessage, profileID string, targetAgentID string) {
	if h == nil || h.threadPool == nil || thread == nil || message == nil {
		return
	}
	ctx := context.Background()
	h.publishThreadThinking(ctx, message.ThreadID, profileID, message.ID)
	routedMessage := stripLeadingThreadMention(message.Content, profileID, targetAgentID)
	routedMessage = enrichMessageWithFileRefs(routedMessage, message.Metadata)
	if sendErr := h.threadPool.SendMessage(ctx, message.ThreadID, profileID, routedMessage); sendErr != nil {
		h.publishThreadAgentFailure(ctx, message.ThreadID, profileID, sendErr)
	}
}

func (h *Handler) runConcurrentMeeting(ctx context.Context, thread *core.Thread, source *core.ThreadMessage, profileIDs []string) {
	runID := fmt.Sprintf("meeting-%d-%d", thread.ID, source.ID)
	promptBase := buildConcurrentMeetingPrompt(source, profileIDs)

	results := make([]threadConcurrentReply, len(profileIDs))
	var wg sync.WaitGroup

	for index, profileID := range profileIDs {
		wg.Add(1)
		go func(i int, pid string) {
			defer wg.Done()
			h.publishThreadThinking(ctx, source.ThreadID, pid, source.ID)
			reply, err := h.threadPool.PromptAgent(ctx, source.ThreadID, pid, promptBase)
			if err != nil {
				results[i] = threadConcurrentReply{profileID: pid, err: err}
				h.publishThreadAgentFailure(ctx, source.ThreadID, pid, err)
				return
			}
			content := ""
			if reply != nil {
				content = strings.TrimSpace(reply.Content)
			}
			results[i] = threadConcurrentReply{profileID: pid, content: content}
			if content == "" {
				return
			}
			metadata := map[string]any{
				"meeting_mode":   string(threadMeetingModeConcurrent),
				"meeting_run_id": runID,
				"meeting_round":  1,
			}
			if _, err := h.persistThreadAgentMessage(ctx, source.ThreadID, pid, content, metadata); err != nil {
				h.publishThreadAgentFailure(ctx, source.ThreadID, pid, err)
				return
			}
			h.publishThreadAgentOutput(ctx, source.ThreadID, pid, content, metadata)
		}(index, profileID)
	}
	wg.Wait()

	summary := buildConcurrentMeetingSummary(results)
	if strings.TrimSpace(summary) == "" {
		return
	}
	_, _ = h.persistAndPublishThreadSystemMessage(ctx, source.ThreadID, summary, map[string]any{
		"type":           "meeting_summary",
		"meeting_mode":   string(threadMeetingModeConcurrent),
		"meeting_run_id": runID,
		"meeting_status": "completed",
	})
}

func (h *Handler) runGroupChatMeeting(ctx context.Context, thread *core.Thread, source *core.ThreadMessage, profileIDs []string) {
	runID := fmt.Sprintf("meeting-%d-%d", thread.ID, source.ID)
	maxRounds := readThreadMeetingMaxRounds(thread)
	selector := readThreadMeetingSelector(thread)
	turns := make([]threadMeetingTurn, 0, maxRounds)
	stopReason := "max rounds reached"

	for round := 1; round <= maxRounds; round++ {
		speakerID := selectGroupChatSpeaker(selector, profileIDs, round)
		if speakerID == "" {
			stopReason = "no available speaker"
			break
		}
		h.publishThreadThinking(ctx, source.ThreadID, speakerID, source.ID)
		prompt := buildGroupChatMeetingPrompt(source, profileIDs, turns, round, maxRounds, speakerID)
		reply, err := h.threadPool.PromptAgent(ctx, source.ThreadID, speakerID, prompt)
		if err != nil {
			h.publishThreadAgentFailure(ctx, source.ThreadID, speakerID, err)
			stopReason = fmt.Sprintf("%s failed", speakerID)
			break
		}
		content := ""
		if reply != nil {
			content = strings.TrimSpace(reply.Content)
		}
		if content == "" {
			continue
		}
		isFinal, cleaned := extractFinalReply(content)
		turns = append(turns, threadMeetingTurn{
			ProfileID: speakerID,
			Content:   cleaned,
			Round:     round,
		})
		metadata := map[string]any{
			"meeting_mode":     string(threadMeetingModeGroupChat),
			"meeting_run_id":   runID,
			"meeting_round":    round,
			"meeting_selector": selector,
		}
		if _, err := h.persistThreadAgentMessage(ctx, source.ThreadID, speakerID, cleaned, metadata); err != nil {
			h.publishThreadAgentFailure(ctx, source.ThreadID, speakerID, err)
			stopReason = fmt.Sprintf("%s persist failed", speakerID)
			break
		}
		h.publishThreadAgentOutput(ctx, source.ThreadID, speakerID, cleaned, metadata)
		if isFinal {
			stopReason = fmt.Sprintf("%s declared final", speakerID)
			break
		}
	}

	summary := buildGroupChatMeetingSummary(turns, selector, stopReason)
	if strings.TrimSpace(summary) == "" {
		return
	}
	_, _ = h.persistAndPublishThreadSystemMessage(ctx, source.ThreadID, summary, map[string]any{
		"type":             "meeting_summary",
		"meeting_mode":     string(threadMeetingModeGroupChat),
		"meeting_run_id":   runID,
		"meeting_selector": selector,
		"meeting_status":   "completed",
		"meeting_rounds":   len(turns),
		"stop_reason":      stopReason,
	})
}

func buildConcurrentMeetingPrompt(source *core.ThreadMessage, profileIDs []string) string {
	var b strings.Builder
	b.WriteString("你正在参加 thread 内的并行会议。\n")
	b.WriteString("会议模式：concurrent\n")
	fmt.Fprintf(&b, "参与者：%s\n\n", strings.Join(profileIDs, ", "))
	b.WriteString("请独立分析最新消息，给出你的结论、建议或风险判断。\n")
	b.WriteString("不要假设自己能看到其他 agent 的本轮发言。\n")
	b.WriteString("请直接输出你的发言内容。\n\n")
	b.WriteString("最新消息：\n")
	b.WriteString(enrichMessageWithFileRefs(source.Content, source.Metadata))
	return b.String()
}

func buildGroupChatMeetingPrompt(source *core.ThreadMessage, profileIDs []string, turns []threadMeetingTurn, round int, maxRounds int, speakerID string) string {
	var b strings.Builder
	b.WriteString("你正在参加 thread 内的主持人会议。\n")
	b.WriteString("会议模式：group_chat\n")
	fmt.Fprintf(&b, "当前发言人：%s\n", speakerID)
	fmt.Fprintf(&b, "当前轮次：%d/%d\n", round, maxRounds)
	fmt.Fprintf(&b, "参与者：%s\n\n", strings.Join(profileIDs, ", "))
	b.WriteString("请基于最新消息和前面轮次的发言继续推进讨论，尽量避免重复。\n")
	b.WriteString("如果你认为讨论已经收敛，可以在回复开头写 [FINAL]。\n\n")
	b.WriteString("最新消息：\n")
	b.WriteString(enrichMessageWithFileRefs(source.Content, source.Metadata))
	if len(turns) > 0 {
		b.WriteString("\n\n本次会议已有发言：\n")
		for _, turn := range turns {
			fmt.Fprintf(&b, "- 第 %d 轮 %s：%s\n", turn.Round, turn.ProfileID, compactMeetingReply(turn.Content, 240))
		}
	}
	return b.String()
}

func buildConcurrentMeetingSummary(results []threadConcurrentReply) string {
	if len(results) == 0 {
		return ""
	}
	var lines []string
	for _, item := range results {
		switch {
		case item.err != nil:
			lines = append(lines, fmt.Sprintf("- %s：失败（%s）", item.profileID, item.err.Error()))
		case strings.TrimSpace(item.content) != "":
			lines = append(lines, fmt.Sprintf("- %s：%s", item.profileID, compactMeetingReply(item.content, 160)))
		default:
			lines = append(lines, fmt.Sprintf("- %s：未返回有效内容", item.profileID))
		}
	}
	return "并行会议已完成，汇总如下：\n" + strings.Join(lines, "\n")
}

func buildGroupChatMeetingSummary(turns []threadMeetingTurn, selector string, stopReason string) string {
	if len(turns) == 0 {
		return ""
	}
	lines := []string{
		fmt.Sprintf("主持人会议已完成，选择器：%s。", selector),
		fmt.Sprintf("停止原因：%s。", stopReason),
	}
	for _, turn := range turns {
		lines = append(lines, fmt.Sprintf("- 第 %d 轮 %s：%s", turn.Round, turn.ProfileID, compactMeetingReply(turn.Content, 160)))
	}
	return strings.Join(lines, "\n")
}

func selectGroupChatSpeaker(selector string, profileIDs []string, round int) string {
	if len(profileIDs) == 0 {
		return ""
	}
	switch selector {
	case "", "round_robin":
		return profileIDs[(round-1)%len(profileIDs)]
	default:
		return profileIDs[(round-1)%len(profileIDs)]
	}
}

func extractFinalReply(content string) (bool, string) {
	trimmed := strings.TrimSpace(content)
	upper := strings.ToUpper(trimmed)
	switch {
	case strings.HasPrefix(upper, "[FINAL]"):
		return true, strings.TrimSpace(trimmed[len("[FINAL]"):])
	case strings.HasPrefix(upper, "FINAL:"):
		return true, strings.TrimSpace(trimmed[len("FINAL:"):])
	default:
		return false, trimmed
	}
}

func compactMeetingReply(content string, limit int) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if limit <= 0 || len(normalized) <= limit {
		return normalized
	}
	if limit <= 3 {
		return normalized[:limit]
	}
	return normalized[:limit-3] + "..."
}

func sortedProfileIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func (h *Handler) publishThreadThinking(ctx context.Context, threadID int64, profileID string, messageID int64) {
	if h == nil || h.bus == nil {
		return
	}
	h.bus.Publish(ctx, core.Event{
		Type: core.EventThreadAgentThinking,
		Data: map[string]any{
			"thread_id":  threadID,
			"profile_id": profileID,
			"message_id": messageID,
		},
		Timestamp: time.Now().UTC(),
	})
}

func (h *Handler) publishThreadAgentFailure(ctx context.Context, threadID int64, profileID string, err error) {
	if h == nil || h.bus == nil || err == nil {
		return
	}
	h.bus.Publish(ctx, core.Event{
		Type: core.EventThreadAgentFailed,
		Data: map[string]any{
			"thread_id":  threadID,
			"profile_id": profileID,
			"error":      err.Error(),
		},
		Timestamp: time.Now().UTC(),
	})
}

func (h *Handler) publishThreadAgentOutput(ctx context.Context, threadID int64, profileID string, content string, metadata map[string]any) {
	if h == nil || h.bus == nil || strings.TrimSpace(content) == "" {
		return
	}
	data := map[string]any{
		"thread_id":  threadID,
		"profile_id": profileID,
		"content":    content,
	}
	if len(metadata) > 0 {
		data["metadata"] = cloneAnyMap(metadata)
	}
	h.bus.Publish(ctx, core.Event{
		Type:      core.EventThreadAgentOutput,
		Data:      data,
		Timestamp: time.Now().UTC(),
	})
}

func (h *Handler) persistThreadAgentMessage(ctx context.Context, threadID int64, profileID string, content string, metadata map[string]any) (*core.ThreadMessage, error) {
	if h == nil || h.store == nil {
		return nil, fmt.Errorf("thread store is not configured")
	}
	msg := &core.ThreadMessage{
		ThreadID: threadID,
		SenderID: profileID,
		Role:     "agent",
		Content:  strings.TrimSpace(content),
		Metadata: cloneAnyMap(metadata),
	}
	id, err := h.store.CreateThreadMessage(ctx, msg)
	if err != nil {
		return nil, err
	}
	msg.ID = id
	return msg, nil
}

func (h *Handler) persistAndPublishThreadSystemMessage(ctx context.Context, threadID int64, content string, metadata map[string]any) (*core.ThreadMessage, error) {
	if h == nil || h.store == nil {
		return nil, fmt.Errorf("thread store is not configured")
	}
	msg := &core.ThreadMessage{
		ThreadID: threadID,
		SenderID: "system",
		Role:     "system",
		Content:  strings.TrimSpace(content),
		Metadata: cloneAnyMap(metadata),
	}
	id, err := h.store.CreateThreadMessage(ctx, msg)
	if err != nil {
		slog.Warn("thread meeting: persist system summary failed", "thread_id", threadID, "error", err)
		return nil, err
	}
	msg.ID = id

	eventData := map[string]any{
		"thread_id":  msg.ThreadID,
		"message_id": msg.ID,
		"message":    msg.Content,
		"content":    msg.Content,
		"sender_id":  msg.SenderID,
		"role":       msg.Role,
	}
	if len(msg.Metadata) > 0 {
		eventData["metadata"] = cloneAnyMap(msg.Metadata)
	}
	h.bus.Publish(ctx, core.Event{
		Type:      core.EventThreadMessage,
		Data:      eventData,
		Timestamp: time.Now().UTC(),
	})
	return msg, nil
}
