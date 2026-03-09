package core

// Memory provides layered context for prompt building.
// Implementations return pre-formatted text ready for prompt injection.
type Memory interface {
	// RecallCold returns rarely-changing background context.
	RecallCold(issueID string) (string, error)

	// RecallWarm returns parent and sibling issue summaries.
	RecallWarm(issueID string) (string, error)

	// RecallHot returns recent activity for an issue/run pair.
	RecallHot(issueID string, runID string) (string, error)
}
