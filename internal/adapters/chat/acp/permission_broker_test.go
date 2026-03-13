package acp

import (
	"context"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
)

func TestPermissionBrokerTimeoutCancelsRequest(t *testing.T) {
	t.Parallel()

	broker := newPermissionBroker()
	resp, err := broker.Submit(context.Background(), broker.NextID(), acpproto.RequestPermissionRequest{}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if resp.Outcome.Cancelled == nil {
		t.Fatalf("expected timeout to cancel permission request, got %#v", resp.Outcome)
	}
}

func TestPermissionBrokerResolveSelectedOption(t *testing.T) {
	t.Parallel()

	broker := newPermissionBroker()
	permID := broker.NextID()
	done := make(chan acpproto.RequestPermissionResponse, 1)

	go func() {
		resp, _ := broker.Submit(context.Background(), permID, acpproto.RequestPermissionRequest{}, time.Second)
		done <- resp
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if ok := broker.Resolve(permID, "allow_once", false); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Resolve() should accept known pending permission")
		}
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case resp := <-done:
		if resp.Outcome.Selected == nil || string(resp.Outcome.Selected.OptionId) != "allow_once" {
			t.Fatalf("expected selected allow_once response, got %#v", resp.Outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broker response")
	}
}
