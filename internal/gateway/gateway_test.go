package gateway

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGateway_PathTraversalDenied(t *testing.T) {
	root := t.TempDir()
	aud, err := NewAuditor(filepath.Join(root, "logs", "tool_audit.jsonl"))
	if err != nil {
		t.Fatalf("NewAuditor: %v", err)
	}
	t.Cleanup(func() { _ = aud.Close() })

	gw, err := New(root, Actor{AgentID: "t", Role: "system"}, Linkage{TaskID: "task-1"}, Policy{
		AllowedWritePrefixes: []string{""},
	}, aud)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = gw.ReplaceFile("../escape.txt", []byte("x"), 0o644, "test")
	if err == nil {
		t.Fatalf("expected error")
	}
	var de DenyError
	if !errors.As(err, &de) {
		t.Fatalf("expected DenyError, got %T: %v", err, err)
	}
	if de.Code != "E_BAD_PATH" {
		t.Fatalf("unexpected code: %s", de.Code)
	}
}

func TestGateway_ACLDenied(t *testing.T) {
	root := t.TempDir()
	aud, err := NewAuditor(filepath.Join(root, "logs", "tool_audit.jsonl"))
	if err != nil {
		t.Fatalf("NewAuditor: %v", err)
	}
	t.Cleanup(func() { _ = aud.Close() })

	gw, err := New(root, Actor{AgentID: "t", Role: "system"}, Linkage{}, Policy{
		AllowedWritePrefixes: []string{"revs/"},
	}, aud)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = gw.ReplaceFile("state.json", []byte("{}\n"), 0o644, "test")
	if err == nil {
		t.Fatalf("expected error")
	}
	var de DenyError
	if !errors.As(err, &de) {
		t.Fatalf("expected DenyError, got %T: %v", err, err)
	}
	if de.Code != "E_ACL_DENY" {
		t.Fatalf("unexpected code: %s", de.Code)
	}
}

func TestGateway_AppendOnlyEnforced(t *testing.T) {
	root := t.TempDir()
	aud, err := NewAuditor(filepath.Join(root, "logs", "tool_audit.jsonl"))
	if err != nil {
		t.Fatalf("NewAuditor: %v", err)
	}
	t.Cleanup(func() { _ = aud.Close() })

	gw, err := New(root, Actor{AgentID: "recorder-1", Role: "recorder"}, Linkage{MeetingID: "mtg-1"}, Policy{
		AllowedWritePrefixes: []string{"shared/"},
		AppendOnlyFiles:      []string{"shared/transcript.log"},
	}, aud)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := gw.AppendFile("shared/transcript.log", []byte("hello\n"), 0o644, "append"); err != nil {
		t.Fatalf("AppendFile: %v", err)
	}

	err = gw.ReplaceFile("shared/transcript.log", []byte("overwrite\n"), 0o644, "replace")
	if err == nil {
		t.Fatalf("expected error")
	}
	var de DenyError
	if !errors.As(err, &de) {
		t.Fatalf("expected DenyError, got %T: %v", err, err)
	}
	if de.Code != "E_APPEND_ONLY_VIOLATION" {
		t.Fatalf("unexpected code: %s", de.Code)
	}
}

func TestGateway_SingleWriterRoleAndLock(t *testing.T) {
	root := t.TempDir()
	aud, err := NewAuditor(filepath.Join(root, "logs", "tool_audit.jsonl"))
	if err != nil {
		t.Fatalf("NewAuditor: %v", err)
	}
	t.Cleanup(func() { _ = aud.Close() })

	// 非 recorder 角色：应被拒绝
	guest, err := New(root, Actor{AgentID: "a1", Role: "architect"}, Linkage{MeetingID: "mtg-1"}, Policy{
		AllowedWritePrefixes: []string{""},
		SingleWriterPrefixes: []string{"shared/"},
		SingleWriterRoles:    []string{"recorder"},
		LockFile:             "shared/.writer.lock",
	}, aud)
	if err != nil {
		t.Fatalf("New guest: %v", err)
	}
	err = guest.ReplaceFile("shared/whiteboard.md", []byte("x\n"), 0o644, "replace")
	if err == nil {
		t.Fatalf("expected error")
	}
	var de DenyError
	if !errors.As(err, &de) {
		t.Fatalf("expected DenyError, got %T: %v", err, err)
	}
	if de.Code != "E_SINGLE_WRITER_VIOLATION" {
		t.Fatalf("unexpected code: %s", de.Code)
	}

	// recorder 角色但未持锁：应被拒绝
	rec, err := New(root, Actor{AgentID: "recorder-1", Role: "recorder"}, Linkage{MeetingID: "mtg-1"}, Policy{
		AllowedWritePrefixes: []string{""},
		SingleWriterPrefixes: []string{"shared/"},
		SingleWriterRoles:    []string{"recorder"},
		LockFile:             "shared/.writer.lock",
	}, aud)
	if err != nil {
		t.Fatalf("New recorder: %v", err)
	}
	err = rec.ReplaceFile("shared/whiteboard.md", []byte("x\n"), 0o644, "replace")
	if err == nil {
		t.Fatalf("expected error")
	}
	de = DenyError{}
	if !errors.As(err, &de) {
		t.Fatalf("expected DenyError, got %T: %v", err, err)
	}
	if de.Code != "E_LOCK_NOT_HELD" {
		t.Fatalf("unexpected code: %s", de.Code)
	}

	// 获取锁后允许写入
	if err := rec.AcquireLock(); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if err := rec.ReplaceFile("shared/whiteboard.md", []byte("ok\n"), 0o644, "replace"); err != nil {
		t.Fatalf("ReplaceFile: %v", err)
	}

	lockPath := filepath.Join(root, "shared", ".writer.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}

	// 审计至少应有一行
	b, err := os.ReadFile(filepath.Join(root, "logs", "tool_audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		t.Fatalf("expected audit not empty")
	}
}
