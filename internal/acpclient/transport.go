package acpclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
)

type NotificationHandler func(ctx context.Context, method string, params json.RawMessage)

type RequestHandler func(ctx context.Context, method string, params json.RawMessage) (any, error)

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("rpc error(code=%d): %s", e.Code, e.Message)
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type pendingResponse struct {
	result json.RawMessage
	err    error
}

type Transport struct {
	writer io.WriteCloser
	reader *bufio.Scanner

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan pendingResponse
	nextID    atomic.Int64

	handlerMu           sync.RWMutex
	notificationHandler NotificationHandler
	requestHandler      RequestHandler

	readErrMu sync.Mutex
	readErr   error

	done     chan struct{}
	doneOnce sync.Once
}

func NewTransport(writer io.WriteCloser, reader io.Reader) *Transport {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	t := &Transport{
		writer:  writer,
		reader:  scanner,
		pending: make(map[string]chan pendingResponse),
		done:    make(chan struct{}),
	}
	go t.readLoop()
	return t
}

func (t *Transport) SetNotificationHandler(handler NotificationHandler) {
	t.handlerMu.Lock()
	defer t.handlerMu.Unlock()
	t.notificationHandler = handler
}

func (t *Transport) SetRequestHandler(handler RequestHandler) {
	t.handlerMu.Lock()
	defer t.handlerMu.Unlock()
	t.requestHandler = handler
}

func (t *Transport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if method == "" {
		return nil, errors.New("method is required")
	}
	if err := t.ensureAlive(); err != nil {
		return nil, err
	}

	id := strconv.FormatInt(t.nextID.Add(1), 10)
	waitCh := make(chan pendingResponse, 1)
	t.pendingMu.Lock()
	t.pending[id] = waitCh
	t.pendingMu.Unlock()
	defer t.deletePending(id)

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	if err := t.writeMessage(msg); err != nil {
		return nil, err
	}

	select {
	case resp := <-waitCh:
		return resp.result, resp.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.done:
		if err := t.getReadErr(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
}

func (t *Transport) Notify(method string, params any) error {
	if method == "" {
		return errors.New("method is required")
	}
	if err := t.ensureAlive(); err != nil {
		return err
	}
	return t.writeMessage(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func (t *Transport) Close() error {
	t.shutdown(io.EOF)
	if t.writer == nil {
		return nil
	}
	return t.writer.Close()
}

func (t *Transport) readLoop() {
	for t.reader.Scan() {
		line := bytes.TrimSpace(t.reader.Bytes())
		if len(line) == 0 {
			continue
		}
		var env rpcEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			t.shutdown(fmt.Errorf("decode rpc envelope: %w", err))
			return
		}
		t.dispatchEnvelope(env)
	}
	if err := t.reader.Err(); err != nil {
		t.shutdown(err)
		return
	}
	t.shutdown(io.EOF)
}

func (t *Transport) dispatchEnvelope(env rpcEnvelope) {
	if env.Method != "" {
		if len(env.ID) == 0 {
			t.dispatchNotification(env.Method, env.Params)
			return
		}
		go t.dispatchInboundRequest(env)
		return
	}
	if len(env.ID) == 0 {
		return
	}
	id, err := normalizeID(env.ID)
	if err != nil {
		return
	}
	t.pendingMu.Lock()
	ch, ok := t.pending[id]
	t.pendingMu.Unlock()
	if !ok {
		return
	}

	resp := pendingResponse{result: env.Result}
	if env.Error != nil {
		resp.err = env.Error
	}
	select {
	case ch <- resp:
	default:
	}
}

func (t *Transport) dispatchNotification(method string, params json.RawMessage) {
	t.handlerMu.RLock()
	handler := t.notificationHandler
	t.handlerMu.RUnlock()
	if handler == nil {
		return
	}
	handler(context.Background(), method, params)
}

func (t *Transport) dispatchInboundRequest(env rpcEnvelope) {
	idValue, err := decodeIDValue(env.ID)
	if err != nil {
		return
	}

	t.handlerMu.RLock()
	handler := t.requestHandler
	t.handlerMu.RUnlock()
	if handler == nil {
		_ = t.writeMessage(map[string]any{
			"jsonrpc": "2.0",
			"id":      idValue,
			"error": map[string]any{
				"code":    -32601,
				"message": "method not found",
			},
		})
		return
	}

	result, callErr := handler(context.Background(), env.Method, env.Params)
	if callErr != nil {
		_ = t.writeMessage(map[string]any{
			"jsonrpc": "2.0",
			"id":      idValue,
			"error": map[string]any{
				"code":    -32000,
				"message": callErr.Error(),
			},
		})
		return
	}
	_ = t.writeMessage(map[string]any{
		"jsonrpc": "2.0",
		"id":      idValue,
		"result":  result,
	})
}

func (t *Transport) writeMessage(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if bytes.Contains(data, []byte{'\n'}) {
		return errors.New("rpc message contains newline")
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if t.writer == nil {
		return io.ErrClosedPipe
	}
	if _, err := t.writer.Write(data); err != nil {
		return err
	}
	_, err = t.writer.Write([]byte{'\n'})
	return err
}

func (t *Transport) deletePending(id string) {
	t.pendingMu.Lock()
	defer t.pendingMu.Unlock()
	delete(t.pending, id)
}

func (t *Transport) ensureAlive() error {
	select {
	case <-t.done:
		if err := t.getReadErr(); err != nil {
			return err
		}
		return io.EOF
	default:
		return nil
	}
}

func (t *Transport) shutdown(err error) {
	t.doneOnce.Do(func() {
		t.readErrMu.Lock()
		t.readErr = err
		t.readErrMu.Unlock()
		close(t.done)
	})
}

func (t *Transport) getReadErr() error {
	t.readErrMu.Lock()
	defer t.readErrMu.Unlock()
	return t.readErr
}

func decodeIDValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, errors.New("missing id")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func normalizeID(raw json.RawMessage) (string, error) {
	v, err := decodeIDValue(raw)
	if err != nil {
		return "", err
	}
	switch id := v.(type) {
	case string:
		return id, nil
	case float64:
		return strconv.FormatInt(int64(id), 10), nil
	default:
		return fmt.Sprintf("%v", id), nil
	}
}
