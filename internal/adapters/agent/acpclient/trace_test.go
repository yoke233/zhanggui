package acpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

type testTraceRecorder struct {
	mu      sync.Mutex
	records []JSONTraceRecord
}

func (r *testTraceRecorder) RecordJSONTrace(record JSONTraceRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
}

func (r *testTraceRecorder) snapshot() []JSONTraceRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]JSONTraceRecord, len(r.records))
	copy(out, r.records)
	return out
}

func TestTransportTraceRecordsSendAndRecv(t *testing.T) {
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer clientRead.Close()
	defer clientWrite.Close()
	defer serverRead.Close()
	defer serverWrite.Close()

	recorder := &testTraceRecorder{}
	transport := NewTransport(clientWrite, clientRead, newTraceRelay(recorder))
	defer func() { _ = transport.Close() }()

	go func() {
		scanner := bufio.NewScanner(serverRead)
		if !scanner.Scan() {
			return
		}
		var inbound map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &inbound); err != nil {
			return
		}
		_ = writeLineJSON(serverWrite, map[string]any{
			"jsonrpc": "2.0",
			"id":      inbound["id"],
			"result": map[string]any{
				"ok": true,
			},
		})
	}()

	raw, err := transport.Call(context.Background(), "ping", map[string]any{"value": "hello"})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty response payload")
	}

	deadline := time.Now().Add(2 * time.Second)
	for len(recorder.snapshot()) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	records := recorder.snapshot()
	if len(records) < 2 {
		t.Fatalf("expected at least 2 trace records, got %d", len(records))
	}
	if records[0].Direction != TraceDirectionSend {
		t.Fatalf("first record direction = %q, want %q", records[0].Direction, TraceDirectionSend)
	}
	if records[1].Direction != TraceDirectionRecv {
		t.Fatalf("second record direction = %q, want %q", records[1].Direction, TraceDirectionRecv)
	}

	var outbound struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(records[0].JSON, &outbound); err != nil {
		t.Fatalf("decode outbound trace: %v", err)
	}
	if outbound.Method != "ping" {
		t.Fatalf("outbound method = %q, want ping", outbound.Method)
	}

	var inbound struct {
		Result struct {
			OK bool `json:"ok"`
		} `json:"result"`
	}
	if err := json.Unmarshal(records[1].JSON, &inbound); err != nil {
		t.Fatalf("decode inbound trace: %v", err)
	}
	if !inbound.Result.OK {
		t.Fatal("expected inbound trace result.ok=true")
	}
}
