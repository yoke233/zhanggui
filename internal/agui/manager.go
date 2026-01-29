package agui

import (
	"fmt"
	"sync"
)

type ToolResult struct {
	ThreadID   string
	RunID      string
	ToolCallID string
	Content    any
}

type Session struct {
	ToolResults chan ToolResult
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{sessions: map[string]*Session{}}
}

func (m *Manager) Start(runID string) (*Session, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := &Session{ToolResults: make(chan ToolResult, 8)}
	m.sessions[runID] = s
	return s, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.sessions, runID)
		close(s.ToolResults)
	}
}

func (m *Manager) DeliverToolResult(runID string, tr ToolResult) error {
	m.mu.Lock()
	s := m.sessions[runID]
	m.mu.Unlock()
	if s == nil {
		return fmt.Errorf("run not active: %s", runID)
	}
	select {
	case s.ToolResults <- tr:
		return nil
	default:
		return fmt.Errorf("tool result queue full: %s", runID)
	}
}
