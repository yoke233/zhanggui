package a2a

import (
	"strings"
	"sync"
	"time"

	a2ago "github.com/a2aproject/a2a-go/a2a"
)

type Store struct {
	mu    sync.Mutex
	tasks map[a2ago.TaskID]*a2ago.Task
	order []a2ago.TaskID
}

func NewStore() *Store {
	return &Store{
		tasks: map[a2ago.TaskID]*a2ago.Task{},
		order: []a2ago.TaskID{},
	}
}

func (s *Store) UpsertMessage(msg *a2ago.Message) *a2ago.Task {
	if msg == nil {
		return nil
	}
	if strings.TrimSpace(msg.ID) == "" {
		msg.ID = a2ago.NewMessageID()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.TaskID != "" {
		if task, ok := s.tasks[msg.TaskID]; ok {
			if msg.ContextID == "" {
				msg.ContextID = task.ContextID
			}
			task.History = append(task.History, cloneMessage(msg))
			return cloneTask(task)
		}
	}

	taskID := msg.TaskID
	if taskID == "" {
		taskID = a2ago.NewTaskID()
	}
	contextID := msg.ContextID
	if contextID == "" {
		contextID = a2ago.NewContextID()
	}
	msg.TaskID = taskID
	msg.ContextID = contextID

	task := &a2ago.Task{
		ID:        taskID,
		ContextID: contextID,
		Status:    a2ago.TaskStatus{State: a2ago.TaskStateSubmitted, Timestamp: nowPtr()},
		History:   []*a2ago.Message{cloneMessage(msg)},
	}
	s.tasks[taskID] = task
	s.order = append(s.order, taskID)
	return cloneTask(task)
}

func (s *Store) EnsureTask(id string, contextID string) *a2ago.Task {
	if strings.TrimSpace(id) == "" {
		return nil
	}

	taskID := a2ago.TaskID(strings.TrimSpace(id))

	s.mu.Lock()
	defer s.mu.Unlock()

	if task, ok := s.tasks[taskID]; ok {
		return cloneTask(task)
	}

	if strings.TrimSpace(contextID) == "" {
		contextID = a2ago.NewContextID()
	}
	task := &a2ago.Task{
		ID:        taskID,
		ContextID: contextID,
		Status:    a2ago.TaskStatus{State: a2ago.TaskStateSubmitted, Timestamp: nowPtr()},
	}
	s.tasks[taskID] = task
	s.order = append(s.order, taskID)
	return cloneTask(task)
}

func (s *Store) UpdateTask(task *a2ago.Task) {
	if task == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[task.ID]; !ok {
		return
	}
	s.tasks[task.ID] = cloneTask(task)
}

func (s *Store) UpdateTaskStatus(taskID string, contextID string, status a2ago.TaskStatus) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	if status.Timestamp == nil {
		status.Timestamp = nowPtr()
	}

	id := a2ago.TaskID(strings.TrimSpace(taskID))

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		if strings.TrimSpace(contextID) == "" {
			contextID = a2ago.NewContextID()
		}
		task = &a2ago.Task{
			ID:        id,
			ContextID: contextID,
			Status:    status,
		}
		s.tasks[id] = task
		s.order = append(s.order, id)
		return
	}
	task.Status = status
}

func (s *Store) AppendMessage(taskID string, contextID string, msg *a2ago.Message) {
	if strings.TrimSpace(taskID) == "" || msg == nil {
		return
	}
	if strings.TrimSpace(msg.ID) == "" {
		msg.ID = a2ago.NewMessageID()
	}

	id := a2ago.TaskID(strings.TrimSpace(taskID))

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		if strings.TrimSpace(contextID) == "" {
			contextID = a2ago.NewContextID()
		}
		task = &a2ago.Task{
			ID:        id,
			ContextID: contextID,
			Status:    a2ago.TaskStatus{State: a2ago.TaskStateSubmitted, Timestamp: nowPtr()},
		}
		s.tasks[id] = task
		s.order = append(s.order, id)
	}

	msg.TaskID = id
	if msg.ContextID == "" {
		msg.ContextID = task.ContextID
	}
	task.History = append(task.History, cloneMessage(msg))
}

func (s *Store) AppendActivity(taskID string, contextID string, activity map[string]any) {
	if strings.TrimSpace(taskID) == "" || activity == nil {
		return
	}

	id := a2ago.TaskID(strings.TrimSpace(taskID))

	entry := map[string]any{
		"type":      "A2UI",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"content":   activity,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		if strings.TrimSpace(contextID) == "" {
			contextID = a2ago.NewContextID()
		}
		task = &a2ago.Task{
			ID:        id,
			ContextID: contextID,
			Status:    a2ago.TaskStatus{State: a2ago.TaskStateSubmitted, Timestamp: nowPtr()},
		}
		s.tasks[id] = task
		s.order = append(s.order, id)
	}
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	list, _ := task.Metadata["activities"].([]any)
	task.Metadata["activities"] = append(list, entry)
}

func (s *Store) GetTask(id string) (*a2ago.Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[a2ago.TaskID(strings.TrimSpace(id))]
	if !ok {
		return nil, false
	}
	return cloneTask(task), true
}

func (s *Store) ListTasks(offset int, limit int) ([]*a2ago.Task, int) {
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
		return []*a2ago.Task{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := make([]*a2ago.Task, 0, end-offset)
	for _, id := range s.order[offset:end] {
		if task, ok := s.tasks[id]; ok {
			out = append(out, cloneTask(task))
		}
	}
	return out, total
}

func nowPtr() *time.Time {
	t := time.Now().UTC()
	return &t
}

func cloneTask(task *a2ago.Task) *a2ago.Task {
	if task == nil {
		return nil
	}
	cp := *task
	if task.Metadata != nil {
		cp.Metadata = copyMap(task.Metadata)
	}
	if task.History != nil {
		cp.History = make([]*a2ago.Message, 0, len(task.History))
		for _, msg := range task.History {
			cp.History = append(cp.History, cloneMessage(msg))
		}
	}
	if task.Artifacts != nil {
		cp.Artifacts = make([]*a2ago.Artifact, 0, len(task.Artifacts))
		for _, art := range task.Artifacts {
			cp.Artifacts = append(cp.Artifacts, cloneArtifact(art))
		}
	}
	if task.Status.Message != nil {
		cp.Status.Message = cloneMessage(task.Status.Message)
	}
	if task.Status.Timestamp != nil {
		ts := *task.Status.Timestamp
		cp.Status.Timestamp = &ts
	}
	return &cp
}

func cloneMessage(msg *a2ago.Message) *a2ago.Message {
	if msg == nil {
		return nil
	}
	cp := *msg
	if msg.Metadata != nil {
		cp.Metadata = copyMap(msg.Metadata)
	}
	if msg.Parts != nil {
		cp.Parts = cloneParts(msg.Parts)
	}
	if msg.ReferenceTasks != nil {
		cp.ReferenceTasks = append([]a2ago.TaskID{}, msg.ReferenceTasks...)
	}
	if msg.Extensions != nil {
		cp.Extensions = append([]string{}, msg.Extensions...)
	}
	return &cp
}

func cloneArtifact(art *a2ago.Artifact) *a2ago.Artifact {
	if art == nil {
		return nil
	}
	cp := *art
	if art.Metadata != nil {
		cp.Metadata = copyMap(art.Metadata)
	}
	if art.Parts != nil {
		cp.Parts = cloneParts(art.Parts)
	}
	if art.Extensions != nil {
		cp.Extensions = append([]string{}, art.Extensions...)
	}
	return &cp
}

func cloneParts(parts a2ago.ContentParts) a2ago.ContentParts {
	if parts == nil {
		return nil
	}
	cp := make(a2ago.ContentParts, 0, len(parts))
	for _, part := range parts {
		cp = append(cp, clonePart(part))
	}
	return cp
}

func clonePart(part a2ago.Part) a2ago.Part {
	switch p := part.(type) {
	case a2ago.TextPart:
		cp := p
		if p.Metadata != nil {
			cp.Metadata = copyMap(p.Metadata)
		}
		return cp
	case a2ago.DataPart:
		cp := p
		if p.Metadata != nil {
			cp.Metadata = copyMap(p.Metadata)
		}
		if p.Data != nil {
			cp.Data = copyMap(p.Data)
		}
		return cp
	case a2ago.FilePart:
		cp := p
		if p.Metadata != nil {
			cp.Metadata = copyMap(p.Metadata)
		}
		switch f := p.File.(type) {
		case a2ago.FileBytes:
			cp.File = f
		case a2ago.FileURI:
			cp.File = f
		default:
			cp.File = f
		}
		return cp
	default:
		return part
	}
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
