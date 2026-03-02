package acpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func TestTransportConcurrentCalls(t *testing.T) {
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer clientRead.Close()
	defer clientWrite.Close()
	defer serverRead.Close()
	defer serverWrite.Close()

	transport := NewTransport(clientWrite, clientRead)
	defer func() { _ = transport.Close() }()

	go runEchoServer(t, serverRead, serverWrite)

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := transport.Call(context.Background(), "echo", map[string]int{"value": i})
			if err != nil {
				errCh <- err
				return
			}
			var out struct {
				Value int `json:"value"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				errCh <- err
				return
			}
			if out.Value != i {
				errCh <- errors.New("unexpected echoed value")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent call failed: %v", err)
		}
	}
}

func TestTransportDispatchesNotification(t *testing.T) {
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer clientRead.Close()
	defer clientWrite.Close()
	defer serverRead.Close()
	defer serverWrite.Close()

	transport := NewTransport(clientWrite, clientRead)
	defer func() { _ = transport.Close() }()

	notified := make(chan string, 1)
	transport.SetNotificationHandler(func(_ context.Context, method string, _ json.RawMessage) {
		notified <- method
	})

	if err := writeLineJSON(serverWrite, map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params":  map[string]any{"sessionId": "s1", "type": "agent_message_chunk"},
	}); err != nil {
		t.Fatalf("write notification failed: %v", err)
	}

	select {
	case method := <-notified:
		if method != "session/update" {
			t.Fatalf("expected session/update, got %q", method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification dispatch")
	}
}

func TestTransportDispatchesInboundRequest(t *testing.T) {
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer clientRead.Close()
	defer clientWrite.Close()
	defer serverRead.Close()
	defer serverWrite.Close()

	transport := NewTransport(clientWrite, clientRead)
	defer func() { _ = transport.Close() }()

	transport.SetRequestHandler(func(_ context.Context, method string, _ json.RawMessage) (any, error) {
		if method != "fs/write_file" {
			return nil, errors.New("unexpected method")
		}
		return map[string]any{"ok": true}, nil
	})

	if err := writeLineJSON(serverWrite, map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-1",
		"method":  "fs/write_file",
		"params":  map[string]any{"path": "a.txt", "content": "hello"},
	}); err != nil {
		t.Fatalf("write inbound request failed: %v", err)
	}

	lineCh := make(chan []byte, 1)
	go func() {
		r := bufio.NewReader(serverRead)
		line, _ := r.ReadBytes('\n')
		lineCh <- line
	}()

	select {
	case line := <-lineCh:
		var resp struct {
			ID     string `json:"id"`
			Result struct {
				OK bool `json:"ok"`
			} `json:"result"`
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("decode response failed: %v", err)
		}
		if resp.ID != "req-1" {
			t.Fatalf("expected response id req-1, got %q", resp.ID)
		}
		if resp.Error != nil {
			t.Fatalf("expected success response, got error code=%d", resp.Error.Code)
		}
		if !resp.Result.OK {
			t.Fatal("expected ok=true in response result")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for inbound request response")
	}
}

func runEchoServer(t *testing.T, in io.Reader, out io.Writer) {
	t.Helper()
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		var req struct {
			ID     any               `json:"id"`
			Method string            `json:"method"`
			Params map[string]int    `json:"params"`
			JSONRP string            `json:"jsonrpc"`
			Result map[string]string `json:"result"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			return
		}
		if req.Method == "" {
			continue
		}
		if err := writeLineJSON(out, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"value": req.Params["value"],
			},
		}); err != nil {
			return
		}
	}
}

func writeLineJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}
