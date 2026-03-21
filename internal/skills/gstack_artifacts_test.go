package skills

import (
	"testing"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
)

func TestBuildGstackArtifactRelPath(t *testing.T) {
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)

	got, err := BuildGstackArtifactRelPath(GstackArtifactEngReview, ts, "login-flow")
	if err != nil {
		t.Fatalf("BuildGstackArtifactRelPath() error = %v", err)
	}

	want := ".ai-workflow/artifacts/gstack/eng-review/2026-03-21-login-flow.md"
	if got != want {
		t.Fatalf("BuildGstackArtifactRelPath() = %q, want %q", got, want)
	}
}

func TestBuildGstackArtifactRelPathRejectsUnknownType(t *testing.T) {
	_, err := BuildGstackArtifactRelPath("unknown", time.Now(), "login-flow")
	if err == nil {
		t.Fatal("expected error for unknown artifact type")
	}
}

func TestBuildGstackArtifactRelPathRejectsUnsafeSlug(t *testing.T) {
	ts := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	for _, slug := range []string{"../login-flow", "nested/topic", `nested\topic`, ".", ".."} {
		if _, err := BuildGstackArtifactRelPath(GstackArtifactEngReview, ts, slug); err == nil {
			t.Fatalf("expected error for unsafe slug %q", slug)
		}
	}
}

func TestBuildGstackArtifactMetadata(t *testing.T) {
	base := map[string]any{"existing": "value"}
	meta := BuildGstackArtifactMetadata(base, GstackArtifactSpec{
		Type:         GstackArtifactReviewReport,
		SkillName:    "gstack-review",
		RelativePath: ".ai-workflow/artifacts/gstack/review/2026-03-21-login-flow.md",
		Title:        "Login Flow Review",
		Summary:      "Found two correctness issues.",
	})

	if got := meta["existing"]; got != "value" {
		t.Fatalf("existing metadata lost, got %v", got)
	}
	if got := meta[core.ResultMetaArtifactNamespace]; got != GstackArtifactNamespace {
		t.Fatalf("artifact namespace = %v", got)
	}
	if got := meta[core.ResultMetaArtifactType]; got != GstackArtifactReviewReport {
		t.Fatalf("artifact type = %v", got)
	}
	if got := meta[core.ResultMetaArtifactFormat]; got != "markdown" {
		t.Fatalf("artifact format = %v", got)
	}
	if got := meta[core.ResultMetaProducerSkill]; got != "gstack-review" {
		t.Fatalf("producer skill = %v", got)
	}
	if got := meta[core.ResultMetaArtifactRelPath]; got != ".ai-workflow/artifacts/gstack/review/2026-03-21-login-flow.md" {
		t.Fatalf("artifact path = %v", got)
	}
	if got := meta[core.ResultMetaArtifactTitle]; got != "Login Flow Review" {
		t.Fatalf("artifact title = %v", got)
	}
	if got := meta[core.ResultMetaSummary]; got != "Found two correctness issues." {
		t.Fatalf("artifact summary = %v", got)
	}
}
