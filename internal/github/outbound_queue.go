package github

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	ghapi "github.com/google/go-github/v68/github"
)

const (
	defaultOutboundRateLimitRPS     = 1.0
	defaultOutboundRateLimitBurst   = 5
	defaultRateLimitRetryMax        = 3
	defaultServerRetryMax           = 3
	defaultServerRetryBaseBackoff   = 250 * time.Millisecond
	defaultRateLimitRetryAfterSleep = 1 * time.Second
)

// OutboundQueueOptions controls queue rate limiting and retry behavior.
type OutboundQueueOptions struct {
	RateLimitRPS       float64
	RateLimitBurst     int
	MaxRateLimitRetries int
	MaxServerRetries   int
	ServerBackoffBase  time.Duration
	Now                func() time.Time
	Sleep              func(time.Duration)
}

// OutboundWriteRequest is one GitHub write operation routed by outbound queue.
type OutboundWriteRequest struct {
	IssueNumber    int
	Operation      string
	IdempotencyKey string
	Execute        func(context.Context) (*ghapi.Response, error)
}

// OutboundQueue serializes per-issue writes, enforces token-bucket limit, and applies retry policy.
type OutboundQueue struct {
	rateLimitRPS        float64
	rateLimitBurst      float64
	maxRateLimitRetries int
	maxServerRetries    int
	serverBackoffBase   time.Duration
	now                 func() time.Time
	sleep               func(time.Duration)

	mu                  sync.Mutex
	issueLocks          map[int]*sync.Mutex
	idempotencyDone     map[string]struct{}
	tokens              float64
	lastRefillTimestamp time.Time
}

// NewOutboundQueue builds a write queue with spec defaults: 1 req/s and burst 5.
func NewOutboundQueue(opts OutboundQueueOptions) *OutboundQueue {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	sleepFn := opts.Sleep
	if sleepFn == nil {
		sleepFn = time.Sleep
	}

	rps := opts.RateLimitRPS
	if rps <= 0 {
		rps = defaultOutboundRateLimitRPS
	}
	burst := opts.RateLimitBurst
	if burst <= 0 {
		burst = defaultOutboundRateLimitBurst
	}
	maxRateRetries := opts.MaxRateLimitRetries
	if maxRateRetries < 0 {
		maxRateRetries = 0
	}
	if maxRateRetries == 0 {
		maxRateRetries = defaultRateLimitRetryMax
	}
	maxServerRetries := opts.MaxServerRetries
	if maxServerRetries < 0 {
		maxServerRetries = 0
	}
	if maxServerRetries == 0 {
		maxServerRetries = defaultServerRetryMax
	}
	serverBackoffBase := opts.ServerBackoffBase
	if serverBackoffBase <= 0 {
		serverBackoffBase = defaultServerRetryBaseBackoff
	}

	now := nowFn()
	return &OutboundQueue{
		rateLimitRPS:        rps,
		rateLimitBurst:      float64(burst),
		maxRateLimitRetries: maxRateRetries,
		maxServerRetries:    maxServerRetries,
		serverBackoffBase:   serverBackoffBase,
		now:                 nowFn,
		sleep:               sleepFn,
		issueLocks:          make(map[int]*sync.Mutex),
		idempotencyDone:     make(map[string]struct{}),
		tokens:              float64(burst),
		lastRefillTimestamp: now,
	}
}

// Do executes one outbound write operation.
func (q *OutboundQueue) Do(ctx context.Context, req OutboundWriteRequest) error {
	if q == nil {
		return errors.New("outbound queue is nil")
	}
	if req.Execute == nil {
		return errors.New("outbound request execute function is required")
	}
	operation := strings.TrimSpace(req.Operation)
	if operation == "" {
		operation = "github outbound write"
	}

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey != "" && q.isIdempotencyCompleted(idempotencyKey) {
		return nil
	}

	lock := q.issueLock(req.IssueNumber)
	lock.Lock()
	defer lock.Unlock()

	if idempotencyKey != "" && q.isIdempotencyCompleted(idempotencyKey) {
		return nil
	}

	rateLimitRetries := 0
	serverRetries := 0
	for {
		if err := q.takeToken(ctx); err != nil {
			return fmt.Errorf("%s: acquire rate-limit token: %w", operation, err)
		}

		resp, err := req.Execute(ctx)
		statusCode := httpStatusCode(resp)
		if statusCode >= 200 && statusCode < 400 && err == nil {
			if idempotencyKey != "" {
				q.markIdempotencyCompleted(idempotencyKey)
			}
			return nil
		}
		if statusCode == 0 && err == nil {
			if idempotencyKey != "" {
				q.markIdempotencyCompleted(idempotencyKey)
			}
			return nil
		}

		if retryAfter, shouldRetry := rateLimitBackoff(resp, err); shouldRetry {
			if rateLimitRetries >= q.maxRateLimitRetries {
				return fmt.Errorf("%s: exceeded rate-limit retries (%d): %w", operation, q.maxRateLimitRetries, firstNonNil(err, unexpectedStatusError(statusCode)))
			}
			rateLimitRetries++
			if sleepErr := q.sleepWithContext(ctx, retryAfter); sleepErr != nil {
				return fmt.Errorf("%s: rate-limit backoff interrupted: %w", operation, sleepErr)
			}
			continue
		}

		if shouldRetryServer(statusCode) {
			if serverRetries >= q.maxServerRetries {
				return fmt.Errorf("%s: exceeded 5xx retries (%d): %w", operation, q.maxServerRetries, firstNonNil(err, unexpectedStatusError(statusCode)))
			}
			backoff := q.serverBackoffBase * time.Duration(1<<serverRetries)
			serverRetries++
			if sleepErr := q.sleepWithContext(ctx, backoff); sleepErr != nil {
				return fmt.Errorf("%s: server backoff interrupted: %w", operation, sleepErr)
			}
			continue
		}

		if err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
		if statusCode >= 400 {
			return fmt.Errorf("%s: unexpected github status %d", operation, statusCode)
		}
		return nil
	}
}

func (q *OutboundQueue) issueLock(issueNumber int) *sync.Mutex {
	issueKey := issueNumber
	if issueKey < 0 {
		issueKey = 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	lock, ok := q.issueLocks[issueKey]
	if !ok {
		lock = &sync.Mutex{}
		q.issueLocks[issueKey] = lock
	}
	return lock
}

func (q *OutboundQueue) takeToken(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	for {
		now := q.now()
		q.mu.Lock()
		q.refillTokensLocked(now)
		if q.tokens >= 1 {
			q.tokens--
			q.mu.Unlock()
			return nil
		}

		waitSeconds := (1 - q.tokens) / q.rateLimitRPS
		if waitSeconds <= 0 {
			waitSeconds = 1 / q.rateLimitRPS
		}
		waitDuration := time.Duration(waitSeconds * float64(time.Second))
		if waitDuration <= 0 {
			waitDuration = time.Millisecond
		}
		q.mu.Unlock()

		if err := q.sleepWithContext(ctx, waitDuration); err != nil {
			return err
		}
	}
}

func (q *OutboundQueue) refillTokensLocked(now time.Time) {
	if q.lastRefillTimestamp.IsZero() {
		q.lastRefillTimestamp = now
	}
	elapsed := now.Sub(q.lastRefillTimestamp).Seconds()
	if elapsed > 0 {
		q.tokens += elapsed * q.rateLimitRPS
		if q.tokens > q.rateLimitBurst {
			q.tokens = q.rateLimitBurst
		}
		q.lastRefillTimestamp = now
	}
	if q.tokens < 0 {
		q.tokens = 0
	}
}

func (q *OutboundQueue) sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	q.sleep(d)
	return ctx.Err()
}

func (q *OutboundQueue) isIdempotencyCompleted(key string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.idempotencyDone[key]
	return ok
}

func (q *OutboundQueue) markIdempotencyCompleted(key string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.idempotencyDone[key] = struct{}{}
}

func rateLimitBackoff(resp *ghapi.Response, err error) (time.Duration, bool) {
	statusCode := httpStatusCode(resp)
	if statusCode == 429 {
		return retryAfter(resp), true
	}
	if statusCode == 403 {
		if hasRetryAfter(resp) {
			return retryAfter(resp), true
		}
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "secondary rate limit") {
			return defaultRateLimitRetryAfterSleep, true
		}
	}
	return 0, false
}

func shouldRetryServer(statusCode int) bool {
	return statusCode >= 500 && statusCode <= 599
}

func retryAfter(resp *ghapi.Response) time.Duration {
	if resp == nil || resp.Response == nil {
		return defaultRateLimitRetryAfterSleep
	}
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if raw == "" {
		return defaultRateLimitRetryAfterSleep
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultRateLimitRetryAfterSleep
	}
	return time.Duration(seconds) * time.Second
}

func hasRetryAfter(resp *ghapi.Response) bool {
	if resp == nil || resp.Response == nil {
		return false
	}
	return strings.TrimSpace(resp.Header.Get("Retry-After")) != ""
}

func httpStatusCode(resp *ghapi.Response) int {
	if resp == nil || resp.Response == nil {
		return 0
	}
	return resp.StatusCode
}

func unexpectedStatusError(statusCode int) error {
	if statusCode <= 0 {
		return errors.New("unknown github response status")
	}
	return fmt.Errorf("unexpected github response status %d", statusCode)
}

func firstNonNil(primary error, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}
