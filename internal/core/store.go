package core

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu    sync.Mutex
	tasks map[string]*Task
	order []string
}

func NewStore() *Store {
	return &Store{
		tasks: map[string]*Task{},
		order: []string{},
	}
}

func (s *Store) CreateTask(msg Message) *Task {
	now := time.Now()
	taskID := NewTaskID(now)
	contextID := msg.ContextID
	if contextID == "" {
		contextID = NewContextID(now)
	}

	status := TaskStatus{
		State:     TaskStateWorking,
		Timestamp: now,
	}
	task := &Task{
		ID:        taskID,
		ContextID: contextID,
		Status:    status,
		History:   []Message{msg},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskID] = task
	s.order = append(s.order, taskID)
	return cloneTask(task)
}

func (s *Store) EnsureTask(id string, contextID string) *Task {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if task, ok := s.tasks[id]; ok {
		return cloneTask(task)
	}
	now := time.Now()
	if contextID == "" {
		contextID = NewContextID(now)
	}
	task := &Task{
		ID:        id,
		ContextID: contextID,
		Status: TaskStatus{
			State:     TaskStateSubmitted,
			Timestamp: now,
		},
	}
	s.tasks[id] = task
	s.order = append(s.order, id)
	return cloneTask(task)
}

func (s *Store) UpdateTask(task *Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if task == nil {
		return
	}
	if _, ok := s.tasks[task.ID]; !ok {
		return
	}
	s.tasks[task.ID] = cloneTask(task)
}

func (s *Store) UpdateTaskStatus(taskID string, contextID string, status TaskStatus) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		now := time.Now()
		if contextID == "" {
			contextID = NewContextID(now)
		}
		task = &Task{
			ID:        taskID,
			ContextID: contextID,
		}
		s.tasks[taskID] = task
		s.order = append(s.order, taskID)
	}
	task.Status = status
}

func (s *Store) AppendMessage(taskID string, contextID string, msg Message) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		now := time.Now()
		if contextID == "" {
			contextID = NewContextID(now)
		}
		task = &Task{
			ID:        taskID,
			ContextID: contextID,
			Status: TaskStatus{
				State:     TaskStateSubmitted,
				Timestamp: now,
			},
		}
		s.tasks[taskID] = task
		s.order = append(s.order, taskID)
	}
	task.History = append(task.History, msg)
}

func (s *Store) AppendActivity(taskID string, contextID string, activity Activity) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		now := time.Now()
		if contextID == "" {
			contextID = NewContextID(now)
		}
		task = &Task{
			ID:        taskID,
			ContextID: contextID,
			Status: TaskStatus{
				State:     TaskStateSubmitted,
				Timestamp: now,
			},
		}
		s.tasks[taskID] = task
		s.order = append(s.order, taskID)
	}
	task.Activities = append(task.Activities, activity)
}

func (s *Store) GetTask(id string) (*Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return nil, false
	}
	return cloneTask(task), true
}

func (s *Store) ListTasks(offset int, limit int) ([]Task, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := len(s.order)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = total
	}
	if offset > total {
		return []Task{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := make([]Task, 0, end-offset)
	for _, id := range s.order[offset:end] {
		if task, ok := s.tasks[id]; ok {
			out = append(out, *cloneTask(task))
		}
	}
	return out, total
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	cp := *task
	if task.Artifacts != nil {
		cp.Artifacts = append([]Artifact{}, task.Artifacts...)
	}
	if task.History != nil {
		cp.History = append([]Message{}, task.History...)
	}
	if task.Activities != nil {
		cp.Activities = append([]Activity{}, task.Activities...)
	}
	if task.Metadata != nil {
		cp.Metadata = copyMap(task.Metadata)
	}
	return &cp
}

func copyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func TerminalState(state string) bool {
	switch state {
	case TaskStateCompleted, TaskStateFailed, TaskStateCanceled, TaskStateRejected:
		return true
	default:
		return false
	}
}

func ParsePageToken(token string) int {
	if token == "" {
		return 0
	}
	var offset int
	_, err := fmt.Sscanf(token, "offset:%d", &offset)
	if err != nil {
		return 0
	}
	if offset < 0 {
		return 0
	}
	return offset
}

func NextPageToken(offset int, limit int, total int) string {
	next := offset + limit
	if next >= total {
		return ""
	}
	return fmt.Sprintf("offset:%d", next)
}
