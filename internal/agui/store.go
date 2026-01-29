package agui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yoke233/zhanggui/internal/gateway"
)

type RunMeta struct {
	SchemaVersion int    `json:"schema_version"`
	Protocol      string `json:"protocol"`
	ThreadID      string `json:"threadId,omitempty"`
	RunID         string `json:"runId"`
	ParentRunID   string `json:"parentRunId,omitempty"`
	Input         any    `json:"input,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

type PendingToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type Interrupt struct {
	ID        string `json:"id"`
	Reason    string `json:"reason,omitempty"`
	Payload   any    `json:"payload,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type ErrorInfo struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type RunState struct {
	SchemaVersion int              `json:"schema_version"`
	Protocol      string           `json:"protocol"`
	ThreadID      string           `json:"threadId,omitempty"`
	RunID         string           `json:"runId"`
	Status        string           `json:"status"`
	CurrentStep   string           `json:"currentStep,omitempty"`
	PendingTool   *PendingToolCall `json:"pendingToolCall,omitempty"`
	PendingInt    *Interrupt       `json:"pendingInterrupt,omitempty"`
	Data          map[string]any   `json:"data,omitempty"`
	LastError     *ErrorInfo       `json:"lastError,omitempty"`
	UpdatedAt     string           `json:"updatedAt"`
}

func writeRunMeta(gw *gateway.Gateway, meta RunMeta) error {
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return gw.CreateFile("run.json", b, 0o644, "write run.json")
}

func writeRunState(gw *gateway.Gateway, st RunState) error {
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return gw.ReplaceFile("state.json", b, 0o644, "write state.json")
}

func readRunMeta(runRoot string) (RunMeta, error) {
	var meta RunMeta
	b, err := os.ReadFile(filepath.Join(runRoot, "run.json"))
	if err != nil {
		return RunMeta{}, err
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return RunMeta{}, err
	}
	return meta, nil
}

func readRunState(runRoot string) (RunState, error) {
	var st RunState
	b, err := os.ReadFile(filepath.Join(runRoot, "state.json"))
	if err != nil {
		return RunState{}, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return RunState{}, err
	}
	return st, nil
}

type Event map[string]any

type EventLog struct {
	Gateway      *gateway.Gateway
	EventsRel    string
	AppendPerm   os.FileMode
	AppendDetail string
}

func (l *EventLog) Append(ev Event) error {
	if l == nil || l.Gateway == nil {
		return fmt.Errorf("event log not initialized")
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	perm := l.AppendPerm
	if perm == 0 {
		perm = 0o644
	}
	return l.Gateway.AppendFile(filepath.ToSlash(l.EventsRel), b, perm, l.AppendDetail)
}
