package core

import "strings"

// Legacy run result metadata keys used to express normalized artifact semantics.
//
// The current system is migrating to unified Deliverable objects, but Run keeps
// these metadata keys for compatibility with existing artifact HTTP routes and
// gate/signal consumers.
const (
	ResultMetaArtifactNamespace = "artifact_namespace"
	ResultMetaArtifactType      = "artifact_type"
	ResultMetaArtifactFormat    = "artifact_format"
	ResultMetaArtifactRelPath   = "artifact_relpath"
	ResultMetaArtifactTitle     = "artifact_title"
	ResultMetaProducerSkill     = "producer_skill"
	ResultMetaProducerKind      = "producer_kind"
	ResultMetaSummary           = "summary"
)

const (
	DeliverablePayloadKeyMarkdown = "markdown"
	DeliverablePayloadKeyMetadata = "metadata"
	DeliverablePayloadKeyArtifact = "artifact"
)

// HasArtifactResultMetadata reports whether Run.ResultMetadata contains any
// normalized artifact contract fields with non-empty values.
func HasArtifactResultMetadata(metadata map[string]any) bool {
	return NormalizeArtifactMetadata(metadata) != nil
}

// NormalizeArtifactMetadata maps legacy run result metadata into the stable
// artifact response shape used by HTTP and other readers.
func NormalizeArtifactMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	artifact := map[string]any{}
	copyNormalizedString(metadata, artifact, ResultMetaArtifactNamespace, "namespace")
	copyNormalizedString(metadata, artifact, ResultMetaArtifactType, "type")
	copyNormalizedString(metadata, artifact, ResultMetaArtifactFormat, "format")
	copyNormalizedString(metadata, artifact, ResultMetaArtifactRelPath, "relpath")
	copyNormalizedString(metadata, artifact, ResultMetaArtifactTitle, "title")
	copyNormalizedString(metadata, artifact, ResultMetaProducerSkill, "producer_skill")
	copyNormalizedString(metadata, artifact, ResultMetaProducerKind, "producer_kind")
	copyNormalizedString(metadata, artifact, ResultMetaSummary, "summary")
	if len(artifact) == 0 {
		return nil
	}
	return artifact
}

// NormalizeDeliverableArtifact extracts the normalized artifact block from a
// unified deliverable payload.
func NormalizeDeliverableArtifact(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	raw, ok := payload[DeliverablePayloadKeyArtifact].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	artifact := map[string]any{}
	for _, key := range []string{
		"namespace",
		"type",
		"format",
		"relpath",
		"title",
		"producer_skill",
		"producer_kind",
		"summary",
	} {
		copyNormalizedString(raw, artifact, key, key)
	}
	if len(artifact) == 0 {
		return nil
	}
	return artifact
}

func DeliverablePayloadMarkdown(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	value, _ := payload[DeliverablePayloadKeyMarkdown].(string)
	return strings.TrimSpace(value)
}

func DeliverablePayloadMetadata(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	raw, ok := payload[DeliverablePayloadKeyMetadata].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(raw))
	for key, value := range raw {
		cloned[key] = value
	}
	return cloned
}

// RunResultToDeliverable projects legacy inline run results into the unified
// deliverable shape so new code can consume one result abstraction.
func RunResultToDeliverable(run *Run) *Deliverable {
	if run == nil || !run.HasResult() {
		return nil
	}

	payload := map[string]any{}
	if strings.TrimSpace(run.ResultMarkdown) != "" {
		payload[DeliverablePayloadKeyMarkdown] = run.ResultMarkdown
	}
	if len(run.ResultMetadata) > 0 {
		payload[DeliverablePayloadKeyMetadata] = cloneAnyMap(run.ResultMetadata)
	}
	if artifact := NormalizeArtifactMetadata(run.ResultMetadata); artifact != nil {
		payload[DeliverablePayloadKeyArtifact] = artifact
	}
	if len(run.ResultAssets) > 0 {
		payload["assets"] = append([]Asset(nil), run.ResultAssets...)
	}

	workItemID := run.WorkItemID
	return &Deliverable{
		WorkItemID:   &workItemID,
		Kind:         inferDeliverableKind(run.ResultMetadata),
		Title:        artifactString(run.ResultMetadata, ResultMetaArtifactTitle),
		Summary:      inferDeliverableSummary(run.ResultMetadata, run.ResultMarkdown),
		Payload:      payload,
		ProducerType: DeliverableProducerRun,
		ProducerID:   run.ID,
		Status:       DeliverableFinal,
		CreatedAt:    run.CreatedAt,
	}
}

func inferDeliverableKind(metadata map[string]any) DeliverableKind {
	switch strings.ToLower(strings.TrimSpace(artifactString(metadata, ResultMetaArtifactType))) {
	case "code_change", "branch", "diff", "patch":
		return DeliverableCodeChange
	case "pull_request", "pr":
		return DeliverablePullRequest
	case "decision":
		return DeliverableDecision
	case "meeting_summary":
		return DeliverableMeetingSummary
	case "aggregate_report", "report":
		return DeliverableAggregateReport
	default:
		return DeliverableDocument
	}
}

func inferDeliverableSummary(metadata map[string]any, markdown string) string {
	if summary := artifactString(metadata, ResultMetaSummary); summary != "" {
		return summary
	}
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line != "" {
			return line
		}
	}
	return ""
}

func artifactString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func copyNormalizedString(src, dst map[string]any, srcKey, dstKey string) {
	value, ok := src[srcKey].(string)
	if !ok {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	dst[dstKey] = value
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
