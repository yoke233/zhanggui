package outbox

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestGetUintCache(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	value, err := svc.getUintCache(ctx, "lead:backend:cursor:event_id")
	if err != nil {
		t.Fatalf("getUintCache(missing) error = %v", err)
	}
	if value != 0 {
		t.Fatalf("getUintCache(missing) = %d, want 0", value)
	}

	cache.data["lead:backend:cursor:event_id"] = "42"
	value, err = svc.getUintCache(ctx, "lead:backend:cursor:event_id")
	if err != nil {
		t.Fatalf("getUintCache(valid) error = %v", err)
	}
	if value != 42 {
		t.Fatalf("getUintCache(valid) = %d, want 42", value)
	}

	cache.data["lead:backend:cursor:event_id"] = "not-a-number"
	if _, err := svc.getUintCache(ctx, "lead:backend:cursor:event_id"); err == nil {
		t.Fatalf("getUintCache(invalid) expected error")
	}
}

func TestNextRunIDIncrements(t *testing.T) {
	svc, cache := setupService(t)
	ctx := context.Background()

	runID1, err := svc.nextRunID(ctx, "local#88", "backend")
	if err != nil {
		t.Fatalf("nextRunID(first) error = %v", err)
	}
	runID2, err := svc.nextRunID(ctx, "local#88", "backend")
	if err != nil {
		t.Fatalf("nextRunID(second) error = %v", err)
	}

	if !strings.HasSuffix(runID1, "-0001") {
		t.Fatalf("runID1 = %q", runID1)
	}
	if !strings.HasSuffix(runID2, "-0002") {
		t.Fatalf("runID2 = %q", runID2)
	}

	seqValue := cache.data[leadRunSeqKey("backend", "local#88")]
	if seqValue != strconv.Itoa(2) {
		t.Fatalf("run seq cache = %q, want 2", seqValue)
	}
}

func TestLeadContextPackDirSanitizesIssueRef(t *testing.T) {
	dir := leadContextPackDir("owner/repo#12:ab\\cd", "2026-02-14-backend-0001")
	if !strings.Contains(dir, "owner_repo_12_ab_cd") {
		t.Fatalf("leadContextPackDir() = %q", dir)
	}
}

func TestFormatReadUpTo(t *testing.T) {
	if got := formatReadUpTo(0); got != "none" {
		t.Fatalf("formatReadUpTo(0) = %q, want none", got)
	}
	if got := formatReadUpTo(17); got != "e17" {
		t.Fatalf("formatReadUpTo(17) = %q, want e17", got)
	}
}

func TestCurrentStateLabel(t *testing.T) {
	labels := []string{"to:backend", "  state:doing  ", "state:review"}
	if got := currentStateLabel(labels); got != "state:doing" {
		t.Fatalf("currentStateLabel() = %q, want state:doing", got)
	}
	if got := currentStateLabel([]string{"to:backend"}); got != "" {
		t.Fatalf("currentStateLabel(no state) = %q, want empty", got)
	}
}

func TestNextRoleForReviewChanges(t *testing.T) {
	testCases := []struct {
		name   string
		labels []string
		want   string
	}{
		{name: "backend preferred", labels: []string{"to:backend", "to:frontend"}, want: "backend"},
		{name: "frontend only", labels: []string{"to:frontend"}, want: "frontend"},
		{name: "qa fallback", labels: []string{"to:qa"}, want: "qa"},
		{name: "default backend", labels: []string{"to:reviewer"}, want: "backend"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := nextRoleForReviewChanges(testCase.labels)
			if got != testCase.want {
				t.Fatalf("nextRoleForReviewChanges() = %q, want %q", got, testCase.want)
			}
		})
	}
}
