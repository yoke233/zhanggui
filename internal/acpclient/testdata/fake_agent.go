package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

type envelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type server struct {
	r        *bufio.Reader
	w        io.Writer
	sessions map[string]struct{}
	seq      int
}

func main() {
	s := &server{
		r:        bufio.NewReader(os.Stdin),
		w:        os.Stdout,
		sessions: map[string]struct{}{},
	}
	if err := s.run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func (s *server) run() error {
	for {
		msg, err := s.read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if msg.Method == "" {
			continue
		}
		switch msg.Method {
		case "initialize":
			if err := s.replyResult(msg.ID, map[string]any{
				"protocolVersion": 1,
				"agentInfo": map[string]any{
					"name":    "fake-acp-agent",
					"title":   "Fake ACP Agent",
					"version": "0.1.0",
				},
			}); err != nil {
				return err
			}
		case "session/new":
			s.seq++
			sessionID := "fake-session-" + strconv.Itoa(s.seq)
			s.sessions[sessionID] = struct{}{}
			if err := s.replyResult(msg.ID, map[string]any{
				"sessionId": sessionID,
			}); err != nil {
				return err
			}
		case "session/load":
			var req struct {
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(msg.Params, &req)
			if _, ok := s.sessions[req.SessionID]; !ok {
				if err := s.replyError(msg.ID, -32004, "session not found"); err != nil {
					return err
				}
				continue
			}
			if err := s.replyResult(msg.ID, map[string]any{
				"sessionId": req.SessionID,
			}); err != nil {
				return err
			}
		case "session/prompt":
			if err := s.handlePrompt(msg); err != nil {
				return err
			}
		case "session/cancel":
			// ignore notification in fake agent
		default:
			if len(msg.ID) > 0 {
				if err := s.replyError(msg.ID, -32601, "method not found"); err != nil {
					return err
				}
			}
		}
	}
}

func (s *server) handlePrompt(msg envelope) error {
	var req struct {
		SessionID string `json:"sessionId"`
		Prompt    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"prompt"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(msg.Params, &req); err != nil {
		return s.replyError(msg.ID, -32602, "invalid prompt params")
	}
	if _, ok := s.sessions[req.SessionID]; !ok {
		return s.replyError(msg.ID, -32004, "session not found")
	}

	if err := s.requestPermission(); err != nil {
		return err
	}
	if err := s.requestWriteFile(); err != nil {
		return err
	}

	role := req.Metadata["role_id"]
	text := "FAKE_REPLY"
	if role != "" {
		text = text + " role=" + role
	}
	if len(req.Prompt) > 0 && req.Prompt[0].Text != "" {
		text = text + " prompt=" + req.Prompt[0].Text
	}

	if err := s.notify(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": req.SessionID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": text,
				},
			},
		},
	}); err != nil {
		return err
	}

	return s.replyResult(msg.ID, map[string]any{
		"requestId":  "req-1",
		"stopReason": "end_turn",
		"usage": map[string]any{
			"inputTokens":  10,
			"outputTokens": 5,
			"totalTokens":  15,
		},
	})
}

func (s *server) requestPermission() error {
	requestID := "perm-1"
	if err := s.notify(map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  "session/request_permission",
		"params": map[string]any{
			"action": "write_file",
			"reason": "integration test",
		},
	}); err != nil {
		return err
	}
	return s.waitForResponseID(requestID)
}

func (s *server) requestWriteFile() error {
	toolID := "tool-1"
	if err := s.notify(map[string]any{
		"jsonrpc": "2.0",
		"id":      toolID,
		"method":  "fs/write_file",
		"params": map[string]any{
			"path":    "from-fake.txt",
			"content": "payload",
		},
	}); err != nil {
		return err
	}
	return s.waitForResponseID(toolID)
}

func (s *server) waitForResponseID(wantID string) error {
	for {
		msg, err := s.read()
		if err != nil {
			return err
		}
		if msg.Method != "" {
			continue
		}
		if string(msg.ID) == "" {
			continue
		}
		id, err := normalizeID(msg.ID)
		if err != nil {
			return err
		}
		if id != wantID {
			continue
		}
		if msg.Error != nil {
			return fmt.Errorf("request %s failed: %s", wantID, msg.Error.Message)
		}
		return nil
	}
}

func (s *server) read() (envelope, error) {
	line, err := s.r.ReadBytes('\n')
	if err != nil {
		return envelope{}, err
	}
	var msg envelope
	if err := json.Unmarshal(line, &msg); err != nil {
		return envelope{}, err
	}
	return msg, nil
}

func (s *server) notify(v any) error {
	return s.write(v)
}

func (s *server) replyResult(id json.RawMessage, result any) error {
	v, err := decodeID(id)
	if err != nil {
		return err
	}
	return s.write(map[string]any{
		"jsonrpc": "2.0",
		"id":      v,
		"result":  result,
	})
}

func (s *server) replyError(id json.RawMessage, code int, message string) error {
	v, err := decodeID(id)
	if err != nil {
		return err
	}
	return s.write(map[string]any{
		"jsonrpc": "2.0",
		"id":      v,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func (s *server) write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.w.Write(append(data, '\n'))
	return err
}

func normalizeID(raw json.RawMessage) (string, error) {
	v, err := decodeID(raw)
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

func decodeID(raw json.RawMessage) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}
