package core

import (
	"fmt"
	"time"
)

// HumanActionType represents supported human-in-the-loop operations.
type HumanActionType string

const (
	ActionApprove    HumanActionType = "approve"
	ActionReject     HumanActionType = "reject"
	ActionModify     HumanActionType = "modify"
	ActionSkip       HumanActionType = "skip"
	ActionRerun      HumanActionType = "rerun"
	ActionChangeRole HumanActionType = "change_role"
	ActionAbort      HumanActionType = "abort"
	ActionPause      HumanActionType = "pause"
	ActionResume     HumanActionType = "resume"
)

var validHumanActionTypes = map[HumanActionType]struct{}{
	ActionApprove:    {},
	ActionReject:     {},
	ActionModify:     {},
	ActionSkip:       {},
	ActionRerun:      {},
	ActionChangeRole: {},
	ActionAbort:      {},
	ActionPause:      {},
	ActionResume:     {},
}

// Validate checks whether the action type is one of the supported values.
func (t HumanActionType) Validate() error {
	if _, ok := validHumanActionTypes[t]; !ok {
		return fmt.Errorf("invalid human action type %q", t)
	}
	return nil
}

// RunAction is the normalized action payload accepted by engine/scheduler.
type RunAction struct {
	RunID     string          `json:"run_id"`
	Type      HumanActionType `json:"type"`
	Stage     StageID         `json:"stage"`
	Message   string          `json:"message,omitempty"`
	Role      string          `json:"role,omitempty"`
	CreatedAt time.Time       `json:"created_at,omitempty"`
}

// Validate checks whether a Run action contains minimal required fields.
func (a RunAction) Validate() error {
	if a.RunID == "" {
		return fmt.Errorf("run_id is required")
	}
	return a.Type.Validate()
}
