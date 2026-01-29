package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Status string

const (
	StatusRunning Status = "RUNNING"
	StatusDone    Status = "DONE"
	StatusFailed  Status = "FAILED"
)

type StepName string

const (
	StepInit       StepName = "INIT"
	StepSandboxRun StepName = "SANDBOX_RUN"
	StepVerify     StepName = "VERIFY"
	StepPack       StepName = "PACK"
)

type StepStatus string

const (
	StepStatusRunning StepStatus = "RUNNING"
	StepStatusDone    StepStatus = "DONE"
	StepStatusFailed  StepStatus = "FAILED"
)

type ErrorInfo struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Hint       string `json:"hint,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

type StepState struct {
	Name      StepName   `json:"name"`
	Status    StepStatus `json:"status"`
	StartedAt string     `json:"started_at,omitempty"`
	EndedAt   string     `json:"ended_at,omitempty"`
	Error     *ErrorInfo `json:"error,omitempty"`
}

type State struct {
	SchemaVersion int         `json:"schema_version"`
	TaskID        string      `json:"task_id"`
	RunID         string      `json:"run_id"`
	Status        Status      `json:"status"`
	CurrentStep   StepName    `json:"current_step,omitempty"`
	Steps         []StepState `json:"steps,omitempty"`
	LastError     *ErrorInfo  `json:"last_error,omitempty"`
}

func New(taskID, runID string) State {
	return State{
		SchemaVersion: 1,
		TaskID:        taskID,
		RunID:         runID,
		Status:        StatusRunning,
		Steps:         []StepState{},
	}
}

func (s *State) StartStep(name StepName) {
	s.CurrentStep = name
	s.Status = StatusRunning
	now := time.Now().Format(time.RFC3339)
	s.Steps = append(s.Steps, StepState{
		Name:      name,
		Status:    StepStatusRunning,
		StartedAt: now,
	})
}

func (s *State) EndStepSuccess() {
	if len(s.Steps) == 0 {
		return
	}
	now := time.Now().Format(time.RFC3339)
	last := &s.Steps[len(s.Steps)-1]
	last.Status = StepStatusDone
	last.EndedAt = now
	s.LastError = nil
}

func (s *State) FailStep(err ErrorInfo) {
	if len(s.Steps) > 0 {
		now := time.Now().Format(time.RFC3339)
		last := &s.Steps[len(s.Steps)-1]
		last.Status = StepStatusFailed
		last.EndedAt = now
		last.Error = &err
	}
	s.Status = StatusFailed
	s.LastError = &err
}

func (s *State) MarkDone() {
	s.Status = StatusDone
}

func ReadJSON(path string) (State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, err
	}
	return st, nil
}

func WriteJSON(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeFileAtomic(path, b, 0o644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp := filepath.Join(dir, "."+base+".tmp")
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic rename failed: %w", err)
	}
	return nil
}
