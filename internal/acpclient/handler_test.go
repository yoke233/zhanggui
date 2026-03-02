package acpclient

import (
	"context"
	"testing"
)

func TestNopHandlerImplementsHandler(t *testing.T) {
	var h Handler = &NopHandler{}
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestNopHandlerImplementsEventHandler(t *testing.T) {
	var h EventHandler = &NopHandler{}
	if h == nil {
		t.Fatal("expected non-nil event handler")
	}
}

func TestNopHandlerCallbacksReturnZeroValues(t *testing.T) {
	ctx := context.Background()
	h := &NopHandler{}

	if _, err := h.HandleReadFile(ctx, ReadFileRequest{Path: "demo.txt"}); err != nil {
		t.Fatalf("HandleReadFile returned error: %v", err)
	}
	if _, err := h.HandleWriteFile(ctx, WriteFileRequest{Path: "demo.txt", Content: "hello"}); err != nil {
		t.Fatalf("HandleWriteFile returned error: %v", err)
	}
	if _, err := h.HandleRequestPermission(ctx, PermissionRequest{Action: "write_file"}); err != nil {
		t.Fatalf("HandleRequestPermission returned error: %v", err)
	}
	if _, err := h.HandleTerminalCreate(ctx, TerminalCreateRequest{Command: []string{"echo", "ok"}}); err != nil {
		t.Fatalf("HandleTerminalCreate returned error: %v", err)
	}
	if _, err := h.HandleTerminalWrite(ctx, TerminalWriteRequest{TerminalID: "t1", Input: "pwd\n"}); err != nil {
		t.Fatalf("HandleTerminalWrite returned error: %v", err)
	}
	if _, err := h.HandleTerminalRead(ctx, TerminalReadRequest{TerminalID: "t1", MaxBytes: 128}); err != nil {
		t.Fatalf("HandleTerminalRead returned error: %v", err)
	}
	if _, err := h.HandleTerminalResize(ctx, TerminalResizeRequest{TerminalID: "t1", Cols: 120, Rows: 40}); err != nil {
		t.Fatalf("HandleTerminalResize returned error: %v", err)
	}
	if _, err := h.HandleTerminalClose(ctx, TerminalCloseRequest{TerminalID: "t1"}); err != nil {
		t.Fatalf("HandleTerminalClose returned error: %v", err)
	}
	if err := h.HandleSessionUpdate(ctx, SessionUpdate{SessionID: "s1", Type: "agent_message_chunk"}); err != nil {
		t.Fatalf("HandleSessionUpdate returned error: %v", err)
	}
}
