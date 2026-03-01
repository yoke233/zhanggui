package github

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ghapi "github.com/google/go-github/v68/github"
)

func TestOutboundQueue_RespectsTokenBucketRate(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	queue := NewOutboundQueue(OutboundQueueOptions{
		RateLimitRPS: 1,
		RateLimitBurst: 1,
		Now: clock.Now,
		Sleep: clock.Sleep,
	})

	firstCalls := int32(0)
	if err := queue.Do(context.Background(), OutboundWriteRequest{
		IssueNumber: 101,
		Operation: "first",
		Execute: func(context.Context) (*ghapi.Response, error) {
			atomic.AddInt32(&firstCalls, 1)
			return nil, nil
		},
	}); err != nil {
		t.Fatalf("first queue do failed: %v", err)
	}

	secondCalls := int32(0)
	if err := queue.Do(context.Background(), OutboundWriteRequest{
		IssueNumber: 101,
		Operation: "second",
		Execute: func(context.Context) (*ghapi.Response, error) {
			atomic.AddInt32(&secondCalls, 1)
			return nil, nil
		},
	}); err != nil {
		t.Fatalf("second queue do failed: %v", err)
	}

	if atomic.LoadInt32(&firstCalls) != 1 || atomic.LoadInt32(&secondCalls) != 1 {
		t.Fatalf("expected both requests to execute once, got first=%d second=%d", firstCalls, secondCalls)
	}
	if got := clock.TotalSlept(); got < time.Second {
		t.Fatalf("expected token-bucket sleep >= 1s, got %v", got)
	}
}

func TestOutboundQueue_RetryWithBackoffOn429(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	queue := NewOutboundQueue(OutboundQueueOptions{
		RateLimitRPS: 100,
		RateLimitBurst: 100,
		MaxRateLimitRetries: 3,
		Now: clock.Now,
		Sleep: clock.Sleep,
	})

	attempts := int32(0)
	err := queue.Do(context.Background(), OutboundWriteRequest{
		IssueNumber: 101,
		Operation: "429-retry",
		Execute: func(context.Context) (*ghapi.Response, error) {
			current := atomic.AddInt32(&attempts, 1)
			if current == 1 {
				return githubHTTPResponse(429, "2"), errors.New("rate limited")
			}
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("queue do failed: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if got := clock.TotalSlept(); got != 2*time.Second {
		t.Fatalf("expected retry sleep 2s, got %v", got)
	}
}

func TestOutboundQueue_RetryWithBackoffOn403SecondaryLimit(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	queue := NewOutboundQueue(OutboundQueueOptions{
		RateLimitRPS: 100,
		RateLimitBurst: 100,
		MaxRateLimitRetries: 3,
		Now: clock.Now,
		Sleep: clock.Sleep,
	})

	attempts := int32(0)
	err := queue.Do(context.Background(), OutboundWriteRequest{
		IssueNumber: 102,
		Operation: "403-secondary",
		Execute: func(context.Context) (*ghapi.Response, error) {
			current := atomic.AddInt32(&attempts, 1)
			if current == 1 {
				return githubHTTPResponse(403, "3"), errors.New("secondary rate limit")
			}
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("queue do failed: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if got := clock.TotalSlept(); got != 3*time.Second {
		t.Fatalf("expected retry sleep 3s, got %v", got)
	}
}

func TestOutboundQueue_RetryAtMost3TimesOnRateLimit(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	queue := NewOutboundQueue(OutboundQueueOptions{
		RateLimitRPS: 100,
		RateLimitBurst: 100,
		MaxRateLimitRetries: 3,
		Now: clock.Now,
		Sleep: clock.Sleep,
	})

	attempts := int32(0)
	err := queue.Do(context.Background(), OutboundWriteRequest{
		IssueNumber: 103,
		Operation: "retry-limit",
		Execute: func(context.Context) (*ghapi.Response, error) {
			atomic.AddInt32(&attempts, 1)
			return githubHTTPResponse(429, "1"), errors.New("rate limited")
		},
	})
	if err == nil {
		t.Fatal("expected retry budget error")
	}
	if atomic.LoadInt32(&attempts) != 4 {
		t.Fatalf("expected initial + 3 retries = 4 attempts, got %d", attempts)
	}
}

func TestOutboundQueue_PreservesPerIssueOrdering(t *testing.T) {
	queue := NewOutboundQueue(OutboundQueueOptions{
		RateLimitRPS: 100,
		RateLimitBurst: 100,
	})

	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	secondExecuted := make(chan struct{})

	var mu sync.Mutex
	order := make([]string, 0, 2)

	doneFirst := make(chan error, 1)
	go func() {
		doneFirst <- queue.Do(context.Background(), OutboundWriteRequest{
			IssueNumber: 200,
			Operation: "first",
			Execute: func(context.Context) (*ghapi.Response, error) {
				mu.Lock()
				order = append(order, "first")
				mu.Unlock()
				close(firstStarted)
				<-firstRelease
				return nil, nil
			},
		})
	}()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first request did not start in time")
	}

	doneSecond := make(chan error, 1)
	go func() {
		doneSecond <- queue.Do(context.Background(), OutboundWriteRequest{
			IssueNumber: 200,
			Operation: "second",
			Execute: func(context.Context) (*ghapi.Response, error) {
				mu.Lock()
				order = append(order, "second")
				mu.Unlock()
				close(secondExecuted)
				return nil, nil
			},
		})
	}()

	select {
	case <-secondExecuted:
		t.Fatal("second operation executed before first released")
	case <-time.After(80 * time.Millisecond):
	}

	close(firstRelease)

	select {
	case err := <-doneFirst:
		if err != nil {
			t.Fatalf("first queue do failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first queue do timeout")
	}

	select {
	case err := <-doneSecond:
		if err != nil {
			t.Fatalf("second queue do failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second queue do timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("expected order [first second], got %v", order)
	}
}

func githubHTTPResponse(statusCode int, retryAfter string) *ghapi.Response {
	headers := http.Header{}
	if retryAfter != "" {
		headers.Set("Retry-After", retryAfter)
	}
	return &ghapi.Response{
		Response: &http.Response{
			StatusCode: statusCode,
			Header: headers,
		},
	}
}

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	sleeps []time.Duration
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Sleep(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if d <= 0 {
		return
	}
	f.sleeps = append(f.sleeps, d)
	f.now = f.now.Add(d)
}

func (f *fakeClock) TotalSlept() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	var total time.Duration
	for _, d := range f.sleeps {
		total += d
	}
	return total
}
