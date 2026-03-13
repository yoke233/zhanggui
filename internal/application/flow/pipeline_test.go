package flow

import (
	"strings"
	"testing"
)

func TestRenderInputSnapshot_IncludesContextRefs(t *testing.T) {
	objective := "Review and update the implementation."
	contextRefs := []ContextRef{
		{
			Type:   CtxUpstreamArtifact,
			RefID:  101,
			Label:  "implement output",
			Inline: "Ran `go test ./...` and updated README.md.",
		},
		{
			Type:   CtxUpstreamArtifact,
			RefID:  102,
			Inline: "Opened PR #12.",
		},
	}

	got := renderInputSnapshot(objective, contextRefs)
	if !strings.Contains(got, "Review and update the implementation.") {
		t.Fatalf("expected objective in snapshot, got: %q", got)
	}
	if !strings.Contains(got, "# Context") {
		t.Fatalf("expected context header in snapshot, got: %q", got)
	}
	if !strings.Contains(got, "## implement output") {
		t.Fatalf("expected explicit context label, got: %q", got)
	}
	if !strings.Contains(got, "Ran `go test ./...` and updated README.md.") {
		t.Fatalf("expected upstream artifact inline content, got: %q", got)
	}
	if !strings.Contains(got, "## upstream_artifact:102") {
		t.Fatalf("expected fallback context label, got: %q", got)
	}
}

func TestRenderInputSnapshot_RespectsTotalBudget(t *testing.T) {
	longObjective := strings.Repeat("o", maxInputTotalChars+500)
	longRef := strings.Repeat("x", maxInputRefChars+500)
	contextRefs := []ContextRef{
		{
			Type:   CtxUpstreamArtifact,
			RefID:  201,
			Label:  "upstream",
			Inline: longRef,
		},
	}

	got := renderInputSnapshot(longObjective, contextRefs)
	if len(got) > maxInputTotalChars {
		t.Fatalf("snapshot length=%d exceeds budget=%d", len(got), maxInputTotalChars)
	}
	if strings.Contains(got, "# Context") {
		t.Fatalf("expected no context when objective already exhausted budget, got length=%d", len(got))
	}
	if !strings.Contains(got, "[truncated]") {
		t.Fatalf("expected truncated marker in snapshot, got: %q", got)
	}
}
