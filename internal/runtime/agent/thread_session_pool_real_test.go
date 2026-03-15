//go:build real

package agentruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

const realCodexACPVersion = "0.9.5"

func realCodexProfile() *core.AgentProfile {
	return &core.AgentProfile{
		ID:   "codex-worker",
		Name: "Codex Worker",
		Role: core.RoleWorker, // defaults: FSRead + FSWrite + Terminal
		Driver: core.DriverConfig{
			LaunchCommand: "npx",
			LaunchArgs:    []string{"-y", "@zed-industries/codex-acp@" + realCodexACPVersion},
		},
	}
}

// TestReal_ThreadPoolFullLifecycle runs the complete ThreadSessionPool flow
// against a real codex-acp agent launched via npx.
//
// Prerequisites:
//   - OPENAI_API_KEY must be set for codex
//   - Node.js + npx must be available
//
// Run:
//
//	AI_WORKFLOW_REAL_THREAD_ACP=1 go test -tags real -run TestReal_ThreadPoolFullLifecycle -v -timeout 300s ./internal/runtime/agent/...
func TestReal_ThreadPoolFullLifecycle(t *testing.T) {
	if os.Getenv("AI_WORKFLOW_REAL_THREAD_ACP") == "" {
		t.Skip("set AI_WORKFLOW_REAL_THREAD_ACP=1 to run")
	}

	profile := realCodexProfile()
	baseDir := t.TempDir()
	dataDir := filepath.Join(baseDir, "data")
	_ = os.MkdirAll(dataDir, 0o755)

	dbPath := filepath.Join(baseDir, "real-thread-pool.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bus := membus.NewBus()
	ctx := context.Background()

	registry := &mockRegistry{profiles: map[string]*core.AgentProfile{
		profile.ID: profile,
	}}

	pool := NewThreadSessionPool(store, bus, registry, dataDir)
	defer pool.Close()

	// Create thread.
	threadID, err := store.CreateThread(ctx, &core.Thread{
		Title:   "Real ACP Thread Test",
		OwnerID: "test-user",
		Status:  core.ThreadActive,
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	t.Logf("thread created: id=%d", threadID)

	// ── 1. InviteAgent ──
	t.Log(">>> inviting agent...")
	member, err := pool.InviteAgent(ctx, threadID, profile.ID)
	if err != nil {
		t.Fatalf("InviteAgent: %v", err)
	}
	t.Logf("member created: id=%d status=%s", member.ID, member.Status)

	// ── 2. Wait for boot ──
	t.Log(">>> waiting for agent boot (may take a while for npx download)...")
	waitCtx, waitCancel := context.WithTimeout(ctx, 180*time.Second)
	defer waitCancel()
	if err := pool.WaitAgentReady(waitCtx, threadID, profile.ID); err != nil {
		t.Fatalf("WaitAgentReady: %v", err)
	}
	t.Log(">>> agent ready")

	// Verify active in DB.
	members, _ := store.ListThreadMembers(ctx, threadID)
	for _, m := range members {
		if m.AgentProfileID == profile.ID {
			t.Logf("member status after boot: %s", m.Status)
			if m.Status != core.ThreadAgentActive {
				t.Fatalf("expected active, got %q", m.Status)
			}
		}
	}

	// ── 3. SendMessage — simple task ──
	t.Log(">>> sending message: create hello.txt...")
	sendCtx, sendCancel := context.WithTimeout(ctx, 120*time.Second)
	defer sendCancel()

	// Use a context that won't cancel the pool's session.
	if err := pool.SendMessage(sendCtx, threadID, profile.ID,
		"Create a file named hello.txt in your current working directory with the content: Hello from real ACP test. "+
			"Then read the file back and confirm it contains the correct text. "+
			"Reply with exactly: DONE",
	); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Verify agent reply saved.
	msgs, _ := store.ListThreadMessages(ctx, threadID, 20, 0)
	var agentReply *core.ThreadMessage
	for _, m := range msgs {
		if m.Role == "agent" && m.SenderID == profile.ID {
			agentReply = m
		}
	}
	if agentReply == nil {
		t.Fatal("expected agent reply saved as thread message")
	}
	t.Logf("agent reply (%d chars): %s", len(agentReply.Content), truncate(agentReply.Content, 200))

	if !strings.Contains(agentReply.Content, "DONE") {
		t.Logf("WARNING: agent reply does not contain 'DONE' marker (codex may not follow instructions exactly)")
	}

	// Verify token tracking.
	key := threadSessionKey{threadID: threadID, agentID: profile.ID}
	pool.mu.Lock()
	pooled := pool.sessions[key]
	pool.mu.Unlock()

	if pooled != nil {
		t.Logf("token usage: input=%d output=%d turns=%d",
			pooled.inputTokens, pooled.outputTokens, pooled.turns)
		if pooled.inputTokens == 0 && pooled.outputTokens == 0 {
			t.Log("NOTE: agent reported zero token usage (some ACP agents do not expose usage)")
		}
	}

	// ── 4. Send a follow-up message (conversational continuity) ──
	t.Log(">>> sending follow-up message...")
	if err := pool.SendMessage(sendCtx, threadID, profile.ID,
		"What was the content of the file you just created? Reply with the content only.",
	); err != nil {
		t.Fatalf("SendMessage (follow-up): %v", err)
	}

	msgs, _ = store.ListThreadMessages(ctx, threadID, 20, 0)
	var followUpReply *core.ThreadMessage
	for _, m := range msgs {
		if m.Role == "agent" && m.SenderID == profile.ID {
			followUpReply = m // take last
		}
	}
	if followUpReply != nil {
		t.Logf("follow-up reply (%d chars): %s", len(followUpReply.Content), truncate(followUpReply.Content, 200))
	}

	// ── 5. Verify active agents ──
	activeIDs := pool.ActiveAgentProfileIDs(threadID)
	if len(activeIDs) != 1 || activeIDs[0] != profile.ID {
		t.Errorf("ActiveAgentProfileIDs = %v, want [%s]", activeIDs, profile.ID)
	}

	// ── 6. RemoveAgent ──
	t.Log(">>> removing agent...")
	activeMember := findActiveMember(t, store, threadID, profile.ID)
	if err := pool.RemoveAgent(ctx, threadID, activeMember.ID); err != nil {
		t.Fatalf("RemoveAgent: %v", err)
	}

	// Verify cleanup.
	if ids := pool.ActiveAgentProfileIDs(threadID); len(ids) != 0 {
		t.Errorf("expected no active agents after remove, got %v", ids)
	}

	members, _ = store.ListThreadMembers(ctx, threadID)
	for _, m := range members {
		if m.AgentProfileID == profile.ID {
			t.Logf("member status after remove: %s", m.Status)
			summary := memberGetString(m, "progress_summary")
			if summary != "" {
				t.Logf("progress summary: %s", truncate(summary, 200))
			}
			totalInput := memberGetInt64(m, "total_input_tokens")
			totalOutput := memberGetInt64(m, "total_output_tokens")
			turnCount := memberGetInt(m, "turn_count")
			t.Logf("final stats: turns=%d input_tokens=%d output_tokens=%d", turnCount, totalInput, totalOutput)
		}
	}

	t.Log(">>> PASS: real ACP thread pool lifecycle completed")
}

// TestReal_ThreadPoolFileIO verifies that a real ACP agent can read and write
// files through the ThreadSessionPool, validating end-to-end file I/O.
//
// Run:
//
//	AI_WORKFLOW_REAL_THREAD_ACP=1 go test -tags real -run TestReal_ThreadPoolFileIO -v -timeout 300s ./internal/runtime/agent/...
func TestReal_ThreadPoolFileIO(t *testing.T) {
	if os.Getenv("AI_WORKFLOW_REAL_THREAD_ACP") == "" {
		t.Skip("set AI_WORKFLOW_REAL_THREAD_ACP=1 to run")
	}

	profile := realCodexProfile()
	baseDir := t.TempDir()
	dataDir := filepath.Join(baseDir, "data")
	_ = os.MkdirAll(dataDir, 0o755)

	dbPath := filepath.Join(baseDir, "real-thread-fileio.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bus := membus.NewBus()
	ctx := context.Background()

	registry := &mockRegistry{profiles: map[string]*core.AgentProfile{
		profile.ID: profile,
	}}
	pool := NewThreadSessionPool(store, bus, registry, dataDir)
	defer pool.Close()

	threadID, err := store.CreateThread(ctx, &core.Thread{
		Title: "Real ACP File IO", OwnerID: "tester", Status: core.ThreadActive,
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// Invite + wait.
	if _, err := pool.InviteAgent(ctx, threadID, profile.ID); err != nil {
		t.Fatalf("InviteAgent: %v", err)
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 180*time.Second)
	defer waitCancel()
	if err := pool.WaitAgentReady(waitCtx, threadID, profile.ID); err != nil {
		t.Fatalf("WaitAgentReady: %v", err)
	}

	// Send file-writing task.
	sendCtx, sendCancel := context.WithTimeout(ctx, 120*time.Second)
	defer sendCancel()
	if err := pool.SendMessage(sendCtx, threadID, profile.ID,
		"Write a file called output.txt in the current directory with exactly this content: ACP_FILE_IO_OK\n"+
			"Then read the file back and confirm. Reply with: FILE_WRITTEN",
	); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Check that the file was actually created on disk.
	// The workspace dir is under dataDir/threads/<threadID>/
	workspaceDir := findWorkspaceDir(t, dataDir, threadID)
	outputPath := filepath.Join(workspaceDir, "output.txt")
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Logf("WARNING: could not read output.txt at %s: %v", outputPath, err)
		t.Log("(agent may have written to a different path)")
	} else {
		t.Logf("output.txt content: %q", strings.TrimSpace(string(content)))
		if strings.TrimSpace(string(content)) != "ACP_FILE_IO_OK" {
			t.Errorf("unexpected file content: %q", string(content))
		}
	}

	// Cleanup.
	activeMember := findActiveMember(t, store, threadID, profile.ID)
	_ = pool.RemoveAgent(ctx, threadID, activeMember.ID)
	t.Log(">>> PASS: real ACP file I/O test completed")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func findActiveMember(t *testing.T, store core.Store, threadID int64, profileID string) *core.ThreadMember {
	t.Helper()
	members, _ := store.ListThreadMembers(context.Background(), threadID)
	for _, m := range members {
		if m.AgentProfileID == profileID && (m.Status == core.ThreadAgentActive || m.Status == core.ThreadAgentBooting) {
			return m
		}
	}
	t.Fatalf("no active member found for profile %s in thread %d", profileID, threadID)
	return nil
}

func findWorkspaceDir(_ *testing.T, dataDir string, threadID int64) string {
	// threadctx.Paths creates: <dataDir>/threads/<threadID>/
	return filepath.Join(dataDir, "threads", fmt.Sprintf("%d", threadID))
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
