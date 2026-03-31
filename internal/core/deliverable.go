package core

import (
	"fmt"
	"strings"
	"time"
)

type DeliverableKind string

const (
	DeliverableDocument        DeliverableKind = "document"
	DeliverableCodeChange      DeliverableKind = "code_change"
	DeliverablePullRequest     DeliverableKind = "pull_request"
	DeliverableDecision        DeliverableKind = "decision"
	DeliverableMeetingSummary  DeliverableKind = "meeting_summary"
	DeliverableAggregateReport DeliverableKind = "aggregate_report"
)

func (k DeliverableKind) Valid() bool {
	switch k {
	case DeliverableDocument, DeliverableCodeChange, DeliverablePullRequest,
		DeliverableDecision, DeliverableMeetingSummary, DeliverableAggregateReport:
		return true
	default:
		return false
	}
}

type DeliverableProducerType string

const (
	DeliverableProducerRun      DeliverableProducerType = "run"
	DeliverableProducerThread   DeliverableProducerType = "thread"
	DeliverableProducerWorkItem DeliverableProducerType = "workitem"
)

func (t DeliverableProducerType) Valid() bool {
	switch t {
	case DeliverableProducerRun, DeliverableProducerThread, DeliverableProducerWorkItem:
		return true
	default:
		return false
	}
}

type DeliverableStatus string

const (
	DeliverableDraft   DeliverableStatus = "draft"
	DeliverableFinal   DeliverableStatus = "final"
	DeliverableAdopted DeliverableStatus = "adopted"
)

func (s DeliverableStatus) Valid() bool {
	switch s {
	case DeliverableDraft, DeliverableFinal, DeliverableAdopted:
		return true
	default:
		return false
	}
}

// Deliverable is the unified output object shared by runs, threads, and work items.
type Deliverable struct {
	ID           int64                   `json:"id"`
	WorkItemID   *int64                  `json:"work_item_id,omitempty"`
	ThreadID     *int64                  `json:"thread_id,omitempty"`
	Kind         DeliverableKind         `json:"kind"`
	Title        string                  `json:"title,omitempty"`
	Summary      string                  `json:"summary,omitempty"`
	Payload      map[string]any          `json:"payload,omitempty"`
	ProducerType DeliverableProducerType `json:"producer_type"`
	ProducerID   int64                   `json:"producer_id"`
	Status       DeliverableStatus       `json:"status"`
	CreatedAt    time.Time               `json:"created_at"`
}

func (d *Deliverable) Validate() error {
	if d == nil {
		return fmt.Errorf("deliverable is nil")
	}
	if !d.Kind.Valid() {
		return fmt.Errorf("invalid deliverable kind %q", d.Kind)
	}
	if !d.ProducerType.Valid() {
		return fmt.Errorf("invalid deliverable producer_type %q", d.ProducerType)
	}
	if d.ProducerID <= 0 {
		return fmt.Errorf("deliverable producer_id must be > 0")
	}
	if !d.Status.Valid() {
		return fmt.Errorf("invalid deliverable status %q", d.Status)
	}
	if d.WorkItemID != nil && *d.WorkItemID <= 0 {
		return fmt.Errorf("deliverable work_item_id must be > 0")
	}
	if d.ThreadID != nil && *d.ThreadID <= 0 {
		return fmt.Errorf("deliverable thread_id must be > 0")
	}
	return nil
}

func (d *Deliverable) HasContent() bool {
	if d == nil {
		return false
	}
	return strings.TrimSpace(d.Title) != "" ||
		strings.TrimSpace(d.Summary) != "" ||
		len(d.Payload) > 0
}
