package acp

import (
	"testing"

	chatapp "github.com/yoke233/zhanggui/internal/application/chat"
)

func TestSetAndTakePending(t *testing.T) {
	agent := &LeadAgent{
		pendingMsgs: make(map[string]*chatapp.PendingMessage),
	}
	sid := "sess-1"

	// Initially no pending.
	if got := agent.takePending(sid); got != nil {
		t.Fatal("expected nil pending")
	}

	// Set a pending message.
	agent.setPending(sid, &chatapp.PendingMessage{Message: "hello"})
	got := agent.takePending(sid)
	if got == nil || got.Message != "hello" {
		t.Fatalf("expected pending message 'hello', got %v", got)
	}

	// After take, pending should be cleared.
	if got := agent.takePending(sid); got != nil {
		t.Fatal("expected nil after take")
	}
}

func TestSetPendingReplacesPrevious(t *testing.T) {
	agent := &LeadAgent{
		pendingMsgs: make(map[string]*chatapp.PendingMessage),
	}
	sid := "sess-1"

	agent.setPending(sid, &chatapp.PendingMessage{Message: "first"})
	agent.setPending(sid, &chatapp.PendingMessage{Message: "second"})

	got := agent.takePending(sid)
	if got == nil || got.Message != "second" {
		t.Fatalf("expected 'second', got %v", got)
	}
}

func TestCancelPending(t *testing.T) {
	agent := &LeadAgent{
		pendingMsgs: make(map[string]*chatapp.PendingMessage),
	}
	sid := "sess-1"

	// Cancel when nothing pending.
	if agent.CancelPending(sid) {
		t.Fatal("expected false when no pending")
	}

	// Cancel when pending exists.
	agent.setPending(sid, &chatapp.PendingMessage{Message: "hello"})
	if !agent.CancelPending(sid) {
		t.Fatal("expected true when pending existed")
	}

	// Should be cleared.
	if got := agent.takePending(sid); got != nil {
		t.Fatal("expected nil after cancel")
	}
}
