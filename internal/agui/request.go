package agui

import "strings"

type Resume struct {
	InterruptID string
	Payload     any
	Raw         map[string]any
}

type RunRequest struct {
	ThreadID string
	RunID    string
	Resume   *Resume

	Workflow string
	Raw      map[string]any
}

func parseRunRequest(raw map[string]any) RunRequest {
	req := RunRequest{
		ThreadID: firstString(raw, "threadId", "thread_id"),
		RunID:    firstString(raw, "runId", "run_id"),
		Workflow: strings.TrimSpace(firstString(raw, "workflow", "workflow_name")),
		Raw:      raw,
	}
	if req.Workflow == "" {
		req.Workflow = "demo"
	}

	if rm := asMap(raw["resume"]); rm != nil {
		resume := &Resume{
			InterruptID: firstString(rm, "interruptId", "interrupt_id", "interruptID"),
			Payload:     rm["payload"],
			Raw:         rm,
		}
		req.Resume = resume
	}
	return req
}
