package github

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
)

// ResilientClient wraps issue sync writes and degrades network errors into no-op.
type ResilientClient struct {
	base RunIssueSyncClient

	mu       sync.RWMutex
	degraded bool
}

func NewResilientClient(base RunIssueSyncClient) *ResilientClient {
	return &ResilientClient{base: base}
}

func (c *ResilientClient) UpdateIssueLabels(ctx context.Context, issueNumber int, labels []string) error {
	if c == nil || c.base == nil {
		return nil
	}
	err := c.base.UpdateIssueLabels(ctx, issueNumber, labels)
	return c.handleError(err)
}

func (c *ResilientClient) AddIssueComment(ctx context.Context, issueNumber int, body string) error {
	if c == nil || c.base == nil {
		return nil
	}
	err := c.base.AddIssueComment(ctx, issueNumber, body)
	return c.handleError(err)
}

func (c *ResilientClient) IsDegraded() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.degraded
}

func (c *ResilientClient) MarkRecovered() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.degraded = false
	c.mu.Unlock()
}

func (c *ResilientClient) handleError(err error) error {
	if err == nil {
		return nil
	}
	if isNetworkError(err) {
		c.mu.Lock()
		c.degraded = true
		c.mu.Unlock()
		return nil
	}
	return err
}

func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, token := range []string{
		"timeout",
		"timed out",
		"dial tcp",
		"connection reset",
		"connection refused",
		"temporarily unavailable",
		"no such host",
		"network is unreachable",
	} {
		if strings.Contains(msg, token) {
			return true
		}
	}
	return false
}
