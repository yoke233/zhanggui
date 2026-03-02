package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ChatAssistantRequest contains one user turn for model completion.
type ChatAssistantRequest struct {
	Message        string
	WorkDir        string
	AgentSessionID string
}

// ChatAssistantResponse contains assistant content and provider session identity.
type ChatAssistantResponse struct {
	Reply          string
	AgentSessionID string
}

// ChatAssistant provides multi-turn chat completion for /chat APIs.
type ChatAssistant interface {
	Reply(ctx context.Context, req ChatAssistantRequest) (ChatAssistantResponse, error)
}

type claudeCommandRunner interface {
	Run(ctx context.Context, workDir, command string, args []string) (stdout string, stderr string, err error)
}

type shellClaudeCommandRunner struct{}

func (r shellClaudeCommandRunner) Run(ctx context.Context, workDir, command string, args []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if strings.TrimSpace(workDir) != "" {
		cmd.Dir = strings.TrimSpace(workDir)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// ClaudeChatAssistant calls claude CLI in stream-json mode and supports session resume.
type ClaudeChatAssistant struct {
	binary   string
	maxTurns int
	runner   claudeCommandRunner
}

// NewClaudeChatAssistant creates a ChatAssistant backed by claude CLI.
func NewClaudeChatAssistant(binary string) ChatAssistant {
	trimmedBinary := strings.TrimSpace(binary)
	if trimmedBinary == "" {
		trimmedBinary = "claude"
	}
	return &ClaudeChatAssistant{
		binary:   trimmedBinary,
		maxTurns: 1,
		runner:   shellClaudeCommandRunner{},
	}
}

func newClaudeChatAssistantForTest(binary string, maxTurns int, runner claudeCommandRunner) *ClaudeChatAssistant {
	if maxTurns <= 0 {
		maxTurns = 1
	}
	if runner == nil {
		runner = shellClaudeCommandRunner{}
	}
	trimmedBinary := strings.TrimSpace(binary)
	if trimmedBinary == "" {
		trimmedBinary = "claude"
	}
	return &ClaudeChatAssistant{
		binary:   trimmedBinary,
		maxTurns: maxTurns,
		runner:   runner,
	}
}

func (a *ClaudeChatAssistant) Reply(ctx context.Context, req ChatAssistantRequest) (ChatAssistantResponse, error) {
	if a == nil {
		return ChatAssistantResponse{}, errors.New("chat assistant is nil")
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return ChatAssistantResponse{}, errors.New("message is required")
	}
	if a.runner == nil {
		return ChatAssistantResponse{}, errors.New("chat assistant runner is not configured")
	}

	args := []string{
		"-p", message,
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", strconv.Itoa(a.maxTurns),
	}
	if strings.TrimSpace(req.AgentSessionID) != "" {
		args = append([]string{"--resume", strings.TrimSpace(req.AgentSessionID)}, args...)
	}

	runCtx, cancel := withDefaultTimeout(ctx, 90*time.Second)
	defer cancel()
	stdout, stderr, err := a.runner.Run(runCtx, req.WorkDir, a.binary, args)
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail == "" {
			detail = err.Error()
		}
		return ChatAssistantResponse{}, fmt.Errorf("claude command failed: %s", detail)
	}

	reply, sessionID, parseErr := parseClaudeStreamJSON(stdout)
	if parseErr != nil {
		return ChatAssistantResponse{}, parseErr
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(req.AgentSessionID)
	}

	return ChatAssistantResponse{
		Reply:          reply,
		AgentSessionID: sessionID,
	}, nil
}

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func parseClaudeStreamJSON(output string) (reply, sessionID string, err error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	textParts := make([]string, 0, 8)
	resultText := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw map[string]any
		if jsonErr := json.Unmarshal([]byte(line), &raw); jsonErr != nil {
			continue
		}
		if sid := extractString(raw["session_id"]); sid != "" {
			sessionID = sid
		}

		eventType := extractString(raw["type"])
		switch eventType {
		case "assistant":
			message, _ := raw["message"].(map[string]any)
			if sid := extractString(message["session_id"]); sid != "" {
				sessionID = sid
			}
			contents, _ := message["content"].([]any)
			for _, item := range contents {
				contentMap, _ := item.(map[string]any)
				if extractString(contentMap["type"]) != "text" {
					continue
				}
				text := strings.TrimSpace(extractString(contentMap["text"]))
				if text != "" {
					textParts = append(textParts, text)
				}
			}
		case "result":
			text := strings.TrimSpace(extractString(raw["result"]))
			if text != "" {
				resultText = text
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", fmt.Errorf("parse claude output stream: %w", err)
	}

	reply = strings.TrimSpace(strings.Join(textParts, "\n\n"))
	if reply == "" {
		reply = strings.TrimSpace(resultText)
	}
	if reply == "" {
		return "", sessionID, errors.New("claude returned empty reply")
	}
	return reply, sessionID, nil
}

func extractString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
