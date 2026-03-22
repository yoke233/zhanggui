package acphandler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
)

func TestACPHandlerResolveThreadPaths(t *testing.T) {
	baseDir := t.TempDir()
	workspaceDir := filepath.Join(baseDir, "workspace")
	mountDir := filepath.Join(baseDir, "project-alpha")
	for _, dir := range []string{workspaceDir, mountDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	h := NewACPHandler(workspaceDir, "", nil)
	h.SetThreadWorkspace(ThreadWorkspaceConfig{
		ThreadID:     1,
		WorkspaceDir: workspaceDir,
		Mounts: []ThreadMount{
			{Alias: "project-alpha", TargetPath: mountDir, Access: "check", CheckCommands: []string{"go test ./..."}},
		},
	})

	if _, err := h.resolvePath("notes/todo.md", accessWrite); err != nil {
		t.Fatalf("workspace write should be allowed: %v", err)
	}
	if _, err := h.resolvePath("mounts/project-alpha/README.md", accessRead); err != nil {
		t.Fatalf("mount read should be allowed: %v", err)
	}
	if _, err := h.resolvePath("mounts/project-alpha/README.md", accessWrite); err == nil {
		t.Fatal("mount write should be rejected for check access")
	}
}

func TestACPHandlerCreateTerminalChecksWhitelist(t *testing.T) {
	baseDir := t.TempDir()
	workspaceDir := filepath.Join(baseDir, "workspace")
	mountDir := filepath.Join(baseDir, "project-alpha")
	for _, dir := range []string{workspaceDir, mountDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	h := NewACPHandler(workspaceDir, "", nil)
	h.SetThreadWorkspace(ThreadWorkspaceConfig{
		ThreadID:     1,
		WorkspaceDir: workspaceDir,
		Mounts: []ThreadMount{
			{Alias: "project-alpha", TargetPath: mountDir, Access: "check", CheckCommands: []string{"go test ./..."}},
		},
	})

	if _, err := h.CreateTerminal(context.Background(), acpproto.CreateTerminalRequest{
		Command: "go",
		Args:    []string{"version"},
		Cwd:     stringPtr("mounts/project-alpha"),
	}); err == nil {
		t.Fatal("expected non-whitelisted command to be rejected")
	}
}

func TestACPHandlerCreateTerminalAllowsWhitelistedMountCommand(t *testing.T) {
	baseDir := t.TempDir()
	workspaceDir := filepath.Join(baseDir, "workspace")
	mountDir := filepath.Join(baseDir, "project-alpha")
	for _, dir := range []string{workspaceDir, mountDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(mountDir, "go.mod"), []byte("module example.com/projectalpha\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mountDir, "main_test.go"), []byte("package projectalpha\n\nimport \"testing\"\n\nfunc TestWorkspace(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatalf("write main_test.go: %v", err)
	}

	h := NewACPHandler(workspaceDir, "", nil)
	h.SetThreadWorkspace(ThreadWorkspaceConfig{
		ThreadID:     1,
		WorkspaceDir: workspaceDir,
		Mounts: []ThreadMount{
			{Alias: "project-alpha", TargetPath: mountDir, Access: "check", CheckCommands: []string{"go test ./..."}},
		},
	})

	resp, err := h.CreateTerminal(context.Background(), acpproto.CreateTerminalRequest{
		Command: "go",
		Args:    []string{"test", "./..."},
		Cwd:     stringPtr("mounts/project-alpha"),
	})
	if err != nil {
		t.Fatalf("CreateTerminal() error = %v", err)
	}

	exit, err := h.WaitForTerminalExit(context.Background(), acpproto.WaitForTerminalExitRequest{TerminalId: resp.TerminalId})
	if err != nil {
		t.Fatalf("WaitForTerminalExit() error = %v", err)
	}
	if exit.ExitCode == nil || *exit.ExitCode != 0 {
		output, _ := h.TerminalOutput(context.Background(), acpproto.TerminalOutputRequest{TerminalId: resp.TerminalId})
		t.Fatalf("expected go test success, exit=%+v output=%s", exit, output.Output)
	}
}

func TestACPHandlerWriteTextFileAllowsWriteMount(t *testing.T) {
	baseDir := t.TempDir()
	workspaceDir := filepath.Join(baseDir, "workspace")
	mountDir := filepath.Join(baseDir, "project-alpha")
	for _, dir := range []string{workspaceDir, mountDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	h := NewACPHandler(workspaceDir, "", nil)
	h.SetThreadWorkspace(ThreadWorkspaceConfig{
		ThreadID:     1,
		WorkspaceDir: workspaceDir,
		Mounts: []ThreadMount{
			{Alias: "project-alpha", TargetPath: mountDir, Access: "write"},
		},
	})

	if _, err := h.WriteTextFile(context.Background(), acpproto.WriteTextFileRequest{
		Path:    "mounts/project-alpha/notes.md",
		Content: "hello",
	}); err != nil {
		t.Fatalf("WriteTextFile on write mount: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(mountDir, "notes.md"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(raw) != "hello" {
		t.Fatalf("unexpected file content: %q", string(raw))
	}
}

func TestACPHandlerPermissionScopeAllowsMountPath(t *testing.T) {
	baseDir := t.TempDir()
	workspaceDir := filepath.Join(baseDir, "workspace")
	mountDir := filepath.Join(baseDir, "project-alpha")
	for _, dir := range []string{workspaceDir, mountDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	h := NewACPHandler(workspaceDir, "", nil)
	h.SetThreadWorkspace(ThreadWorkspaceConfig{
		ThreadID:     1,
		WorkspaceDir: workspaceDir,
		Mounts: []ThreadMount{
			{Alias: "project-alpha", TargetPath: mountDir, Access: "read"},
		},
	})

	if !h.permissionScopeAllowed("mounts/project-alpha/README.md", "cwd") {
		t.Fatal("expected mount path to be considered within cwd scope for permission matching")
	}
	if h.permissionScopeAllowed("../../outside.txt", "cwd") {
		t.Fatal("expected outside path to be rejected for cwd scope")
	}
}

func TestMountAllowsCommand(t *testing.T) {
	mount := &ThreadMount{
		Alias:         "project-alpha",
		Access:        "check",
		CheckCommands: []string{"go test ./...", "npm test"},
	}
	if !mountAllowsCommand(mount, "go", []string{"test", "./..."}) {
		t.Fatal("expected go test ./... to be allowed")
	}
	if !mountAllowsCommand(mount, "go.exe", []string{"test", "./..."}) {
		t.Fatal("expected go.exe test ./... to be allowed")
	}
	if mountAllowsCommand(mount, "go", []string{"build", "./..."}) {
		t.Fatal("expected go build ./... to be rejected")
	}
}

func stringPtr(value string) *string {
	return &value
}

type recordingPublisher struct {
	events []Event
}

func (p *recordingPublisher) Publish(_ context.Context, evt Event) error {
	p.events = append(p.events, evt)
	return nil
}

type recordingRunEventRecorder struct {
	events []ChatRunEvent
}

func (r *recordingRunEventRecorder) AppendChatRunEvent(event ChatRunEvent) error {
	r.events = append(r.events, event)
	return nil
}

func TestLockedBufferSnapshot(t *testing.T) {
	buf := &lockedBuffer{}
	if _, err := buf.Write([]byte("hello world")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	full, truncated := buf.Snapshot(0)
	if full != "hello world" || truncated {
		t.Fatalf("Snapshot(0) = (%q, %v), want full content and false", full, truncated)
	}

	tail, truncated := buf.Snapshot(5)
	if tail != "world" || !truncated {
		t.Fatalf("Snapshot(5) = (%q, %v), want (world, true)", tail, truncated)
	}
}

func TestApplyReadLineWindow(t *testing.T) {
	content := "a\nb\nc\nd"

	if got := applyReadLineWindow(content, nil, nil); got != content {
		t.Fatalf("applyReadLineWindow(nil,nil) = %q", got)
	}

	line := 2
	limit := 2
	if got := applyReadLineWindow(content, &line, &limit); got != "b\nc" {
		t.Fatalf("applyReadLineWindow(line=2,limit=2) = %q", got)
	}

	line = 10
	if got := applyReadLineWindow(content, &line, nil); got != "" {
		t.Fatalf("applyReadLineWindow(out of range) = %q, want empty", got)
	}
}

func TestACPHandlerRequestPermissionAndHelpers(t *testing.T) {
	h := NewACPHandler(t.TempDir(), "", nil)
	h.SetPermissionPolicy([]acpclient.PermissionRule{
		{Pattern: "write_file", Action: "reject_once", Scope: "cwd"},
	})

	editKind := acpproto.ToolKindEdit
	resp, err := h.RequestPermission(context.Background(), acpproto.RequestPermissionRequest{
		ToolCall: acpproto.ToolCallUpdate{
			ToolCallId: "tc-1",
			Kind:       &editKind,
			RawInput:   map[string]any{"path": "notes/todo.md"},
		},
		Options: []acpproto.PermissionOption{
			{OptionId: "allow_once", Kind: "allow_once", Name: "Allow once"},
			{OptionId: "reject_once", Kind: "reject_once", Name: "Reject once"},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission() error = %v", err)
	}
	if resp.Outcome.Selected == nil || string(resp.Outcome.Selected.OptionId) != "reject_once" {
		t.Fatalf("expected reject_once selection, got %#v", resp.Outcome)
	}

	unknownResp, err := h.RequestPermission(context.Background(), acpproto.RequestPermissionRequest{
		ToolCall: acpproto.ToolCallUpdate{
			ToolCallId: "tc-2",
			Kind:       ptrToolKind("custom_tool"),
			RawInput:   json.RawMessage(`{"path":"notes/todo.md"}`),
		},
		Options: []acpproto.PermissionOption{
			{OptionId: "allow_once", Kind: "allow_once", Name: "Allow once"},
			{OptionId: "reject_once", Kind: "reject_once", Name: "Reject once"},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission(unknown) error = %v", err)
	}
	if unknownResp.Outcome.Selected == nil || string(unknownResp.Outcome.Selected.OptionId) != "allow_once" {
		t.Fatalf("expected allow_once for unknown action, got %#v", unknownResp.Outcome)
	}

	if got := permissionResourceFromRawInput([]byte(`{"filePath":"nested/file.txt"}`)); got != "nested/file.txt" {
		t.Fatalf("permissionResourceFromRawInput(bytes) = %q", got)
	}
	if got := permissionResourceFromRawInput(123); got != "" {
		t.Fatalf("permissionResourceFromRawInput(unsupported) = %q, want empty", got)
	}

	if normalizePermissionPattern("terminal_create") != "terminal/create" {
		t.Fatal("expected terminal_create to normalize to terminal/create")
	}
	if normalizePermissionAction("reject_always") != "reject_always" {
		t.Fatal("expected reject_always to stay normalized")
	}
	if normalizePermissionAction("something_else") != "" {
		t.Fatal("expected unknown action normalization to be empty")
	}
}

func TestACPHandlerHandleSessionUpdateAggregatesChunks(t *testing.T) {
	recorder := &recordingRunEventRecorder{}
	publisher := &recordingPublisher{}
	baseDir := t.TempDir()
	h := NewACPHandler(baseDir, "agent-session", publisher)
	h.SetChatSessionID("chat-session")
	h.SetProjectID("project-1")
	h.SetRunEventRecorder(recorder)

	var callbackCommands int
	var callbackConfigs int
	h.SetSessionStateCallback(func(commands []acpproto.AvailableCommand, configOptions []acpproto.SessionConfigOptionSelect) {
		if commands != nil {
			callbackCommands = len(commands)
		}
		if configOptions != nil {
			callbackConfigs = len(configOptions)
		}
	})

	if err := h.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{
		Type:     "available_commands_update",
		Commands: []acpproto.AvailableCommand{{Name: "read"}},
	}); err != nil {
		t.Fatalf("HandleSessionUpdate(commands) error = %v", err)
	}
	if err := h.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{
		Type:          "config_option_update",
		ConfigOptions: []acpproto.SessionConfigOptionSelect{{Name: "mode"}},
	}); err != nil {
		t.Fatalf("HandleSessionUpdate(config) error = %v", err)
	}
	if callbackCommands != 1 || callbackConfigs != 1 {
		t.Fatalf("unexpected callback counts: commands=%d configs=%d", callbackCommands, callbackConfigs)
	}

	for _, update := range []acpclient.SessionUpdate{
		{Type: "agent_message_chunk", Text: "Hello "},
		{Type: "agent_message_chunk", RawJSON: json.RawMessage(`{"content":{"text":"world"}}`)},
		{Type: "run_completed", Text: "done", Status: "completed"},
	} {
		if err := h.HandleSessionUpdate(context.Background(), update); err != nil {
			t.Fatalf("HandleSessionUpdate(%s) error = %v", update.Type, err)
		}
	}

	if err := h.FlushPendingChatRunEvents(); err != nil {
		t.Fatalf("FlushPendingChatRunEvents() error = %v", err)
	}

	if len(recorder.events) != 4 {
		t.Fatalf("recorded events = %d, want 4", len(recorder.events))
	}
	if recorder.events[2].UpdateType != "agent_message" || recorder.events[2].Payload["text"] != "Hello world" {
		t.Fatalf("unexpected aggregated event: %+v", recorder.events[2])
	}
	if recorder.events[3].UpdateType != "run_completed" {
		t.Fatalf("unexpected trailing event: %+v", recorder.events[3])
	}

	if len(publisher.events) != 5 {
		t.Fatalf("published events = %d, want 5", len(publisher.events))
	}
}

func TestACPHandlerSessionContextAndChangedFiles(t *testing.T) {
	baseDir := t.TempDir()
	h := NewACPHandler(baseDir, "agent-session", nil)
	h.SetChatSessionID("chat-session")

	if _, err := h.WriteTextFile(context.Background(), acpproto.WriteTextFileRequest{
		Path:    "notes/one.txt",
		Content: "one",
	}); err != nil {
		t.Fatalf("WriteTextFile(first) error = %v", err)
	}
	if _, err := h.WriteTextFile(context.Background(), acpproto.WriteTextFileRequest{
		Path:    "notes/one.txt",
		Content: "two",
	}); err != nil {
		t.Fatalf("WriteTextFile(second) error = %v", err)
	}

	ctx := h.SessionContext()
	if ctx.SessionID != "chat-session" {
		t.Fatalf("SessionContext().SessionID = %q, want chat-session", ctx.SessionID)
	}
	if len(ctx.ChangedFiles) != 1 || ctx.ChangedFiles[0] != "notes/one.txt" {
		t.Fatalf("unexpected changed files: %+v", ctx.ChangedFiles)
	}
}

func ptrToolKind(kind string) *acpproto.ToolKind {
	value := acpproto.ToolKind(kind)
	return &value
}
