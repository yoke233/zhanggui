package web

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestClaudeChatAssistantReplyBuildsNewSessionCommand(t *testing.T) {
	runner := &recordingClaudeRunner{
		stdout: strings.Join([]string{
			`{"type":"system","session_id":"sid-1"}`,
			`{"type":"assistant","message":{"content":[{"type":"text","text":"hello from claude"}]}}`,
			`{"type":"result","result":"ok"}`,
		}, "\n"),
	}
	assistant := newClaudeChatAssistantForTest("claude", 1, runner)

	got, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message: "hello",
		WorkDir: "D:/repo/demo",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}

	if got.Reply != "hello from claude" {
		t.Fatalf("expected reply %q, got %q", "hello from claude", got.Reply)
	}
	if got.AgentSessionID != "sid-1" {
		t.Fatalf("expected session id %q, got %q", "sid-1", got.AgentSessionID)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one runner call, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.command != "claude" {
		t.Fatalf("expected command claude, got %s", call.command)
	}
	if call.workDir != "D:/repo/demo" {
		t.Fatalf("expected workDir D:/repo/demo, got %s", call.workDir)
	}
	joined := strings.Join(call.args, " ")
	if strings.Contains(joined, "--resume") {
		t.Fatalf("did not expect --resume for first turn, args=%v", call.args)
	}
	if !strings.Contains(joined, "--output-format stream-json") || !strings.Contains(joined, "--verbose") {
		t.Fatalf("expected stream-json + --verbose args, got %v", call.args)
	}
}

func TestClaudeChatAssistantReplyUsesResumeForExistingSession(t *testing.T) {
	runner := &recordingClaudeRunner{
		stdout: `{"type":"assistant","message":{"content":[{"type":"text","text":"next turn"}]}}`,
	}
	assistant := newClaudeChatAssistantForTest("claude", 1, runner)

	got, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message:        "continue",
		AgentSessionID: "sid-old",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if got.AgentSessionID != "sid-old" {
		t.Fatalf("expected fallback session id sid-old, got %q", got.AgentSessionID)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one runner call, got %d", len(runner.calls))
	}
	args := runner.calls[0].args
	if len(args) < 2 || args[0] != "--resume" || args[1] != "sid-old" {
		t.Fatalf("expected --resume sid-old prefix, got %v", args)
	}
}

func TestClaudeChatAssistantReplyReturnsCommandError(t *testing.T) {
	runner := &recordingClaudeRunner{
		stderr: "auth failed",
		err:    errors.New("exit status 1"),
	}
	assistant := newClaudeChatAssistantForTest("claude", 1, runner)

	_, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message: "hello",
	})
	if err == nil {
		t.Fatal("expected error when runner fails")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("expected stderr detail in error, got %v", err)
	}
}

func TestParseClaudeStreamJSONUsesResultFallback(t *testing.T) {
	reply, sessionID, err := parseClaudeStreamJSON(`{"type":"result","session_id":"sid-2","result":"fallback reply"}`)
	if err != nil {
		t.Fatalf("parseClaudeStreamJSON returned error: %v", err)
	}
	if reply != "fallback reply" {
		t.Fatalf("expected fallback reply, got %q", reply)
	}
	if sessionID != "sid-2" {
		t.Fatalf("expected session id sid-2, got %q", sessionID)
	}
}

type recordingClaudeRunner struct {
	stdout string
	stderr string
	err    error
	calls  []claudeRunnerCall
}

type claudeRunnerCall struct {
	command string
	workDir string
	args    []string
}

func (r *recordingClaudeRunner) Run(_ context.Context, workDir, command string, args []string) (string, string, error) {
	clonedArgs := make([]string, len(args))
	copy(clonedArgs, args)
	r.calls = append(r.calls, claudeRunnerCall{
		command: command,
		workDir: workDir,
		args:    clonedArgs,
	})
	return r.stdout, r.stderr, r.err
}
