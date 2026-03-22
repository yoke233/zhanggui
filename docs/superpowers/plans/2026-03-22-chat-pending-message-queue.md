# Chat Pending Message Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a user sends a message to a running ACP chat session, queue it and auto-dispatch when the current run finishes, instead of returning an error.

**Architecture:** Backend LeadAgent stores at most one pending message per session. When `StartChat` detects the session is already running, it saves the request as pending and returns `status: "queued"`. When `endRun` fires, it atomically checks for a pending message and re-registers as running before releasing the lock, then auto-dispatches. Frontend handles `pending_dispatched` inside the existing `chat.output` subscriber (bridge events arrive as `chat.output` with inner `type` field). `chat.pending_cancelled` is a direct WS message. Error banner gets a close button.

**Tech Stack:** Go (backend), React + TypeScript + Tailwind (frontend), WebSocket events

---

## Key Design Decisions

### Event Routing
- **`pending_dispatched`** — published via `sess.bridge.PublishData(...)` → arrives on WS as `{type: "chat.output", data: {type: "pending_dispatched", session_id: "..."}}`. Must be handled inside the **existing `chat.output` subscriber** by checking `updateType === "pending_dispatched"`.
- **`chat.pending_cancelled`** — sent directly from WS handler via `writeJSON(wsOutboundMessage{Type: "chat.pending_cancelled", ...})`. Handled as a separate top-level WS subscription.

### Race Condition Prevention
`endRun` must atomically check for pending and re-register in `activeRuns` **before releasing `activeMu`**. This prevents a concurrent `StartChat` from sneaking in between lock release and dispatch.

### Session Cleanup
`removeSession`, `CloseSession`, and `DeleteSession` must clear any pending message to avoid orphaned state and stale frontend banners.

---

## File Map

### Backend — Modified
| File | Responsibility |
|------|----------------|
| `internal/application/chat/contracts.go` | Add `PendingMessage` struct, add `Status` to `AcceptedResponse` |
| `internal/adapters/chat/acp/lead.go` | Add pending map, modify `StartChat` / `endRun`, add `CancelPending`, clean pending on remove |
| `internal/adapters/http/contracts.go` | Add `CancelPending` to `LeadChatService` interface |
| `internal/adapters/http/event.go` | Add `chat.cancel_pending` WS handler, use `accepted.Status` in ack |
| `internal/adapters/http/chat_test.go` | Add stub `CancelPending` method |

### Backend — New
| File | Responsibility |
|------|----------------|
| `internal/adapters/chat/acp/lead_pending_test.go` | Unit tests for pending message logic |

### Frontend — Modified
| File | Responsibility |
|------|----------------|
| `web/src/components/chat/chatTypes.ts` | Add `PendingMessageView` type |
| `web/src/pages/ChatPage.tsx` | Handle `queued` ack, pending events in `chat.output`, fix "already running" bug, error close button, pending banner |
| `web/src/components/chat/ChatInputBar.tsx` | Render pending message banner above input |
| `web/src/i18n/locales/zh-CN.json` | Add i18n keys |
| `web/src/i18n/locales/en.json` | Add i18n keys |

---

## Task 1: Backend — Add PendingMessage type and AcceptedResponse.Status

**Files:**
- Modify: `internal/application/chat/contracts.go:78-82`

- [ ] **Step 1: Add PendingMessage struct and Status field**

In `internal/application/chat/contracts.go`, add `PendingMessage` struct and add `Status` to `AcceptedResponse`:

```go
// AcceptedResponse is returned when a chat request has been accepted for async execution.
type AcceptedResponse struct {
	SessionID string `json:"session_id"`
	WSPath    string `json:"ws_path,omitempty"`
	// Status is "accepted" for immediate execution or "queued" when the session is busy.
	Status string `json:"status"`
}

// PendingMessage holds a queued message waiting for a busy session to become idle.
type PendingMessage struct {
	Message     string       `json:"message"`
	Attachments []Attachment `json:"attachments,omitempty"`
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/application/chat/...`
Expected: PASS (no errors)

- [ ] **Step 3: Commit**

```bash
git add internal/application/chat/contracts.go
git commit -m "feat(chat): add PendingMessage type and Status to AcceptedResponse"
```

---

## Task 2: Backend — Add pending message logic to LeadAgent

**Files:**
- Modify: `internal/adapters/chat/acp/lead.go` (struct, `StartChat`, `endRun`, `removeSession`, new helpers)
- Create: `internal/adapters/chat/acp/lead_pending_test.go`

- [ ] **Step 1: Add pending fields to LeadAgent struct**

In `internal/adapters/chat/acp/lead.go`, add to the `LeadAgent` struct (after `activeRuns`):

```go
type LeadAgent struct {
	cfg    LeadAgentConfig
	broker *permissionBroker

	mu          sync.Mutex
	sessions    map[string]*leadSession
	catalog     map[string]*persistedLeadSession
	catalogPath string

	activeMu   sync.Mutex
	activeRuns map[string]context.CancelFunc

	pendingMu   sync.Mutex
	pendingMsgs map[string]*chatapp.PendingMessage // at most 1 per session
}
```

Initialize the map in the constructor (find `NewLeadAgent` or equivalent init function) — add:
```go
pendingMsgs: make(map[string]*chatapp.PendingMessage),
```

- [ ] **Step 2: Add pending helper methods**

Add these methods to `lead.go` (after `endRun`):

```go
func (l *LeadAgent) setPending(sessionID string, msg *chatapp.PendingMessage) {
	l.pendingMu.Lock()
	l.pendingMsgs[strings.TrimSpace(sessionID)] = msg
	l.pendingMu.Unlock()
}

func (l *LeadAgent) takePending(sessionID string) *chatapp.PendingMessage {
	id := strings.TrimSpace(sessionID)
	l.pendingMu.Lock()
	msg := l.pendingMsgs[id]
	delete(l.pendingMsgs, id)
	l.pendingMu.Unlock()
	return msg
}

func (l *LeadAgent) CancelPending(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	l.pendingMu.Lock()
	_, existed := l.pendingMsgs[id]
	delete(l.pendingMsgs, id)
	l.pendingMu.Unlock()
	return existed
}
```

- [ ] **Step 3: Modify StartChat to queue when busy**

Replace the current `StartChat` method. The key change: after `prepareChat` succeeds, check if the session is currently running. If so, save the message as pending and return `Status: "queued"` instead of spawning a goroutine. Do **NOT** call `appendMessage` here — `runPrompt` will do it when the pending message is dispatched.

```go
func (l *LeadAgent) StartChat(ctx context.Context, req chatapp.Request) (*chatapp.AcceptedResponse, error) {
	sess, publicSessionID, message, err := l.prepareChat(ctx, req)
	if err != nil {
		return nil, err
	}

	// If session is busy, queue the message for later dispatch.
	if l.IsSessionRunning(publicSessionID) {
		l.setPending(publicSessionID, &chatapp.PendingMessage{
			Message:     message,
			Attachments: req.Attachments,
		})
		return &chatapp.AcceptedResponse{
			SessionID: publicSessionID,
			WSPath:    buildChatWSPath(publicSessionID),
			Status:    "queued",
		}, nil
	}

	attachments := req.Attachments
	go func() {
		if _, runErr := l.runPrompt(context.Background(), publicSessionID, sess, message, attachments); runErr != nil {
			sess.bridge.PublishData(context.Background(), map[string]any{
				"type":    "error",
				"content": runErr.Error(),
			})
			slog.Warn("lead chat async prompt failed", "session_id", publicSessionID, "error", runErr)
		}
	}()

	return &chatapp.AcceptedResponse{
		SessionID: publicSessionID,
		WSPath:    buildChatWSPath(publicSessionID),
		Status:    "accepted",
	}, nil
}
```

- [ ] **Step 4: Modify endRun to atomically check and dispatch pending**

Replace `endRun`. Critical: hold `activeMu` while checking for pending and re-registering as running, to prevent a race where a concurrent `StartChat` sneaks in between lock release and dispatch.

```go
func (l *LeadAgent) endRun(sessionID string) {
	id := strings.TrimSpace(sessionID)

	// Atomically: remove from activeRuns, check pending, re-register if pending exists.
	l.pendingMu.Lock()
	pending := l.pendingMsgs[id]
	delete(l.pendingMsgs, id)
	l.pendingMu.Unlock()

	if pending == nil {
		// No pending — just clear the active run.
		l.activeMu.Lock()
		delete(l.activeRuns, id)
		l.activeMu.Unlock()
		return
	}

	// Has pending — swap the cancel func in activeRuns (session stays "running").
	dispatchCtx, dispatchCancel := context.WithCancel(context.Background())
	l.activeMu.Lock()
	delete(l.activeRuns, id)
	l.activeRuns[id] = dispatchCancel
	l.activeMu.Unlock()

	l.mu.Lock()
	sess := l.sessions[id]
	l.mu.Unlock()
	if sess == nil {
		// Session was removed — clean up and bail.
		dispatchCancel()
		l.activeMu.Lock()
		delete(l.activeRuns, id)
		l.activeMu.Unlock()
		return
	}

	// Notify frontend the pending message is now being dispatched.
	sess.bridge.PublishData(context.Background(), map[string]any{
		"type": "pending_dispatched",
	})

	go func() {
		// endRun is deferred inside runPrompt — it will clean up activeRuns.
		// But we already registered above, so runPrompt's beginRun would fail.
		// Instead, call the prompt logic directly without beginRun.
		defer l.endRunSimple(id)
		defer dispatchCancel()

		l.appendMessage(id, "user", pending.Message)
		promptBlocks := buildPromptBlocks(pending.Message, pending.Attachments, sess.workDir)

		result, err := sess.client.Prompt(dispatchCtx, acpproto.PromptRequest{
			SessionId: sess.sessionID,
			Prompt:    promptBlocks,
		})

		sess.bridge.FlushPending(context.Background())

		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				l.resetSessionIdle(id, sess)
			} else {
				l.removeSession(id)
			}
			sess.bridge.PublishData(context.Background(), map[string]any{
				"type":    "error",
				"content": fmt.Sprintf("pending dispatch failed: %v", err),
			})
			slog.Warn("lead chat pending dispatch failed", "session_id", id, "error", err)
			return
		}
		if result == nil {
			l.removeSession(id)
			sess.bridge.PublishData(context.Background(), map[string]any{
				"type":    "error",
				"content": "empty result from agent",
			})
			return
		}

		reply := strings.TrimSpace(result.Text)
		if reply == "" {
			l.removeSession(id)
			sess.bridge.PublishData(context.Background(), map[string]any{
				"type":    "error",
				"content": "empty reply from agent",
			})
			return
		}

		sess.bridge.PublishData(context.Background(), map[string]any{
			"type": "done",
		})
		l.appendMessage(id, "assistant", reply)
		l.resetSessionIdle(id, sess)
	}()
}

// endRunSimple removes the session from activeRuns without checking pending.
// Used by dispatched pending goroutines to avoid infinite recursion.
func (l *LeadAgent) endRunSimple(sessionID string) {
	id := strings.TrimSpace(sessionID)

	// Check if there's ANOTHER pending message queued while we were running.
	l.pendingMu.Lock()
	pending := l.pendingMsgs[id]
	delete(l.pendingMsgs, id)
	l.pendingMu.Unlock()

	if pending == nil {
		l.activeMu.Lock()
		delete(l.activeRuns, id)
		l.activeMu.Unlock()
		return
	}

	// Recursively dispatch — reuse endRun logic but via a fresh goroutine.
	// Store the pending back and call endRun which will pick it up.
	l.setPending(id, pending)
	l.activeMu.Lock()
	delete(l.activeRuns, id)
	l.activeMu.Unlock()
	l.endRun(id) // will find the pending and dispatch it
}
```

**Wait — this is getting too complex.** Simpler approach: since `runPrompt` already contains all the prompt+error+done logic and defers `endRun`, just reuse it. The issue is `beginRun` inside `runPrompt` — but since we already registered in `activeRuns`, it will fail. Better approach: **skip `runPrompt` and just use `endRun` recursively with a simple guard**:

**Actually, simplest correct approach:**

```go
func (l *LeadAgent) endRun(sessionID string) {
	id := strings.TrimSpace(sessionID)

	// Atomically: clear active run, check for pending, re-register if found.
	l.pendingMu.Lock()
	pending := l.pendingMsgs[id]
	delete(l.pendingMsgs, id)
	l.pendingMu.Unlock()

	if pending == nil {
		l.activeMu.Lock()
		delete(l.activeRuns, id)
		l.activeMu.Unlock()
		return
	}

	// Session stays "running" — swap the cancel func atomically.
	dispatchCtx, dispatchCancel := context.WithCancel(context.Background())
	l.activeMu.Lock()
	delete(l.activeRuns, id)
	l.activeRuns[id] = dispatchCancel
	l.activeMu.Unlock()

	l.mu.Lock()
	sess := l.sessions[id]
	l.mu.Unlock()
	if sess == nil {
		dispatchCancel()
		l.activeMu.Lock()
		delete(l.activeRuns, id)
		l.activeMu.Unlock()
		return
	}

	sess.bridge.PublishData(context.Background(), map[string]any{
		"type": "pending_dispatched",
	})

	go l.runPending(dispatchCtx, dispatchCancel, id, sess, pending)
}

func (l *LeadAgent) runPending(ctx context.Context, cancel context.CancelFunc, sessionID string, sess *leadSession, pending *chatapp.PendingMessage) {
	defer l.endRun(sessionID) // recursive: will check for next pending
	defer cancel()

	l.appendMessage(sessionID, "user", pending.Message)
	promptBlocks := buildPromptBlocks(pending.Message, pending.Attachments, sess.workDir)

	result, err := sess.client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sess.sessionID,
		Prompt:    promptBlocks,
	})
	sess.bridge.FlushPending(context.Background())

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			l.resetSessionIdle(sessionID, sess)
		} else {
			l.removeSession(sessionID)
		}
		sess.bridge.PublishData(context.Background(), map[string]any{
			"type":    "error",
			"content": fmt.Sprintf("pending dispatch failed: %v", err),
		})
		return
	}
	if result == nil {
		l.removeSession(sessionID)
		sess.bridge.PublishData(context.Background(), map[string]any{
			"type":    "error",
			"content": "empty result from agent",
		})
		return
	}

	reply := strings.TrimSpace(result.Text)
	if reply == "" {
		l.removeSession(sessionID)
		sess.bridge.PublishData(context.Background(), map[string]any{
			"type":    "error",
			"content": "empty reply from agent",
		})
		return
	}

	sess.bridge.PublishData(context.Background(), map[string]any{"type": "done"})
	l.appendMessage(sessionID, "assistant", reply)
	l.resetSessionIdle(sessionID, sess)
}
```

Key points:
- `endRun` atomically checks pending and re-registers as running **before** releasing `activeMu`
- `runPending` is a streamlined version of `runPrompt` that skips `beginRun` (already registered)
- `runPending` defers `endRun` → supports chained pending messages
- No need to modify `runPrompt` at all

- [ ] **Step 5: Clean pending on session removal**

In `removeSession` (line 1164), add pending cleanup at the top:

```go
func (l *LeadAgent) removeSession(sessionID string) {
	if sessionID == "" {
		return
	}
	l.takePending(sessionID) // discard any orphaned pending message
	l.mu.Lock()
	sess, ok := l.sessions[sessionID]
	if ok {
		delete(l.sessions, sessionID)
	}
	l.mu.Unlock()
	if sess != nil {
		sess.close()
	}
}
```

Since `CloseSession` and `DeleteSession` both call `removeSession` internally, this covers all removal paths.

- [ ] **Step 6: Write unit test for pending logic**

Create `internal/adapters/chat/acp/lead_pending_test.go`:

```go
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
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/adapters/chat/acp/... -run TestPending -v`
Expected: All 3 tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/chat/acp/lead.go internal/adapters/chat/acp/lead_pending_test.go
git commit -m "feat(chat): add pending message queue to LeadAgent"
```

---

## Task 3: Backend — Add CancelPending to interface and WS handler

**Files:**
- Modify: `internal/adapters/http/contracts.go:55-72`
- Modify: `internal/adapters/http/event.go:266-292` (router) and ~line 349 (ack status)
- Modify: `internal/adapters/http/chat_test.go` (stub)

- [ ] **Step 1: Add CancelPending to LeadChatService interface**

In `internal/adapters/http/contracts.go`, add to the `LeadChatService` interface:

```go
CancelPending(sessionID string) bool
```

- [ ] **Step 2: Add stub to test helper**

In `internal/adapters/http/chat_test.go`, add to `stubLeadChatService`:

```go
func (s *stubLeadChatService) CancelPending(string) bool { return false }
```

- [ ] **Step 3: Use accepted.Status in WS ack handler**

In `internal/adapters/http/event.go`, in `handleWSChatSend`, replace the hardcoded `Status: "accepted"` with the value from the response:

```go
_ = writeJSON(wsOutboundMessage{
	Type: "chat.ack",
	Data: wsChatAckPayload{
		RequestID: strings.TrimSpace(req.RequestID),
		SessionID: accepted.SessionID,
		WSPath:    accepted.WSPath,
		Status:    accepted.Status,
	},
})
```

- [ ] **Step 4: Add chat.cancel_pending to WS router**

In `handleWSClientMessage` switch block, add:

```go
case "chat.cancel_pending":
	h.handleWSChatCancelPending(msg, writeJSON)
```

- [ ] **Step 5: Add handleWSChatCancelPending handler**

Add new function in `event.go`:

```go
func (h *Handler) handleWSChatCancelPending(msg wsMessage, writeJSON func(v any) error) {
	if h.lead == nil {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{Code: "CHAT_DISABLED", Error: "lead chat service is not configured"},
		})
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			_ = writeJSON(wsOutboundMessage{
				Type: "chat.error",
				Data: wsErrorPayload{Code: "BAD_REQUEST", Error: "invalid payload"},
			})
			return
		}
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.error",
			Data: wsErrorPayload{Code: "BAD_REQUEST", Error: "session_id is required"},
		})
		return
	}

	if h.lead.CancelPending(sessionID) {
		_ = writeJSON(wsOutboundMessage{
			Type: "chat.pending_cancelled",
			Data: map[string]string{"session_id": sessionID},
		})
	}
}
```

- [ ] **Step 6: Verify compilation**

Run: `go build ./internal/adapters/http/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/http/contracts.go internal/adapters/http/event.go internal/adapters/http/chat_test.go
git commit -m "feat(chat): add CancelPending WS handler and use AcceptedResponse.Status"
```

---

## Task 4: Frontend — Add PendingMessageView type and i18n keys

**Files:**
- Modify: `web/src/components/chat/chatTypes.ts`
- Modify: `web/src/i18n/locales/zh-CN.json`
- Modify: `web/src/i18n/locales/en.json`

- [ ] **Step 1: Add PendingMessageView type**

In `web/src/components/chat/chatTypes.ts`, add:

```typescript
export interface PendingMessageView {
  sessionId: string;
  content: string;
}
```

- [ ] **Step 2: Add i18n keys**

In `web/src/i18n/locales/zh-CN.json`, add to the `chat` section:

```json
"pendingSend": "待发送",
"pendingSendHint": "当前会话运行结束后将自动发送",
"pendingCancelled": "待发送消息已取消"
```

In `web/src/i18n/locales/en.json`, add to the `chat` section:

```json
"pendingSend": "Pending",
"pendingSendHint": "Will send automatically when current run finishes",
"pendingCancelled": "Pending message cancelled"
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/chat/chatTypes.ts web/src/i18n/locales/zh-CN.json web/src/i18n/locales/en.json
git commit -m "feat(chat): add PendingMessageView type and i18n keys"
```

---

## Task 5: Frontend — Handle queued ack and pending events in ChatPage

**Files:**
- Modify: `web/src/pages/ChatPage.tsx`

This is the core frontend task with 4 sub-changes.

### 5a: Add pending message state

- [ ] **Step 1: Add state and import**

At the top of the ChatPage component (near other state declarations around line 95-132), add:

```typescript
import type { PendingMessageView } from "@/components/chat/chatTypes";
```

```typescript
const [pendingMessage, setPendingMessage] = useState<PendingMessageView | null>(null);
```

### 5b: Handle `status: "queued"` in chat.ack handler

- [ ] **Step 2: Modify ack handler**

In the `chat.ack` subscriber (line ~722-781), the existing code structure is:
1. Validate request_id (lines 725-731)
2. Validate session_id (lines 732-735)
3. Clear pendingRequestIdRef (line 736)
4. Set submitting false, clear draft (lines 737-738)
5. Draft transfer + optimistic insert (lines 740-781)

Insert a queued check **after step 3** (after `pendingRequestIdRef.current = null`) but **before step 4**. The queued branch returns early with its own cleanup:

```typescript
    pendingRequestIdRef.current = null;

    // --- NEW: Handle queued status ---
    if (payload.status === "queued") {
      setPendingMessage({
        sessionId,
        content: pendingSendDraftRef.current?.messageInput ?? "",
      });
      setSubmitting(false);
      pendingSendDraftRef.current = null;
      return;
    }
    // --- END NEW ---

    setSubmitting(false);
    pendingSendDraftRef.current = null;
    // ... rest of existing ack logic unchanged ...
```

### 5c: Handle pending_dispatched inside chat.output subscriber

- [ ] **Step 3: Add pending_dispatched handler in chat.output subscriber**

**Important:** `pending_dispatched` arrives via `bridge.PublishData` as `{type: "chat.output", data: {type: "pending_dispatched", session_id: "..."}}`. It must be handled inside the **existing** `chat.output` subscriber, NOT as a separate top-level subscription.

In the `chat.output` subscriber (the one starting with `wsClient.subscribe("chat.output", ...)`), add a branch for `updateType === "pending_dispatched"` near the existing `done` / `error` handlers:

```typescript
        if (updateType === "pending_dispatched") {
          setPendingMessage(null);
          setSubmitting(true);
          setSessions((current) => touchSessionList(current, sessionId, "running", nowISO));
          return;
        }
```

Place this **before** the `done` handler so it's checked first.

### 5d: Subscribe to chat.pending_cancelled (direct WS message)

- [ ] **Step 4: Add pending_cancelled subscription**

`chat.pending_cancelled` is sent directly from the WS handler (not through EventBridge), so it arrives as a top-level WS message type. Add a **new subscription** in the same `useEffect`:

```typescript
const unsubscribePendingCancelled = wsClient.subscribe<{ session_id?: string }>(
  "chat.pending_cancelled",
  (payload) => {
    const sessionId = payload.session_id?.trim();
    if (sessionId) {
      setPendingMessage(null);
    }
  },
);
```

Add to the cleanup function:
```typescript
unsubscribePendingCancelled();
```

### 5e: Fix "already running" error handling bug

- [ ] **Step 5: Fix chat.error handler**

In the `chat.error` subscriber (line ~783-806), the code currently marks the session as `"closed"` even when the error is "session is already running". Fix by returning early for this error without touching session status or showing an error:

```typescript
    pendingRequestIdRef.current = null;
    setSubmitting(false);
    const errorMessage = payload.error?.trim() || t("chat.sendFailed");

    // Backend now queues messages for busy sessions. This error should not
    // normally arrive, but as a safety net, silently ignore it.
    if (isSessionAlreadyRunningError(errorMessage)) {
      pendingSendDraftRef.current = null;
      return;
    }

    pendingSendDraftRef.current = null;
    setError(errorMessage);
    const sessionId = payload.session_id?.trim();
    if (sessionId) {
      setSessions((current) => touchSessionList(current, sessionId, "closed", new Date().toISOString()));
    }
```

- [ ] **Step 6: Add cancelPendingMessage function**

Near `cancelSession` (line ~1135), add:

```typescript
const cancelPendingMessage = () => {
  if (!pendingMessage) return;
  wsClient.send({
    type: "chat.cancel_pending",
    data: { session_id: pendingMessage.sessionId },
  });
  setPendingMessage(null); // optimistic
};
```

- [ ] **Step 7: Also fix sendMessage catch block**

In the `sendMessage` catch block (line ~1114-1125), apply the same safety-net fix:

```typescript
} catch (sendError) {
  pendingRequestIdRef.current = null;
  const errorMessage = getErrorMessage(sendError);
  if (isSessionAlreadyRunningError(errorMessage)) {
    pendingSendDraftRef.current = null;
    return;
  }
  pendingSendDraftRef.current = null;
  setError(errorMessage);
  if (workingSessionId) {
    setSessions((current) => touchSessionList(current, workingSessionId, "closed", new Date().toISOString()));
  }
}
```

- [ ] **Step 8: Commit**

```bash
git add web/src/pages/ChatPage.tsx
git commit -m "feat(chat): handle queued ack, pending events, fix already-running bug"
```

---

## Task 6: Frontend — Error banner close button + pending banner in ChatInputBar

**Files:**
- Modify: `web/src/pages/ChatPage.tsx:1430-1434` (error banner)
- Modify: `web/src/components/chat/ChatInputBar.tsx` (pending banner)

### 6a: Error banner close button

- [ ] **Step 1: Add close button to error banner**

In `ChatPage.tsx`, replace the current error banner (line ~1430-1434):

```tsx
{error && (
  <p className="mx-5 mt-4 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
    {error}
  </p>
)}
```

With:

```tsx
{error && (
  <div className="mx-5 mt-4 flex items-center gap-2 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
    <span className="min-w-0 flex-1">{error}</span>
    <button
      type="button"
      className="shrink-0 rounded p-0.5 text-rose-400 transition-colors hover:text-rose-600"
      onClick={() => setError(null)}
    >
      <X className="h-3.5 w-3.5" />
    </button>
  </div>
)}
```

Add `X` to the lucide-react imports at the top of ChatPage.tsx.

### 6b: Pending message banner in ChatInputBar

- [ ] **Step 2: Add pending message props to ChatInputBar**

In `ChatInputBar.tsx`, add to `ChatInputBarProps`:

```typescript
pendingMessage: PendingMessageView | null;
onCancelPending: () => void;
```

Add import:
```typescript
import type { PendingMessageView } from "./chatTypes";
```

Add `X` and `Clock` to the lucide-react imports.

Destructure in the function body:
```typescript
const {
  // ... existing props ...
  pendingMessage,
  onCancelPending,
} = props;
```

- [ ] **Step 3: Render pending banner**

In `ChatInputBar.tsx`, inside the `<div className="space-y-2 border-t px-6 py-4">`, add the pending banner BEFORE the `<FilePreviewList>`:

```tsx
{pendingMessage && (
  <div className="flex items-center gap-2 rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-sm text-sky-700">
    <Clock className="h-3.5 w-3.5 shrink-0" />
    <span className="min-w-0 flex-1 truncate">
      {t("chat.pendingSend")}: {pendingMessage.content}
    </span>
    <button
      type="button"
      className="shrink-0 rounded p-0.5 text-sky-400 transition-colors hover:text-sky-600"
      onClick={onCancelPending}
    >
      <X className="h-3.5 w-3.5" />
    </button>
  </div>
)}
```

- [ ] **Step 4: Pass props from ChatPage**

In `ChatPage.tsx`, find the single `<ChatInputBar>` instance (line ~1504) and add the new props:

```tsx
pendingMessage={pendingMessage}
onCancelPending={cancelPendingMessage}
```

- [ ] **Step 5: Verify frontend builds**

Run: `npm --prefix web run typecheck`
Expected: PASS

Run: `npm --prefix web run build`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/ChatPage.tsx web/src/components/chat/ChatInputBar.tsx
git commit -m "feat(chat): add error banner close button and pending message banner"
```

---

## Task 7: Integration verification

- [ ] **Step 1: Run backend tests**

Run: `pwsh -NoProfile -File ./scripts/test/backend-unit.ps1`
Expected: PASS

- [ ] **Step 2: Run frontend build**

Run: `npm --prefix web run build`
Expected: PASS

- [ ] **Step 3: Run frontend tests**

Run: `npm --prefix web run test`
Expected: PASS

- [ ] **Step 4: Final commit if needed**

Squash any fixup commits or create a final integration commit.

---

## Appendix: Flow Diagrams

### Normal send (session idle)
```
User → chat.send → StartChat → IsSessionRunning=false → goroutine(runPrompt) → ack{status:"accepted"}
```

### Send to busy session (queued)
```
User → chat.send → StartChat → IsSessionRunning=true → setPending → ack{status:"queued"}
  ... current run finishes ...
  runPrompt returns → endRun → takePending → re-register in activeRuns → bridge.PublishData("pending_dispatched") → goroutine(runPending)
  Frontend: chat.output{type:"pending_dispatched"} → clear banner, set submitting=true
```

### Cancel pending message
```
User clicks ✕ on banner → chat.cancel_pending{session_id} → CancelPending → chat.pending_cancelled{session_id}
Frontend: optimistically clears banner, confirmed by chat.pending_cancelled
```

### Cancel running session with pending
```
User clicks ■ → cancelChat API → context cancelled → runPrompt returns → endRun → takePending → dispatches pending
```
