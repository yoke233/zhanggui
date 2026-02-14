package outbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainoutbox "zhanggui/internal/domain/outbox"
)

func TestPrepareContextPackWritesFiles(t *testing.T) {
	contextPackDir := filepath.Join(t.TempDir(), "context-pack")
	order := domainoutbox.WorkOrder{
		IssueRef: "local#9",
		RunID:    "2026-02-14-backend-0009",
		Role:     "backend",
		RepoDir:  ".",
	}
	specSnapshot := "# Spec Snapshot\n- item\n"

	if err := prepareContextPack(contextPackDir, order, specSnapshot, 27); err != nil {
		t.Fatalf("prepareContextPack() error = %v", err)
	}

	workOrderPath := filepath.Join(contextPackDir, "work_order.json")
	if _, err := os.Stat(workOrderPath); err != nil {
		t.Fatalf("work_order.json should exist, err=%v", err)
	}
	loadedOrder, err := loadWorkOrder(workOrderPath)
	if err != nil {
		t.Fatalf("loadWorkOrder() error = %v", err)
	}
	if loadedOrder.IssueRef != order.IssueRef || loadedOrder.RunID != order.RunID {
		t.Fatalf("work_order mismatch, got=%#v want=%#v", loadedOrder, order)
	}

	specRaw, err := os.ReadFile(filepath.Join(contextPackDir, "spec_snapshot.md"))
	if err != nil {
		t.Fatalf("read spec_snapshot.md error = %v", err)
	}
	if string(specRaw) != specSnapshot {
		t.Fatalf("spec_snapshot.md mismatch, got=%q", string(specRaw))
	}

	constraintsRaw, err := os.ReadFile(filepath.Join(contextPackDir, "constraints.md"))
	if err != nil {
		t.Fatalf("read constraints.md error = %v", err)
	}
	constraints := string(constraintsRaw)
	if !strings.Contains(constraints, "Keep IssueRef and RunId unchanged") {
		t.Fatalf("constraints.md missing issue/run constraint, text=%s", constraints)
	}
	if !strings.Contains(constraints, "Provide Changes and Tests evidence") {
		t.Fatalf("constraints.md missing evidence constraint, text=%s", constraints)
	}

	linksRaw, err := os.ReadFile(filepath.Join(contextPackDir, "links.md"))
	if err != nil {
		t.Fatalf("read links.md error = %v", err)
	}
	links := string(linksRaw)
	if !strings.Contains(links, "IssueRef: local#9") {
		t.Fatalf("links.md missing issue ref, text=%s", links)
	}
	if !strings.Contains(links, "ReadUpTo: e27") {
		t.Fatalf("links.md missing read-up-to, text=%s", links)
	}
}

func TestPrepareContextPackReadUpToNone(t *testing.T) {
	contextPackDir := filepath.Join(t.TempDir(), "context-pack")
	order := domainoutbox.WorkOrder{
		IssueRef: "local#10",
		RunID:    "2026-02-14-backend-0010",
		Role:     "backend",
		RepoDir:  ".",
	}

	if err := prepareContextPack(contextPackDir, order, "snapshot", 0); err != nil {
		t.Fatalf("prepareContextPack() error = %v", err)
	}

	linksRaw, err := os.ReadFile(filepath.Join(contextPackDir, "links.md"))
	if err != nil {
		t.Fatalf("read links.md error = %v", err)
	}
	if !strings.Contains(string(linksRaw), "ReadUpTo: none") {
		t.Fatalf("links.md should contain ReadUpTo none, text=%s", string(linksRaw))
	}
}

func TestInvokeWorkerContextCanceled(t *testing.T) {
	svc := &Service{}
	contextPackDir := t.TempDir()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.invokeWorker(canceledCtx, invokeWorkerInput{
		ExecutablePath: "cmd",
		WorkflowFile:   "workflow.toml",
		ContextPackDir: contextPackDir,
		IssueRef:       "local#11",
		RunID:          "2026-02-14-backend-0011",
		Role:           "backend",
	})
	if err == nil {
		t.Fatalf("invokeWorker(canceled context) expected error")
	}

	if _, statErr := os.Stat(filepath.Join(contextPackDir, "stdout.log")); statErr != nil {
		t.Fatalf("stdout.log should exist, err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(contextPackDir, "stderr.log")); statErr != nil {
		t.Fatalf("stderr.log should exist, err=%v", statErr)
	}
}
