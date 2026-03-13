package acpclient

import (
	"encoding/json"
	"sync/atomic"
	"time"
)

type TraceDirection string

const (
	TraceDirectionSend TraceDirection = "send"
	TraceDirectionRecv TraceDirection = "recv"
)

type JSONTraceRecord struct {
	Sequence  int64           `json:"sequence"`
	Timestamp string          `json:"timestamp"`
	OffsetMs  int64           `json:"offset_ms"`
	Direction TraceDirection  `json:"direction"`
	JSON      json.RawMessage `json:"json"`
}

type TraceRecorder interface {
	RecordJSONTrace(JSONTraceRecord)
}

type traceRelay struct {
	start    time.Time
	sequence atomic.Int64
	recorder TraceRecorder
}

func newTraceRelay(recorder TraceRecorder) *traceRelay {
	if recorder == nil {
		return nil
	}
	return &traceRelay{
		start:    time.Now(),
		recorder: recorder,
	}
}

func (r *traceRelay) record(direction TraceDirection, payload []byte) {
	if r == nil || r.recorder == nil || len(payload) == 0 {
		return
	}
	cloned := make([]byte, len(payload))
	copy(cloned, payload)
	r.recorder.RecordJSONTrace(JSONTraceRecord{
		Sequence:  r.sequence.Add(1),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		OffsetMs:  time.Since(r.start).Milliseconds(),
		Direction: direction,
		JSON:      json.RawMessage(cloned),
	})
}
