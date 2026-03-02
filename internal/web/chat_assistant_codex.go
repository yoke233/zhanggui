package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type codexCommandRunner interface {
	Run(ctx context.Context, workDir, command string, args []string) (stdout string, stderr string, err error)
}

// CodexChatAssistant calls codex exec/exec resume and parses JSONL events.
type CodexChatAssistant struct {
	binary    string
	model     string
	reasoning string
	runner    codexCommandRunner
}

// NewCodexChatAssistant creates a ChatAssistant backed by codex CLI.
func NewCodexChatAssistant(binary, model, reasoning string) ChatAssistant {
	trimmedBinary := strings.TrimSpace(binary)
	if trimmedBinary == "" {
		trimmedBinary = "codex"
	}
	return &CodexChatAssistant{
		binary:    trimmedBinary,
		model:     strings.TrimSpace(model),
		reasoning: strings.TrimSpace(reasoning),
		runner:    shellClaudeCommandRunner{},
	}
}

func newCodexChatAssistantForTest(binary, model, reasoning string, runner codexCommandRunner) *CodexChatAssistant {
	trimmedBinary := strings.TrimSpace(binary)
	if trimmedBinary == "" {
		trimmedBinary = "codex"
	}
	if runner == nil {
		runner = shellClaudeCommandRunner{}
	}
	return &CodexChatAssistant{
		binary:    trimmedBinary,
		model:     strings.TrimSpace(model),
		reasoning: strings.TrimSpace(reasoning),
		runner:    runner,
	}
}

func (a *CodexChatAssistant) Reply(ctx context.Context, req ChatAssistantRequest) (ChatAssistantResponse, error) {
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

	args := make([]string, 0, 16)
	if strings.TrimSpace(req.WorkDir) != "" {
		args = append(args, "-C", strings.TrimSpace(req.WorkDir))
	}
	args = append(args, "exec")
	if strings.TrimSpace(req.AgentSessionID) != "" {
		args = append(args, "resume", strings.TrimSpace(req.AgentSessionID), message)
	} else {
		args = append(args, message)
	}
	args = append(args, "--json", "--sandbox", "workspace-write", "-a", "never")
	if a.model != "" {
		args = append(args, "-m", a.model)
	}
	if a.reasoning != "" {
		args = append(args, "-c", "model_reasoning_effort="+a.reasoning)
	}

	runCtx, cancel := withDefaultTimeout(ctx, 90*time.Second)
	defer cancel()
	stdout, stderr, err := a.runner.Run(runCtx, "", a.binary, args)
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail == "" {
			detail = err.Error()
		}
		return ChatAssistantResponse{}, fmt.Errorf("codex command failed: %s", detail)
	}

	reply, sessionID, parseErr := parseCodexJSONL(stdout)
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

func parseCodexJSONL(output string) (reply string, sessionID string, err error) {
	lines := strings.Split(output, "\n")
	textParts := make([]string, 0, 8)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var raw map[string]any
		if jsonErr := json.Unmarshal([]byte(trimmed), &raw); jsonErr != nil {
			continue
		}

		eventType := extractString(raw["type"])
		if sid := extractString(raw["thread_id"]); sid != "" {
			sessionID = sid
		}

		if eventType == "item.completed" {
			item, _ := raw["item"].(map[string]any)
			if sid := extractString(item["thread_id"]); sid != "" {
				sessionID = sid
			}
			itemType := extractString(item["type"])
			if itemType == "agent_message" {
				text := strings.TrimSpace(extractString(item["text"]))
				if text != "" {
					textParts = append(textParts, text)
				}
			}
		}
	}

	reply = strings.TrimSpace(strings.Join(textParts, "\n\n"))
	if reply == "" {
		return "", sessionID, errors.New("codex returned empty reply")
	}
	return reply, sessionID, nil
}
