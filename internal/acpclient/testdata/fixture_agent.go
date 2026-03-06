// fixture_agent: a fake ACP agent that replays events from a JSON fixture file.
//
// Usage:  go run fixture_agent.go <fixture.json> [scenario]
//
// It handles initialize, session/new, session/load, session/prompt.
// On session/load it replays events from the "load_session_replay" scenario.
// On session/prompt it replays events from the scenario matching the prompt text,
// or falls back to the scenario specified on the command line (default: "new_session_simple_prompt").
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type fixtureEvent struct {
	OffsetMs int64           `json:"offset_ms"`
	Raw      json.RawMessage `json:"raw"`
}

type fixtureScenario struct {
	Description string         `json:"description"`
	SessionID   string         `json:"session_id"`
	Events      []fixtureEvent `json:"events"`
	Result      *fixtureResult `json:"result,omitempty"`
}

type fixtureResult struct {
	Text       string `json:"text"`
	StopReason string `json:"stop_reason"`
}

type fixtureFile struct {
	Scenarios map[string]fixtureScenario `json:"scenarios"`
}

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
	fixtures *fixtureFile
	scenario string // default scenario name
	seq      int
	sessions map[string]string // sessionID -> scenario used to create it
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fixture_agent <fixture.json> [default_scenario]")
		os.Exit(1)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read fixture: %v\n", err)
		os.Exit(1)
	}
	var ff fixtureFile
	if err := json.Unmarshal(data, &ff); err != nil {
		fmt.Fprintf(os.Stderr, "parse fixture: %v\n", err)
		os.Exit(1)
	}

	scenario := "new_session_simple_prompt"
	if len(os.Args) > 2 {
		scenario = os.Args[2]
	}

	s := &server{
		r:        bufio.NewReader(os.Stdin),
		w:        os.Stdout,
		fixtures: &ff,
		scenario: scenario,
		sessions: make(map[string]string),
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
					"name":    "fixture-acp-agent",
					"title":   "Fixture ACP Agent",
					"version": "0.1.0",
				},
			}); err != nil {
				return err
			}
		case "session/new":
			s.seq++
			sessionID := fmt.Sprintf("fixture-session-%d", s.seq)
			s.sessions[sessionID] = s.scenario
			if err := s.replyResult(msg.ID, map[string]any{
				"sessionId": sessionID,
			}); err != nil {
				return err
			}
		case "session/load":
			if err := s.handleLoad(msg); err != nil {
				return err
			}
		case "session/prompt":
			if err := s.handlePrompt(msg); err != nil {
				return err
			}
		case "session/cancel":
			// ignore
		default:
			if len(msg.ID) > 0 {
				if err := s.replyError(msg.ID, -32601, "method not found"); err != nil {
					return err
				}
			}
		}
	}
}

func (s *server) handleLoad(msg envelope) error {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.Unmarshal(msg.Params, &req)

	// Replay load_session_replay events if available.
	sc, ok := s.fixtures.Scenarios["load_session_replay"]
	if !ok {
		return s.replyError(msg.ID, -32004, "no load_session_replay scenario in fixture")
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = sc.SessionID
	}
	s.sessions[sessionID] = "load_session_replay"

	// Reply first, then send replay events (mimics real agent behavior).
	if err := s.replyResult(msg.ID, map[string]any{
		"sessionId": sessionID,
	}); err != nil {
		return err
	}

	return s.replayEvents(sessionID, sc)
}

func (s *server) handlePrompt(msg envelope) error {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.Unmarshal(msg.Params, &req)

	// Determine which scenario to replay.
	scenarioName := s.sessions[req.SessionID]
	if scenarioName == "" {
		scenarioName = s.scenario
	}

	// If session was loaded, use load_session_then_prompt if available.
	if scenarioName == "load_session_replay" {
		if _, ok := s.fixtures.Scenarios["load_session_then_prompt"]; ok {
			scenarioName = "load_session_then_prompt"
		}
	}

	sc, ok := s.fixtures.Scenarios[scenarioName]
	if !ok {
		sc = s.fixtures.Scenarios[s.scenario]
	}

	// Replay events.
	if err := s.replayEvents(req.SessionID, sc); err != nil {
		return err
	}

	// Build prompt result.
	result := map[string]any{
		"requestId":  "req-1",
		"stopReason": "end_turn",
	}
	if sc.Result != nil {
		if sc.Result.StopReason != "" {
			result["stopReason"] = sc.Result.StopReason
		}
	}
	return s.replyResult(msg.ID, result)
}

func (s *server) replayEvents(sessionID string, sc fixtureScenario) error {
	var lastOffset int64
	for _, evt := range sc.Events {
		// Simulate timing proportionally (capped at 10ms per gap to keep tests fast).
		gap := evt.OffsetMs - lastOffset
		if gap > 10 {
			gap = 10
		}
		if gap > 0 {
			time.Sleep(time.Duration(gap) * time.Millisecond)
		}
		lastOffset = evt.OffsetMs

		var notification map[string]json.RawMessage
		if err := json.Unmarshal(evt.Raw, &notification); err != nil {
			continue
		}
		updateRaw, ok := notification["update"]
		if !ok || len(updateRaw) == 0 {
			continue
		}
		msg := map[string]any{
			"jsonrpc": "2.0",
			"method":  "session/update",
			"params": map[string]any{
				"sessionId": sessionID,
				"update":    json.RawMessage(updateRaw),
			},
		}
		if err := s.write(msg); err != nil {
			return err
		}
	}
	return nil
}

// --- transport helpers ---

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

func decodeID(raw json.RawMessage) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
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
