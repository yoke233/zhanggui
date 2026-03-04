package github

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/observability"
)

const (
	defaultWebhookCleanupDelay = 5 * time.Minute
	defaultDeliveryTTL         = 15 * time.Minute
)

// WebhookDispatchRequest carries normalized webhook metadata and raw payload.
type WebhookDispatchRequest struct {
	ProjectID  string
	EventType  string
	Action     string
	DeliveryID string
	TraceID    string
	Payload    []byte
	ReceivedAt time.Time
}

// WebhookDispatchResult reports dispatcher-level decisions.
type WebhookDispatchResult struct {
	Duplicate bool
	IssueKey  string
}

// WebhookDispatchHandler handles one accepted webhook dispatch.
type WebhookDispatchHandler interface {
	HandleWebhook(ctx context.Context, req WebhookDispatchRequest) error
}

// WebhookDispatchHandlerFunc adapts a function into WebhookDispatchHandler.
type WebhookDispatchHandlerFunc func(ctx context.Context, req WebhookDispatchRequest) error

func (f WebhookDispatchHandlerFunc) HandleWebhook(ctx context.Context, req WebhookDispatchRequest) error {
	return f(ctx, req)
}

type webhookEventPublisher interface {
	Publish(evt core.Event)
}

type webhookRunEvents interface {
	Subscribe() chan core.Event
	Unsubscribe(ch chan core.Event)
}

// WebhookDispatcherOptions controls dispatcher behavior.
type WebhookDispatcherOptions struct {
	Handler      WebhookDispatchHandler
	Publisher    webhookEventPublisher
	RunEvents    webhookRunEvents
	DLQStore     DLQStore
	CleanupDelay time.Duration
	DeliveryTTL  time.Duration
	Now          func() time.Time
	AfterFunc    func(time.Duration, func())
}

type issueLockState struct {
	lock        sync.Mutex
	inFlight    int
	lastTouched time.Time
}

// WebhookDispatcher serializes issue events, deduplicates delivery IDs and emits core events.
type WebhookDispatcher struct {
	handler      WebhookDispatchHandler
	publisher    webhookEventPublisher
	dlqStore     DLQStore
	cleanupDelay time.Duration
	deliveryTTL  time.Duration
	now          func() time.Time
	afterFunc    func(time.Duration, func())

	mu             sync.Mutex
	issueLocks     map[string]*issueLockState
	seenDeliveries map[string]time.Time

	RunEvents webhookRunEvents
	Runsub    chan core.Event
	done      chan struct{}
	closeOnce sync.Once
}

func NewWebhookDispatcher(opts WebhookDispatcherOptions) *WebhookDispatcher {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	afterFn := opts.AfterFunc
	if afterFn == nil {
		afterFn = func(delay time.Duration, fn func()) {
			time.AfterFunc(delay, fn)
		}
	}

	cleanupDelay := opts.CleanupDelay
	if cleanupDelay <= 0 {
		cleanupDelay = defaultWebhookCleanupDelay
	}

	deliveryTTL := opts.DeliveryTTL
	if deliveryTTL <= 0 {
		deliveryTTL = defaultDeliveryTTL
	}

	handler := opts.Handler
	if handler == nil {
		handler = WebhookDispatchHandlerFunc(func(context.Context, WebhookDispatchRequest) error {
			return nil
		})
	}

	d := &WebhookDispatcher{
		handler:        handler,
		publisher:      opts.Publisher,
		dlqStore:       opts.DLQStore,
		cleanupDelay:   cleanupDelay,
		deliveryTTL:    deliveryTTL,
		now:            nowFn,
		afterFunc:      afterFn,
		issueLocks:     make(map[string]*issueLockState),
		seenDeliveries: make(map[string]time.Time),
		RunEvents:      opts.RunEvents,
		done:           make(chan struct{}),
	}

	if d.RunEvents != nil {
		d.Runsub = d.RunEvents.Subscribe()
		go d.watchRunEvents()
	}
	return d
}

// Close unsubscribes dispatcher from Run events.
func (d *WebhookDispatcher) Close() {
	if d == nil {
		return
	}

	d.closeOnce.Do(func() {
		close(d.done)
		if d.RunEvents != nil && d.Runsub != nil {
			d.RunEvents.Unsubscribe(d.Runsub)
		}
	})
}

// Dispatch routes one webhook payload through dedupe + serialization flow.
func (d *WebhookDispatcher) Dispatch(ctx context.Context, req WebhookDispatchRequest) (WebhookDispatchResult, error) {
	return d.dispatch(ctx, req, false)
}

// ReplayByDeliveryID re-dispatches one failed delivery from DLQ. Replayed delivery is idempotent.
func (d *WebhookDispatcher) ReplayByDeliveryID(ctx context.Context, deliveryID string) (bool, error) {
	if d == nil || d.dlqStore == nil {
		return false, ErrDLQEntryNotFound
	}

	entry, err := d.dlqStore.GetByDeliveryID(ctx, strings.TrimSpace(deliveryID))
	if err != nil {
		return false, err
	}
	if entry.Replayed {
		return false, nil
	}

	_, err = d.dispatch(ctx, WebhookDispatchRequest{
		ProjectID:  entry.ProjectID,
		EventType:  entry.EventType,
		Action:     entry.Action,
		DeliveryID: entry.DeliveryID,
		TraceID:    entry.TraceID,
		Payload:    append([]byte(nil), entry.Payload...),
		ReceivedAt: d.now(),
	}, true)
	if err != nil {
		return false, err
	}
	if err := d.dlqStore.MarkReplayed(ctx, entry.DeliveryID); err != nil {
		return false, err
	}
	return true, nil
}

func (d *WebhookDispatcher) dispatch(ctx context.Context, req WebhookDispatchRequest, ignoreDeliveryDedupe bool) (WebhookDispatchResult, error) {
	if d == nil {
		return WebhookDispatchResult{}, nil
	}

	if req.ReceivedAt.IsZero() {
		req.ReceivedAt = d.now()
	}
	req.EventType = strings.TrimSpace(req.EventType)
	req.Action = strings.TrimSpace(req.Action)
	req.DeliveryID = strings.TrimSpace(req.DeliveryID)
	req.TraceID = observability.TraceIDFromWebhook(strings.TrimSpace(req.TraceID), req.DeliveryID)
	ctx = observability.WithTraceID(ctx, req.TraceID)

	if !ignoreDeliveryDedupe && req.DeliveryID != "" && d.markDuplicateDelivery(req.DeliveryID) {
		return WebhookDispatchResult{Duplicate: true}, nil
	}

	issue, err := parseWebhookIssueContext(req.Payload)
	if err != nil {
		return WebhookDispatchResult{}, err
	}
	issueKey := buildIssueKey(issue.owner, issue.repo, issue.number)

	d.publishReceivedEvent(req, issue, issueKey)

	result := WebhookDispatchResult{IssueKey: issueKey}
	if issueKey == "" {
		err := d.handler.HandleWebhook(ctx, req)
		if err != nil {
			d.pushFailedToDLQ(ctx, req, issue.number, err)
		}
		return result, err
	}

	entry := d.acquireIssueLock(issueKey)
	defer d.releaseIssueLock(issueKey, entry)

	if err := d.handler.HandleWebhook(ctx, req); err != nil {
		d.pushFailedToDLQ(ctx, req, issue.number, err)
		return result, err
	}

	if shouldScheduleCleanup(req, issue) {
		d.scheduleIssueCleanup(issueKey)
	}

	return result, nil
}

func (d *WebhookDispatcher) watchRunEvents() {
	for {
		select {
		case <-d.done:
			return
		case evt, ok := <-d.Runsub:
			if !ok {
				return
			}
			if evt.Type != core.EventRunDone {
				continue
			}

			issueKey, ok := issueKeyFromRunDone(evt)
			if !ok {
				continue
			}
			d.scheduleIssueCleanup(issueKey)
		}
	}
}

func (d *WebhookDispatcher) markDuplicateDelivery(deliveryID string) bool {
	now := d.now()
	expireBefore := now.Add(-d.deliveryTTL)

	d.mu.Lock()
	defer d.mu.Unlock()

	for id, ts := range d.seenDeliveries {
		if ts.Before(expireBefore) {
			delete(d.seenDeliveries, id)
		}
	}

	if _, exists := d.seenDeliveries[deliveryID]; exists {
		return true
	}

	d.seenDeliveries[deliveryID] = now
	return false
}

func (d *WebhookDispatcher) acquireIssueLock(issueKey string) *issueLockState {
	now := d.now()

	d.mu.Lock()
	entry, ok := d.issueLocks[issueKey]
	if !ok {
		entry = &issueLockState{}
		d.issueLocks[issueKey] = entry
	}
	entry.inFlight++
	entry.lastTouched = now
	d.mu.Unlock()

	entry.lock.Lock()
	return entry
}

func (d *WebhookDispatcher) releaseIssueLock(issueKey string, entry *issueLockState) {
	if entry == nil {
		return
	}

	entry.lock.Unlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	current, ok := d.issueLocks[issueKey]
	if !ok || current != entry {
		return
	}
	if current.inFlight > 0 {
		current.inFlight--
	}
	current.lastTouched = d.now()
}

func (d *WebhookDispatcher) scheduleIssueCleanup(issueKey string) {
	scheduledAt := d.now()
	d.afterFunc(d.cleanupDelay, func() {
		shouldReschedule := false

		d.mu.Lock()
		entry, ok := d.issueLocks[issueKey]
		switch {
		case !ok:
		case entry.inFlight > 0:
			shouldReschedule = true
		case entry.lastTouched.After(scheduledAt):
			shouldReschedule = true
		default:
			delete(d.issueLocks, issueKey)
		}
		d.mu.Unlock()

		if shouldReschedule {
			d.scheduleIssueCleanup(issueKey)
		}
	})
}

func (d *WebhookDispatcher) publishReceivedEvent(req WebhookDispatchRequest, issue webhookIssueContext, issueKey string) {
	if d.publisher == nil {
		return
	}

	data := map[string]string{
		"event_type": req.EventType,
		"action":     req.Action,
	}
	if req.DeliveryID != "" {
		data["delivery_id"] = req.DeliveryID
	}
	if req.TraceID != "" {
		data["trace_id"] = req.TraceID
	}
	if issue.owner != "" {
		data["github_owner"] = issue.owner
	}
	if issue.repo != "" {
		data["github_repo"] = issue.repo
	}
	if issue.number > 0 {
		data["issue_number"] = strconv.Itoa(issue.number)
	}
	if issueKey != "" {
		data["issue_key"] = issueKey
	}

	d.publisher.Publish(core.Event{
		Type:      core.EventGitHubWebhookReceived,
		ProjectID: strings.TrimSpace(req.ProjectID),
		Data:      data,
		Timestamp: d.now(),
	})
}

func (d *WebhookDispatcher) pushFailedToDLQ(
	ctx context.Context,
	req WebhookDispatchRequest,
	issueNumber int,
	dispatchErr error,
) {
	if d == nil || d.dlqStore == nil || strings.TrimSpace(req.DeliveryID) == "" {
		return
	}
	_ = d.dlqStore.Push(ctx, DLQEntry{
		DeliveryID:  req.DeliveryID,
		ProjectID:   strings.TrimSpace(req.ProjectID),
		EventType:   strings.TrimSpace(req.EventType),
		Action:      strings.TrimSpace(req.Action),
		IssueNumber: issueNumber,
		TraceID:     strings.TrimSpace(req.TraceID),
		Payload:     append([]byte(nil), req.Payload...),
		FailedAt:    d.now(),
		LastError:   dispatchErr.Error(),
	})
}

type webhookIssueEnvelope struct {
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Issue struct {
		Number int    `json:"number"`
		State  string `json:"state"`
	} `json:"issue"`
}

type webhookIssueContext struct {
	owner  string
	repo   string
	number int
	state  string
}

func parseWebhookIssueContext(payload []byte) (webhookIssueContext, error) {
	if len(payload) == 0 {
		return webhookIssueContext{}, nil
	}

	var envelope webhookIssueEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return webhookIssueContext{}, err
	}

	return webhookIssueContext{
		owner:  normalizeRepoPart(envelope.Repository.Owner.Login),
		repo:   normalizeRepoPart(envelope.Repository.Name),
		number: envelope.Issue.Number,
		state:  strings.ToLower(strings.TrimSpace(envelope.Issue.State)),
	}, nil
}

func shouldScheduleCleanup(req WebhookDispatchRequest, issue webhookIssueContext) bool {
	if !strings.EqualFold(strings.TrimSpace(req.EventType), "issues") {
		return false
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "closed" {
		return true
	}
	return action == "" && issue.state == "closed"
}

func issueKeyFromRunDone(evt core.Event) (string, bool) {
	if evt.Data == nil {
		return "", false
	}

	owner := pickFirstNonEmpty(evt.Data["github_owner"], evt.Data["owner"])
	repo := pickFirstNonEmpty(evt.Data["github_repo"], evt.Data["repo"])
	if owner == "" || repo == "" {
		repository := strings.TrimSpace(evt.Data["repository"])
		if repository != "" {
			parts := strings.Split(repository, "/")
			if len(parts) == 2 {
				if owner == "" {
					owner = parts[0]
				}
				if repo == "" {
					repo = parts[1]
				}
			}
		}
	}

	issueNumberRaw := pickFirstNonEmpty(evt.Data["issue_number"], evt.Data["issue"])
	issueNumber, err := strconv.Atoi(strings.TrimSpace(issueNumberRaw))
	if err != nil {
		return "", false
	}

	issueKey := buildIssueKey(owner, repo, issueNumber)
	if issueKey == "" {
		return "", false
	}
	return issueKey, true
}

func pickFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func buildIssueKey(owner string, repo string, issueNumber int) string {
	normalizedOwner := normalizeRepoPart(owner)
	normalizedRepo := normalizeRepoPart(repo)
	if normalizedOwner == "" || normalizedRepo == "" || issueNumber <= 0 {
		return ""
	}
	return normalizedOwner + "/" + normalizedRepo + "#" + strconv.Itoa(issueNumber)
}

func normalizeRepoPart(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
