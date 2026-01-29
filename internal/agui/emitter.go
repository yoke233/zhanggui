package agui

import (
	"time"
)

type Emitter struct {
	Stream   *SSEStream
	EventLog *EventLog
	ThreadID string
	RunID    string
}

func (e *Emitter) Emit(ev Event) error {
	if ev == nil {
		ev = Event{}
	}
	if _, ok := ev["timestamp"]; !ok {
		ev["timestamp"] = time.Now().Format(time.RFC3339)
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
	if e.EventLog != nil {
		if err := e.EventLog.Append(ev); err != nil {
			return err
		}
	}
	return e.Stream.SendJSON(ev)
}
