package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDefaultAgentActionsAndProfileHelpers(t *testing.T) {
	tests := []struct {
		name    string
		role    AgentRole
		action  AgentAction
		allowed bool
	}{
		{name: "lead can expand flow", role: RoleLead, action: AgentActionExpandFlow, allowed: true},
		{name: "worker can write files", role: RoleWorker, action: AgentActionFSWrite, allowed: true},
		{name: "gate can approve", role: RoleGate, action: AgentActionApprove, allowed: true},
		{name: "support cannot use terminal", role: RoleSupport, action: AgentActionTerminal, allowed: false},
		{name: "unknown still gets common actions", role: AgentRole("unknown"), action: AgentActionReadContext, allowed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &AgentProfile{
				Role:         tt.role,
				Capabilities: []string{"go", "backend"},
			}
			if got := profile.HasAgentAction(tt.action); got != tt.allowed {
				t.Fatalf("HasAgentAction(%q) = %v, want %v", tt.action, got, tt.allowed)
			}
			if !profile.HasCapability("go") {
				t.Fatal("expected go capability to exist")
			}
			if profile.HasCapability("frontend") {
				t.Fatal("expected frontend capability to be absent")
			}
			if !profile.MatchesRequirements([]string{"go"}) {
				t.Fatal("expected requirements to match")
			}
			if profile.MatchesRequirements([]string{"go", "frontend"}) {
				t.Fatal("expected unmatched requirements to fail")
			}
		})
	}

	worker := &AgentProfile{Role: RoleWorker}
	caps := worker.EffectiveCapabilities()
	if !caps.FSRead || !caps.FSWrite || !caps.Terminal {
		t.Fatalf("worker capabilities = %+v, want all true", caps)
	}

	custom := &AgentProfile{
		Role:           RoleWorker,
		ActionsAllowed: []AgentAction{AgentActionApprove},
	}
	if got := custom.EffectiveAgentActions(); len(got) != 1 || got[0] != AgentActionApprove {
		t.Fatalf("EffectiveAgentActions() = %v, want only approve", got)
	}
	if !custom.HasAgentAction(AgentActionApprove) {
		t.Fatal("expected custom action to be allowed")
	}
	if custom.HasAgentAction(AgentActionFSWrite) {
		t.Fatal("expected default worker actions to be overridden")
	}
}

func TestDriverCapabilitiesCovers(t *testing.T) {
	max := DriverCapabilities{FSRead: true, FSWrite: true, Terminal: false}
	if !max.Covers(DriverCapabilities{FSRead: true, FSWrite: true}) {
		t.Fatal("expected capability set to be covered")
	}
	if max.Covers(DriverCapabilities{Terminal: true}) {
		t.Fatal("expected terminal request to be rejected")
	}
}

func TestSignalTypeIsTerminalAndTransientEvents(t *testing.T) {
	terminal := []SignalType{SignalComplete, SignalNeedHelp, SignalBlocked, SignalApprove, SignalReject}
	for _, sigType := range terminal {
		if !sigType.IsTerminal() {
			t.Fatalf("expected %q to be terminal", sigType)
		}
	}

	if SignalProgress.IsTerminal() {
		t.Fatal("expected progress to be non-terminal")
	}

	if !IsTransientAgentEvent(Event{
		Type: EventRunAgentOutput,
		Data: map[string]any{"type": "agent_message_chunk"},
	}) {
		t.Fatal("expected chunked run agent output to be transient")
	}

	if IsTransientAgentEvent(Event{
		Type: EventRunAgentOutput,
		Data: map[string]any{"type": "agent_message"},
	}) {
		t.Fatal("expected aggregated agent output to be persisted")
	}

	if IsTransientAgentEvent(Event{Type: EventActionStarted}) {
		t.Fatal("expected non-agent output event to be persisted")
	}
}

func TestResourceBindingAttachmentHelpers(t *testing.T) {
	binding := NewAttachmentBinding(42, "report.txt", "/tmp/report.txt", "text/plain", 128)
	if binding.Kind != ResourceKindAttachment {
		t.Fatalf("Kind = %q, want %q", binding.Kind, ResourceKindAttachment)
	}
	if binding.AttachmentFileName() != "report.txt" {
		t.Fatalf("AttachmentFileName() = %q", binding.AttachmentFileName())
	}
	if binding.AttachmentFilePath() != "/tmp/report.txt" {
		t.Fatalf("AttachmentFilePath() = %q", binding.AttachmentFilePath())
	}
	if binding.AttachmentMimeType() != "text/plain" {
		t.Fatalf("AttachmentMimeType() = %q", binding.AttachmentMimeType())
	}
	if binding.AttachmentSize() != 128 {
		t.Fatalf("AttachmentSize() = %d", binding.AttachmentSize())
	}

	binding.Config["size"] = float64(256)
	if binding.AttachmentSize() != 256 {
		t.Fatalf("AttachmentSize() with float64 = %d", binding.AttachmentSize())
	}

	empty := &ResourceBinding{}
	if empty.AttachmentMimeType() != "" || empty.AttachmentSize() != 0 {
		t.Fatalf("empty attachment helpers returned unexpected values: mime=%q size=%d", empty.AttachmentMimeType(), empty.AttachmentSize())
	}
}

func TestRunHasResult(t *testing.T) {
	run := &Run{}
	if run.HasResult() {
		t.Fatal("expected empty result to be false")
	}

	run.ResultMarkdown = "done"
	if !run.HasResult() {
		t.Fatal("expected non-empty result to be true")
	}
}

func TestProbeSignalConversions(t *testing.T) {
	now := time.Now().UTC().Round(0)
	later := now.Add(2 * time.Minute)
	agentContextID := int64(77)
	probe := &RunProbe{
		ID:             9,
		RunID:          11,
		WorkItemID:     12,
		ActionID:       13,
		AgentContextID: &agentContextID,
		SessionID:      "session-1",
		OwnerID:        "owner-1",
		TriggerSource:  RunProbeTriggerManual,
		Question:       "still alive?",
		Status:         RunProbeSent,
		Verdict:        RunProbeAlive,
		ReplyText:      "yes",
		Error:          "none",
		SentAt:         &now,
		AnsweredAt:     &later,
		CreatedAt:      now,
	}

	request := NewProbeRequestSignal(probe)
	if request.Type != SignalProbeRequest || request.Source != SignalSourceSystem {
		t.Fatalf("unexpected request signal: %+v", request)
	}
	if request.Payload["session_id"] != "session-1" {
		t.Fatalf("session_id payload missing: %+v", request.Payload)
	}

	response := NewProbeResponseSignal(probe)
	if response.Type != SignalProbeResponse {
		t.Fatalf("unexpected response signal type: %q", response.Type)
	}
	if response.Payload["reply_text"] != "yes" {
		t.Fatalf("reply_text payload missing: %+v", response.Payload)
	}

	restored := ProbeFromSignal(&ActionSignal{
		ID:         9,
		RunID:      11,
		WorkItemID: 12,
		ActionID:   13,
		CreatedAt:  now,
		Payload: map[string]any{
			"trigger_source":   string(RunProbeTriggerWatchdog),
			"question":         "status?",
			"status":           string(RunProbeAnswered),
			"verdict":          string(RunProbeBlocked),
			"session_id":       "session-2",
			"owner_id":         "owner-2",
			"reply_text":       "blocked",
			"error":            "timeout",
			"agent_context_id": float64(91),
			"sent_at":          now.Format(time.RFC3339Nano),
			"answered_at":      later.Format(time.RFC3339Nano),
		},
	})
	if restored == nil {
		t.Fatal("expected restored probe")
	}
	if restored.TriggerSource != RunProbeTriggerWatchdog || restored.Verdict != RunProbeBlocked {
		t.Fatalf("unexpected restored probe: %+v", restored)
	}
	if restored.AgentContextID == nil || *restored.AgentContextID != 91 {
		t.Fatalf("expected agent_context_id=91, got %+v", restored.AgentContextID)
	}
	if restored.SentAt == nil || restored.AnsweredAt == nil {
		t.Fatalf("expected restored timestamps, got %+v", restored)
	}

	invalidTimes := ProbeFromSignal(&ActionSignal{
		Payload: map[string]any{
			"sent_at":     "bad-time",
			"answered_at": "bad-time",
		},
	})
	if invalidTimes.SentAt != nil || invalidTimes.AnsweredAt != nil {
		t.Fatalf("expected invalid times to be ignored: %+v", invalidTimes)
	}
	if ProbeFromSignal(nil) != nil {
		t.Fatal("expected nil signal to produce nil probe")
	}
}

func TestJournalEntryConversions(t *testing.T) {
	now := time.Now().UTC().Round(0)

	if EventToJournalEntry(nil) != nil {
		t.Fatal("expected nil event to produce nil journal entry")
	}
	if EventToJournalEntry(&Event{Type: EventChatOutput}) != nil {
		t.Fatal("expected chat events to be skipped")
	}
	if EventToJournalEntry(&Event{Type: EventThreadMessage}) != nil {
		t.Fatal("expected thread events to be skipped")
	}
	if EventToJournalEntry(&Event{Type: EventNotificationCreated}) != nil {
		t.Fatal("expected notification events to be skipped")
	}

	toolAudit := EventToJournalEntry(&Event{
		Type:       EventRunStarted,
		Category:   EventCategoryToolAudit,
		WorkItemID: 1,
		ActionID:   2,
		RunID:      3,
		Data:       map[string]any{"tool_name": "functions.shell_command"},
		Timestamp:  now,
	})
	if toolAudit == nil || toolAudit.Kind != JournalToolCall || toolAudit.Source != JournalSourceAgent {
		t.Fatalf("unexpected tool audit journal entry: %+v", toolAudit)
	}
	if toolAudit.Summary != "functions.shell_command" {
		t.Fatalf("unexpected tool audit summary: %q", toolAudit.Summary)
	}

	stateChange := EventToJournalEntry(&Event{
		Type:       EventWorkItemStarted,
		WorkItemID: 5,
		Data:       map[string]any{"reason": "manual"},
		Timestamp:  now,
	})
	if stateChange == nil || stateChange.Kind != JournalStateChange {
		t.Fatalf("unexpected state change journal entry: %+v", stateChange)
	}

	signalEntry := ActionSignalToJournalEntry(&ActionSignal{
		WorkItemID:     5,
		ActionID:       6,
		RunID:          7,
		Type:           SignalComplete,
		Source:         SignalSourceAgent,
		Summary:        "done",
		Content:        "all good",
		Payload:        map[string]any{"foo": "bar"},
		Actor:          "agent-1",
		SourceActionID: 8,
		CreatedAt:      now,
	})
	if signalEntry == nil || signalEntry.Kind != JournalSignal {
		t.Fatalf("unexpected signal journal entry: %+v", signalEntry)
	}
	if signalEntry.Payload["signal_type"] != string(SignalComplete) || signalEntry.Payload["content"] != "all good" {
		t.Fatalf("expected signal payload to be merged: %+v", signalEntry.Payload)
	}

	progressEntry := ActionSignalToJournalEntry(&ActionSignal{
		Type:      SignalProgress,
		Source:    SignalSourceAgent,
		Payload:   map[string]any{},
		CreatedAt: now,
	})
	if progressEntry.Kind != JournalAgentOutput {
		t.Fatalf("expected progress to map to agent_output, got %q", progressEntry.Kind)
	}

	usageEntry := UsageRecordToJournalEntry(&UsageRecord{
		RunID:            10,
		WorkItemID:       11,
		ActionID:         12,
		AgentID:          "agent-2",
		ProfileID:        "worker",
		ModelID:          "gpt-test",
		InputTokens:      100,
		OutputTokens:     50,
		CacheReadTokens:  10,
		CacheWriteTokens: 5,
		ReasoningTokens:  20,
		TotalTokens:      185,
		DurationMs:       3210,
		CreatedAt:        now,
	})
	if usageEntry == nil || usageEntry.Kind != JournalUsage || usageEntry.Actor != "agent-2" {
		t.Fatalf("unexpected usage journal entry: %+v", usageEntry)
	}
	if usageEntry.Payload["total_tokens"] != int64(185) {
		t.Fatalf("unexpected usage payload: %+v", usageEntry.Payload)
	}

	if ActionSignalToJournalEntry(nil) != nil {
		t.Fatal("expected nil action signal to produce nil journal entry")
	}
	if UsageRecordToJournalEntry(nil) != nil {
		t.Fatal("expected nil usage record to produce nil journal entry")
	}
}

func TestToolCallAuditEventConversions(t *testing.T) {
	now := time.Now().UTC().Round(0)
	later := now.Add(time.Second)
	exitCode := 7

	audit := &ToolCallAudit{
		ID:             1,
		WorkItemID:     2,
		ActionID:       3,
		RunID:          4,
		SessionID:      "session-1",
		ToolCallID:     "call-1",
		ToolName:       "functions.shell_command",
		Status:         "completed",
		StartedAt:      &now,
		FinishedAt:     &later,
		DurationMs:     1000,
		ExitCode:       &exitCode,
		InputDigest:    "in",
		OutputDigest:   "out",
		StdoutDigest:   "stdout",
		StderrDigest:   "stderr",
		InputPreview:   "input",
		OutputPreview:  "output",
		StdoutPreview:  "stdout preview",
		StderrPreview:  "stderr preview",
		RedactionLevel: "basic",
		CreatedAt:      now,
	}

	event := audit.ToEvent()
	if event.Category != EventCategoryToolAudit || event.Type != "tool_call_audit" {
		t.Fatalf("unexpected tool audit event: %+v", event)
	}
	if event.Data["exit_code"] != exitCode {
		t.Fatalf("unexpected exit code payload: %+v", event.Data)
	}

	restored := ToolCallAuditFromEvent(&Event{
		ID:         1,
		WorkItemID: 2,
		ActionID:   3,
		RunID:      4,
		Timestamp:  now,
		Data: map[string]any{
			"session_id":      "session-1",
			"tool_call_id":    "call-1",
			"tool_name":       "functions.shell_command",
			"status":          "completed",
			"duration_ms":     json.Number("1000"),
			"exit_code":       json.Number("7"),
			"input_digest":    "in",
			"output_digest":   "out",
			"stdout_digest":   "stdout",
			"stderr_digest":   "stderr",
			"input_preview":   "input",
			"output_preview":  "output",
			"stdout_preview":  "stdout preview",
			"stderr_preview":  "stderr preview",
			"redaction_level": "basic",
			"started_at":      now.Format(time.RFC3339Nano),
			"finished_at":     later.Format(time.RFC3339Nano),
		},
	})
	if restored == nil {
		t.Fatal("expected restored audit")
	}
	if restored.DurationMs != 1000 || restored.ExitCode == nil || *restored.ExitCode != 7 {
		t.Fatalf("unexpected restored audit numerics: %+v", restored)
	}
	if restored.StartedAt == nil || restored.FinishedAt == nil {
		t.Fatalf("expected restored timestamps: %+v", restored)
	}

	floatAudit := ToolCallAuditFromEvent(&Event{
		Data: map[string]any{
			"duration_ms": float64(2500),
			"exit_code":   float64(0),
		},
	})
	if floatAudit.DurationMs != 2500 || floatAudit.ExitCode == nil || *floatAudit.ExitCode != 0 {
		t.Fatalf("unexpected float-based audit restoration: %+v", floatAudit)
	}

	if ToolCallAuditFromEvent(nil) != nil {
		t.Fatal("expected nil event to produce nil audit")
	}
}
