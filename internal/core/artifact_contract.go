package core

import "strings"

// Run result metadata keys used to express normalized artifact semantics.
//
// The current system stores deliverables inline on Run via ResultMarkdown +
// ResultMetadata. These keys establish stable conventions without requiring a
// separate artifact table up front.
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

// HasArtifactResultMetadata reports whether Run.ResultMetadata contains any
// normalized artifact contract fields with non-empty values.
func HasArtifactResultMetadata(metadata map[string]any) bool {
	for _, key := range []string{
		ResultMetaArtifactNamespace,
		ResultMetaArtifactType,
		ResultMetaArtifactFormat,
		ResultMetaArtifactRelPath,
		ResultMetaArtifactTitle,
		ResultMetaProducerSkill,
		ResultMetaProducerKind,
		ResultMetaSummary,
	} {
		value, ok := metadata[key].(string)
		if ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}
