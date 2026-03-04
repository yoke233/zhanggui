package github

import (
	"context"
	"net"
	"testing"
)

func TestResilientClient_NetworkError_DegradesToNoop(t *testing.T) {
	base := &fakeRunIssueSyncClientWithError{
		updateErr: &net.OpError{Op: "dial", Err: &net.DNSError{IsTimeout: true}},
	}
	client := NewResilientClient(base)

	err := client.UpdateIssueLabels(context.Background(), 42, []string{"status: run_active:implement"})
	if err != nil {
		t.Fatalf("expected network error degraded to no-op, got %v", err)
	}
	if !client.IsDegraded() {
		t.Fatal("expected client to enter degraded mode after network error")
	}
}

type fakeRunIssueSyncClientWithError struct {
	updateErr  error
	commentErr error
}

func (f *fakeRunIssueSyncClientWithError) UpdateIssueLabels(context.Context, int, []string) error {
	return f.updateErr
}

func (f *fakeRunIssueSyncClientWithError) AddIssueComment(context.Context, int, string) error {
	return f.commentErr
}
