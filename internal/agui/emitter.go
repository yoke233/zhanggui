package agui

import (
	"strings"
	"time"

	a2ago "github.com/a2aproject/a2a-go/a2a"
	a2astore "github.com/yoke233/zhanggui/internal/a2a"
)

type Emitter struct {
	Stream   *SSEStream
	EventLog *EventLog
	ThreadID string
	RunID    string
	Core     *a2astore.Store

	messageBufs map[string]*messageBuf
}

func (e *Emitter) Emit(ev Event) error {
	if ev == nil {
		ev = Event{}
	}
	if _, ok := ev["timestamp"]; !ok {
		ev["timestamp"] = time.Now().UnixMilli()
	}
	if e.ThreadID != "" {
		if _, ok := ev["threadId"]; !ok {
			ev["threadId"] = e.ThreadID
		}
	}
	if e.RunID != "" {
		if _, ok := ev["runId"]; !ok {
			ev["runId"] = e.RunID
		}
	}

	e.applyCore(ev)

	if e.EventLog != nil {
		if err := e.EventLog.Append(ev); err != nil {
			return err
		}
	}
	return e.Stream.SendJSON(ev)
}

type messageBuf struct {
	role string
	sb   strings.Builder
}

func (e *Emitter) applyCore(ev Event) {
	if e.Core == nil {
		return
	}
	typ := firstString(ev, "type")
	if typ == "" {
		return
	}

	runID := firstString(ev, "runId")
	if runID == "" {
		runID = e.RunID
	}
	threadID := firstString(ev, "threadId")
	if threadID == "" {
		threadID = e.ThreadID
	}

	switch typ {
	case "RUN_STARTED":
		e.Core.EnsureTask(runID, threadID)
		now := time.Now().UTC()
		e.Core.UpdateTaskStatus(runID, threadID, a2ago.TaskStatus{
			State:     a2ago.TaskStateWorking,
			Timestamp: &now,
		})
	case "RUN_FINISHED":
		outcome := strings.TrimSpace(firstString(ev, "outcome"))
		state := a2ago.TaskStateCompleted
		switch outcome {
		case "interrupt":
			state = a2ago.TaskStateInputRequired
		case "error":
			state = a2ago.TaskStateFailed
		}
		now := time.Now().UTC()
		e.Core.UpdateTaskStatus(runID, threadID, a2ago.TaskStatus{
			State:     state,
			Timestamp: &now,
		})
	case "RUN_ERROR":
		now := time.Now().UTC()
		e.Core.UpdateTaskStatus(runID, threadID, a2ago.TaskStatus{
			State:     a2ago.TaskStateFailed,
			Timestamp: &now,
		})
	case "activity_message":
		content, _ := ev["content"].(map[string]any)
		if content != nil {
			e.Core.AppendActivity(runID, threadID, content)
		}
	}

	e.trackMessage(ev, runID, threadID)
}

func (e *Emitter) trackMessage(ev Event, runID string, threadID string) {
	typ := firstString(ev, "type")
	if typ == "" {
		return
	}
	messageID := firstString(ev, "messageId")
	if messageID == "" {
		return
	}
	if e.messageBufs == nil {
		e.messageBufs = map[string]*messageBuf{}
	}
	switch typ {
	case "TEXT_MESSAGE_START":
		role := strings.TrimSpace(firstString(ev, "role"))
		e.messageBufs[messageID] = &messageBuf{role: role}
	case "TEXT_MESSAGE_CONTENT":
		buf := e.messageBufs[messageID]
		if buf == nil {
			buf = &messageBuf{}
			e.messageBufs[messageID] = buf
		}
		delta := firstString(ev, "delta")
		if delta != "" {
			buf.sb.WriteString(delta)
		}
	case "TEXT_MESSAGE_END":
		buf := e.messageBufs[messageID]
		if buf == nil {
			return
		}
		content := strings.TrimSpace(buf.sb.String())
		if content == "" {
			delete(e.messageBufs, messageID)
			return
		}
		msg := &a2ago.Message{
			ID:        messageID,
			Role:      normalizeRole(buf.role),
			Parts:     a2ago.ContentParts{a2ago.TextPart{Text: content}},
			ContextID: threadID,
			TaskID:    a2ago.TaskID(runID),
		}
		e.Core.AppendMessage(runID, threadID, msg)
		delete(e.messageBufs, messageID)
	}
}

func normalizeRole(role string) a2ago.MessageRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user", "role_user":
		return a2ago.MessageRoleUser
	case "assistant", "agent", "system", "role_agent":
		return a2ago.MessageRoleAgent
	default:
		return a2ago.MessageRoleUnspecified
	}
}
