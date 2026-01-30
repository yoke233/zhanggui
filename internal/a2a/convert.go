package a2a

import (
	"strings"
	"time"

	a2ago "github.com/a2aproject/a2a-go/a2a"
)

func toA2AMessage(msg Message) *a2ago.Message {
	out := &a2ago.Message{
		ID:        msg.MessageID,
		ContextID: msg.ContextID,
		TaskID:    a2ago.TaskID(strings.TrimSpace(msg.TaskID)),
		Role:      toA2ARole(msg.Role),
		Metadata:  msg.Metadata,
		Extensions: func() []string {
			if len(msg.Extensions) == 0 {
				return nil
			}
			return append([]string{}, msg.Extensions...)
		}(),
	}
	if len(msg.ReferenceTaskIDs) > 0 {
		out.ReferenceTasks = make([]a2ago.TaskID, 0, len(msg.ReferenceTaskIDs))
		for _, id := range msg.ReferenceTaskIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			out.ReferenceTasks = append(out.ReferenceTasks, a2ago.TaskID(id))
		}
	}
	if len(msg.Parts) > 0 {
		out.Parts = make(a2ago.ContentParts, 0, len(msg.Parts))
		for _, part := range msg.Parts {
			if converted := toA2APart(part); converted != nil {
				out.Parts = append(out.Parts, converted)
			}
		}
	}
	return out
}

func toA2APart(p Part) a2ago.Part {
	if p.Text != nil && strings.TrimSpace(*p.Text) != "" {
		return a2ago.TextPart{Text: *p.Text, Metadata: p.Metadata}
	}
	if p.Data != nil {
		if dataMap, ok := p.Data.(map[string]any); ok {
			return a2ago.DataPart{Data: dataMap, Metadata: p.Metadata}
		}
		return a2ago.DataPart{Data: map[string]any{"value": p.Data}, Metadata: p.Metadata}
	}

	meta := a2ago.FileMeta{
		Name:     strings.TrimSpace(p.Filename),
		MimeType: strings.TrimSpace(p.MediaType),
	}
	if p.Raw != nil && strings.TrimSpace(*p.Raw) != "" {
		return a2ago.FilePart{
			File:     a2ago.FileBytes{FileMeta: meta, Bytes: *p.Raw},
			Metadata: p.Metadata,
		}
	}
	if p.URL != nil && strings.TrimSpace(*p.URL) != "" {
		return a2ago.FilePart{
			File:     a2ago.FileURI{FileMeta: meta, URI: *p.URL},
			Metadata: p.Metadata,
		}
	}
	return nil
}

func fromA2ATask(task *a2ago.Task, includeArtifacts bool, historyLength *int) Task {
	if task == nil {
		return Task{}
	}
	cp := Task{
		ID:        string(task.ID),
		ContextID: task.ContextID,
		Status:    fromA2AStatus(task.Status),
		Metadata:  task.Metadata,
	}
	if includeArtifacts && len(task.Artifacts) > 0 {
		cp.Artifacts = make([]Artifact, 0, len(task.Artifacts))
		for _, art := range task.Artifacts {
			cp.Artifacts = append(cp.Artifacts, fromA2AArtifact(art))
		}
	}
	if historyLength != nil && *historyLength >= 0 {
		if *historyLength == 0 {
			cp.History = nil
		} else if len(task.History) > 0 {
			history := task.History
			if len(history) > *historyLength {
				history = history[len(history)-*historyLength:]
			}
			cp.History = make([]Message, 0, len(history))
			for _, msg := range history {
				cp.History = append(cp.History, fromA2AMessage(msg))
			}
		}
	}
	return cp
}

func fromA2AStatus(status a2ago.TaskStatus) TaskStatus {
	out := TaskStatus{
		State: fromA2AState(status.State),
	}
	if status.Message != nil {
		msg := fromA2AMessage(status.Message)
		out.Message = &msg
	}
	if status.Timestamp != nil {
		out.Timestamp = status.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func fromA2AMessage(msg *a2ago.Message) Message {
	if msg == nil {
		return Message{}
	}
	out := Message{
		MessageID:        msg.ID,
		ContextID:        msg.ContextID,
		TaskID:           string(msg.TaskID),
		Role:             fromA2ARole(msg.Role),
		Metadata:         msg.Metadata,
		Extensions:       msg.Extensions,
		ReferenceTaskIDs: toStringIDs(msg.ReferenceTasks),
	}
	if len(msg.Parts) > 0 {
		out.Parts = make([]Part, 0, len(msg.Parts))
		for _, part := range msg.Parts {
			out.Parts = append(out.Parts, fromA2APart(part))
		}
	}
	return out
}

func fromA2APart(part a2ago.Part) Part {
	switch p := part.(type) {
	case a2ago.TextPart:
		text := p.Text
		return Part{Text: &text, Metadata: p.Metadata}
	case a2ago.DataPart:
		return Part{Data: p.Data, Metadata: p.Metadata}
	case a2ago.FilePart:
		out := Part{Metadata: p.Metadata}
		switch file := p.File.(type) {
		case a2ago.FileBytes:
			raw := file.Bytes
			out.Raw = &raw
			out.Filename = file.Name
			out.MediaType = file.MimeType
		case a2ago.FileURI:
			url := file.URI
			out.URL = &url
			out.Filename = file.Name
			out.MediaType = file.MimeType
		}
		return out
	default:
		return Part{}
	}
}

func fromA2AArtifact(art *a2ago.Artifact) Artifact {
	if art == nil {
		return Artifact{}
	}
	out := Artifact{
		ArtifactID:  string(art.ID),
		Name:        art.Name,
		Description: art.Description,
		Metadata:    art.Metadata,
		Extensions:  art.Extensions,
	}
	if len(art.Parts) > 0 {
		out.Parts = make([]Part, 0, len(art.Parts))
		for _, part := range art.Parts {
			out.Parts = append(out.Parts, fromA2APart(part))
		}
	}
	return out
}

func buildEchoArtifact(msg *a2ago.Message) *a2ago.Artifact {
	text := "OK"
	if msg != nil {
		for _, part := range msg.Parts {
			if tp, ok := part.(a2ago.TextPart); ok {
				if strings.TrimSpace(tp.Text) != "" {
					text = tp.Text
					break
				}
			}
		}
	}
	return &a2ago.Artifact{
		ID:   a2ago.NewArtifactID(),
		Name: "echo",
		Parts: a2ago.ContentParts{
			a2ago.TextPart{Text: text},
		},
	}
}

func toStringIDs(ids []a2ago.TaskID) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func toA2AState(state string) a2ago.TaskState {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case TaskStateAuthRequired:
		return a2ago.TaskStateAuthRequired
	case TaskStateCanceled:
		return a2ago.TaskStateCanceled
	case TaskStateCompleted:
		return a2ago.TaskStateCompleted
	case TaskStateFailed:
		return a2ago.TaskStateFailed
	case TaskStateInputRequired:
		return a2ago.TaskStateInputRequired
	case TaskStateRejected:
		return a2ago.TaskStateRejected
	case TaskStateSubmitted:
		return a2ago.TaskStateSubmitted
	case TaskStateWorking:
		return a2ago.TaskStateWorking
	case TaskStateUnspecified:
		return a2ago.TaskStateUnspecified
	}
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "auth-required":
		return a2ago.TaskStateAuthRequired
	case "canceled":
		return a2ago.TaskStateCanceled
	case "completed":
		return a2ago.TaskStateCompleted
	case "failed":
		return a2ago.TaskStateFailed
	case "input-required":
		return a2ago.TaskStateInputRequired
	case "rejected":
		return a2ago.TaskStateRejected
	case "submitted":
		return a2ago.TaskStateSubmitted
	case "working":
		return a2ago.TaskStateWorking
	default:
		return a2ago.TaskStateUnspecified
	}
}

func fromA2AState(state a2ago.TaskState) string {
	switch state {
	case a2ago.TaskStateAuthRequired:
		return TaskStateAuthRequired
	case a2ago.TaskStateCanceled:
		return TaskStateCanceled
	case a2ago.TaskStateCompleted:
		return TaskStateCompleted
	case a2ago.TaskStateFailed:
		return TaskStateFailed
	case a2ago.TaskStateInputRequired:
		return TaskStateInputRequired
	case a2ago.TaskStateRejected:
		return TaskStateRejected
	case a2ago.TaskStateSubmitted:
		return TaskStateSubmitted
	case a2ago.TaskStateWorking:
		return TaskStateWorking
	default:
		return TaskStateUnspecified
	}
}

func toA2ARole(role string) a2ago.MessageRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "role_user", "user":
		return a2ago.MessageRoleUser
	case "role_agent", "agent", "assistant", "system":
		return a2ago.MessageRoleAgent
	default:
		return a2ago.MessageRoleUnspecified
	}
}

func fromA2ARole(role a2ago.MessageRole) string {
	switch role {
	case a2ago.MessageRoleUser:
		return RoleUser
	case a2ago.MessageRoleAgent:
		return RoleAgent
	default:
		return ""
	}
}
