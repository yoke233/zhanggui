package core

import (
	"fmt"
	"time"
)

// HumanActionType represents supported human-in-the-loop operations.
type HumanActionType string

const (
	ActionApprove     HumanActionType = "approve"
	ActionReject      HumanActionType = "reject"
	ActionModify      HumanActionType = "modify"
	ActionSkip        HumanActionType = "skip"
	ActionRerun       HumanActionType = "rerun"
	ActionChangeAgent HumanActionType = "change_agent"
	ActionAbort       HumanActionType = "abort"
	ActionPause       HumanActionType = "pause"
	ActionResume      HumanActionType = "resume"
)

var validHumanActionTypes = map[HumanActionType]struct{}{
	ActionApprove:     {},
	ActionReject:      {},
	ActionModify:      {},
	ActionSkip:        {},
	ActionRerun:       {},
	ActionChangeAgent: {},
	ActionAbort:       {},
	ActionPause:       {},
	ActionResume:      {},
}

// Validate checks whether the action type is one of the supported values.
func (t HumanActionType) Validate() error {
	if _, ok := validHumanActionTypes[t]; !ok {
		return fmt.Errorf("invalid human action type %q", t)
	}
	return nil
}

// PipelineAction is the normalized action payload accepted by engine/scheduler.
type PipelineAction struct {
	PipelineID string          `json:"pipeline_id"`
	Type       HumanActionType `json:"type"`
	Stage      StageID         `json:"stage"`
	Message    string          `json:"message,omitempty"`
	Agent      string          `json:"agent,omitempty"`
	CreatedAt  time.Time       `json:"created_at,omitempty"`
}

// Validate checks whether a pipeline action contains minimal required fields.
func (a PipelineAction) Validate() error {
	if a.PipelineID == "" {
		return fmt.Errorf("pipeline_id is required")
	}
	return a.Type.Validate()
}
