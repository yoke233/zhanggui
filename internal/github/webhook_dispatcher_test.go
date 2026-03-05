package github

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/eventbus"
)

func TestWebhookDispatcher_IssueEvents_SerializedByIssueNumber(t *testing.T) {
	var (
		mu    sync.Mutex
		order []string
	)
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})

	dispatcher := NewWebhookDispatcher(WebhookDispatcherOptions{
		Handler: WebhookDispatchHandlerFunc(func(_ context.Context, req WebhookDispatchRequest) error {
			mu.Lock()
			order = append(order, req.DeliveryID)
			mu.Unlock()

			switch req.DeliveryID {
			case "delivery-1":
				close(firstStarted)
				<-firstRelease
			case "delivery-2":
				close(secondStarted)
			}
			return nil
		}),
	})
	defer dispatcher.Close()

	req1 := testWebhookDispatchRequest(t, "proj-1", "issues", "opened", "delivery-1", "acme", "demo", 101)
	req2 := testWebhookDispatchRequest(t, "proj-1", "issues", "edited", "delivery-2", "acme", "demo", 101)

	firstDone := make(chan error, 1)
	go func() {
		_, err := dispatcher.Dispatch(context.Background(), req1)
		firstDone <- err
	}()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first request did not start in time")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := dispatcher.Dispatch(context.Background(), req2)
		secondDone <- err
	}()

	select {
	case <-secondStarted:
		t.Fatal("second request started before first completed; same issue should be serialized")
	case <-time.After(80 * time.Millisecond):
	}

	close(firstRelease)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first dispatch returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first dispatch timeout")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second dispatch returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second dispatch timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 {
		t.Fatalf("expected 2 handler calls, got %d", len(order))
	}
	if order[0] != "delivery-1" || order[1] != "delivery-2" {
		t.Fatalf("expected serial order [delivery-1 delivery-2], got %v", order)
	}
}

func TestWebhookDispatcher_DifferentIssues_CanRunInParallel(t *testing.T) {
	var current int32
	var maxConcurrent int32
	started := make(chan string, 2)
	release := make(chan struct{})

	dispatcher := NewWebhookDispatcher(WebhookDispatcherOptions{
		Handler: WebhookDispatchHandlerFunc(func(_ context.Context, req WebhookDispatchRequest) error {
			concurrency := atomic.AddInt32(&current, 1)
			for {
				snapshot := atomic.LoadInt32(&maxConcurrent)
				if concurrency <= snapshot || atomic.CompareAndSwapInt32(&maxConcurrent, snapshot, concurrency) {
					break
				}
			}

			started <- req.DeliveryID
			<-release
			atomic.AddInt32(&current, -1)
			return nil
		}),
	})
	defer dispatcher.Close()

	reqA := testWebhookDispatchRequest(t, "proj-1", "issues", "opened", "delivery-a", "acme", "demo", 101)
	reqB := testWebhookDispatchRequest(t, "proj-1", "issues", "opened", "delivery-b", "acme", "demo", 102)

	doneA := make(chan error, 1)
	doneB := make(chan error, 1)
	go func() {
		_, err := dispatcher.Dispatch(context.Background(), reqA)
		doneA <- err
	}()
	go func() {
		_, err := dispatcher.Dispatch(context.Background(), reqB)
		doneB <- err
	}()

	timeout := time.After(time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-timeout:
			t.Fatal("expected both issues to start in parallel")
		}
	}

	close(release)

	select {
	case err := <-doneA:
		if err != nil {
			t.Fatalf("dispatch A returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatch A timeout")
	}
	select {
	case err := <-doneB:
		if err != nil {
			t.Fatalf("dispatch B returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatch B timeout")
	}

	if atomic.LoadInt32(&maxConcurrent) < 2 {
		t.Fatalf("expected concurrent processing >=2, got %d", maxConcurrent)
	}
}

func TestWebhookDispatcher_DeduplicatesDeliveryID(t *testing.T) {
	var called int32
	dispatcher := NewWebhookDispatcher(WebhookDispatcherOptions{
		Handler: WebhookDispatchHandlerFunc(func(_ context.Context, _ WebhookDispatchRequest) error {
			atomic.AddInt32(&called, 1)
			return nil
		}),
	})
	defer dispatcher.Close()

	req := testWebhookDispatchRequest(t, "proj-1", "issues", "opened", "delivery-dup", "acme", "demo", 101)

	first, err := dispatcher.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("first dispatch returned error: %v", err)
	}
	if first.Duplicate {
		t.Fatal("first dispatch should not be duplicate")
	}

	second, err := dispatcher.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("second dispatch returned error: %v", err)
	}
	if !second.Duplicate {
		t.Fatal("second dispatch should be deduplicated")
	}

	if atomic.LoadInt32(&called) != 1 {
		t.Fatalf("expected handler called once, got %d", called)
	}
}

func TestWebhookDispatcher_FailedEvent_PushedToDLQ(t *testing.T) {
	dlqStore := NewInMemoryDLQStore()
	dispatcher := NewWebhookDispatcher(WebhookDispatcherOptions{
		DLQStore: dlqStore,
		Handler: WebhookDispatchHandlerFunc(func(_ context.Context, _ WebhookDispatchRequest) error {
			return errors.New("dispatcher panic")
		}),
	})
	defer dispatcher.Close()

	req := testWebhookDispatchRequest(t, "proj-dlq", "issues", "opened", "delivery-dlq-1", "acme", "demo", 301)
	_, err := dispatcher.Dispatch(context.Background(), req)
	if err == nil {
		t.Fatal("expected dispatch error")
	}

	entry, getErr := dlqStore.GetByDeliveryID(context.Background(), "delivery-dlq-1")
	if getErr != nil {
		t.Fatalf("GetByDeliveryID() error = %v", getErr)
	}
	if entry.EventType != "issues" || entry.Action != "opened" {
		t.Fatalf("unexpected dlq entry event fields: %+v", entry)
	}
	if entry.IssueNumber != 301 {
		t.Fatalf("IssueNumber = %d, want 301", entry.IssueNumber)
	}
	if entry.ProjectID != "proj-dlq" {
		t.Fatalf("ProjectID = %q, want %q", entry.ProjectID, "proj-dlq")
	}
}

func TestWebhookDispatcher_PublishesEventGitHubWebhookReceived(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	sub, err := bus.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	dispatcher := NewWebhookDispatcher(WebhookDispatcherOptions{
		Publisher: bus,
		Handler: WebhookDispatchHandlerFunc(func(_ context.Context, _ WebhookDispatchRequest) error {
			return nil
		}),
	})
	defer dispatcher.Close()

	req := testWebhookDispatchRequest(t, "proj-123", "issues", "opened", "delivery-evt", "acme", "demo", 101)
	if _, err := dispatcher.Dispatch(context.Background(), req); err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}

	select {
	case evt := <-sub.C:
		if evt.Type != core.EventGitHubWebhookReceived {
			t.Fatalf("expected event type %s, got %s", core.EventGitHubWebhookReceived, evt.Type)
		}
		if evt.ProjectID != "proj-123" {
			t.Fatalf("expected project_id proj-123, got %s", evt.ProjectID)
		}
		if evt.Data["delivery_id"] != "delivery-evt" {
			t.Fatalf("expected delivery_id delivery-evt, got %q", evt.Data["delivery_id"])
		}
		if evt.Data["issue_number"] != "101" {
			t.Fatalf("expected issue_number 101, got %q", evt.Data["issue_number"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for github_webhook_received event")
	}
}

func TestWebhookDispatcher_CleansIssueMutexAfterCloseOrRunDone(t *testing.T) {
	t.Run("issue closed triggers delayed cleanup", func(t *testing.T) {
		dispatcher := NewWebhookDispatcher(WebhookDispatcherOptions{
			CleanupDelay: 20 * time.Millisecond,
			Handler: WebhookDispatchHandlerFunc(func(_ context.Context, _ WebhookDispatchRequest) error {
				return nil
			}),
		})
		defer dispatcher.Close()

		opened := testWebhookDispatchRequest(t, "proj-1", "issues", "opened", "delivery-opened", "acme", "demo", 101)
		if _, err := dispatcher.Dispatch(context.Background(), opened); err != nil {
			t.Fatalf("opened dispatch returned error: %v", err)
		}
		if !hasIssueLock(dispatcher, "acme/demo#101") {
			t.Fatal("expected issue lock to exist after opened event")
		}

		closed := testWebhookDispatchRequest(t, "proj-1", "issues", "closed", "delivery-closed", "acme", "demo", 101)
		if _, err := dispatcher.Dispatch(context.Background(), closed); err != nil {
			t.Fatalf("closed dispatch returned error: %v", err)
		}

		waitFor(t, time.Second, func() bool {
			return !hasIssueLock(dispatcher, "acme/demo#101")
		})
	})

	t.Run("Run done triggers delayed cleanup", func(t *testing.T) {
		bus := eventbus.New()
		defer bus.Close()

		dispatcher := NewWebhookDispatcher(WebhookDispatcherOptions{
			RunEvents:    bus,
			CleanupDelay: 20 * time.Millisecond,
			Handler: WebhookDispatchHandlerFunc(func(_ context.Context, _ WebhookDispatchRequest) error {
				return nil
			}),
		})
		defer dispatcher.Close()

		opened := testWebhookDispatchRequest(t, "proj-1", "issues", "opened", "delivery-opened-2", "acme", "demo", 202)
		if _, err := dispatcher.Dispatch(context.Background(), opened); err != nil {
			t.Fatalf("opened dispatch returned error: %v", err)
		}
		if !hasIssueLock(dispatcher, "acme/demo#202") {
			t.Fatal("expected issue lock to exist after opened event")
		}

		bus.Publish(context.Background(), core.Event{
			Type: core.EventRunDone,
			Data: map[string]string{
				"github_owner": "acme",
				"github_repo":  "demo",
				"issue_number": "202",
			},
			Timestamp: time.Now(),
		})

		waitFor(t, time.Second, func() bool {
			return !hasIssueLock(dispatcher, "acme/demo#202")
		})
	})
}

func hasIssueLock(dispatcher *WebhookDispatcher, key string) bool {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	_, ok := dispatcher.issueLocks[key]
	return ok
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not satisfied before timeout")
}

func testWebhookDispatchRequest(
	t *testing.T,
	projectID string,
	eventType string,
	action string,
	deliveryID string,
	owner string,
	repo string,
	issueNumber int,
) WebhookDispatchRequest {
	t.Helper()

	payload := map[string]any{
		"repository": map[string]any{
			"name": repo,
			"owner": map[string]any{
				"login": owner,
			},
		},
		"issue": map[string]any{
			"number": issueNumber,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}

	return WebhookDispatchRequest{
		ProjectID:  projectID,
		EventType:  eventType,
		Action:     action,
		DeliveryID: deliveryID,
		Payload:    raw,
		ReceivedAt: time.Now(),
	}
}
