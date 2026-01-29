package execution

import "github.com/yoke233/zhanggui/internal/scheduler"

type RunStats struct {
	SchemaVersion int            `json:"schema_version"`
	Workflow      string         `json:"workflow"`
	MPUs          int            `json:"mpus"`
	MaxInFlight   int            `json:"max_in_flight"`
	Caps          scheduler.Caps `json:"caps"`
}
