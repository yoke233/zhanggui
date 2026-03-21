package skills

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
)

const (
	GstackArtifactNamespace = "gstack"

	GstackArtifactDesignDoc    = "design_doc"
	GstackArtifactCEOReview    = "ceo_review"
	GstackArtifactEngReview    = "eng_review"
	GstackArtifactReviewReport = "review_report"
	GstackArtifactDocPlan      = "doc_update_plan"
)

// GstackArtifactSpec describes the normalized output contract for a gstack-* skill.
type GstackArtifactSpec struct {
	Type         string
	SkillName    string
	RelativePath string
	Title        string
	Summary      string
}

// BuildGstackArtifactRelPath returns the normalized artifact path used by
// gstack-* skills when they write markdown outputs into the workspace.
func BuildGstackArtifactRelPath(artifactType string, ts time.Time, topicSlug string) (string, error) {
	dir, err := gstackArtifactDir(artifactType)
	if err != nil {
		return "", err
	}
	slug, err := normalizeGstackTopicSlug(topicSlug)
	if err != nil {
		return "", err
	}
	date := ts.UTC().Format("2006-01-02")
	return filepath.ToSlash(filepath.Join(".ai-workflow", "artifacts", "gstack", dir, date+"-"+slug+".md")), nil
}

// BuildGstackArtifactMetadata attaches normalized artifact semantics onto an
// existing Run.ResultMetadata map.
func BuildGstackArtifactMetadata(base map[string]any, spec GstackArtifactSpec) map[string]any {
	out := make(map[string]any, len(base)+6)
	for k, v := range base {
		out[k] = v
	}
	out[core.ResultMetaArtifactNamespace] = GstackArtifactNamespace
	out[core.ResultMetaArtifactType] = spec.Type
	out[core.ResultMetaArtifactFormat] = "markdown"
	out[core.ResultMetaArtifactRelPath] = spec.RelativePath
	out[core.ResultMetaProducerSkill] = spec.SkillName
	out[core.ResultMetaProducerKind] = "skill"
	if strings.TrimSpace(spec.Title) != "" {
		out[core.ResultMetaArtifactTitle] = strings.TrimSpace(spec.Title)
	}
	if strings.TrimSpace(spec.Summary) != "" {
		out[core.ResultMetaSummary] = strings.TrimSpace(spec.Summary)
	}
	return out
}

func gstackArtifactDir(artifactType string) (string, error) {
	switch strings.TrimSpace(artifactType) {
	case GstackArtifactDesignDoc:
		return "office-hours", nil
	case GstackArtifactCEOReview:
		return "ceo-review", nil
	case GstackArtifactEngReview:
		return "eng-review", nil
	case GstackArtifactReviewReport:
		return "review", nil
	case GstackArtifactDocPlan:
		return "document-release", nil
	default:
		return "", fmt.Errorf("unsupported gstack artifact type %q", artifactType)
	}
}

func normalizeGstackTopicSlug(topicSlug string) (string, error) {
	slug := strings.TrimSpace(topicSlug)
	if slug == "" {
		return "", fmt.Errorf("topic slug is empty")
	}
	if slug == "." || slug == ".." {
		return "", fmt.Errorf("topic slug %q is invalid", topicSlug)
	}
	if strings.Contains(slug, "/") || strings.Contains(slug, "\\") {
		return "", fmt.Errorf("topic slug %q must not contain path separators", topicSlug)
	}
	if cleaned := filepath.Clean(slug); cleaned != slug {
		return "", fmt.Errorf("topic slug %q is invalid", topicSlug)
	}
	return slug, nil
}
