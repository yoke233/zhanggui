package sqlite

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
)

func TestMarshalJSONAndUnmarshalNullJSON(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		wantNil bool
		want    string
	}{
		{name: "nil", value: nil, wantNil: true},
		{name: "empty map", value: map[string]any{}, wantNil: true},
		{name: "empty slice", value: []string{}, wantNil: true},
		{name: "populated map", value: map[string]any{"a": 1}, want: "{\"a\":1}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := marshalJSON(tt.value)
			if err != nil {
				t.Fatalf("marshalJSON(%v) error = %v", tt.value, err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("marshalJSON(%v) = %v, want nil", tt.value, got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("marshalJSON(%v) = %v, want %q", tt.value, got, tt.want)
			}
		})
	}

	if _, err := marshalJSON(make(chan int)); err == nil {
		t.Fatal("expected marshalJSON to reject unsupported values")
	}

	var dest map[string]any
	unmarshalNullJSON(sql.NullString{Valid: false}, &dest)
	if dest != nil {
		t.Fatalf("expected invalid null string to keep destination zero, got %+v", dest)
	}

	unmarshalNullJSON(sql.NullString{Valid: true, String: "{\"answer\":42}"}, &dest)
	if dest["answer"] != float64(42) {
		t.Fatalf("unexpected unmarshaled destination: %+v", dest)
	}
}

func TestJSONFieldValueAndScan(t *testing.T) {
	field := JSONField[map[string]any]{Data: map[string]any{"ok": true}}
	value, err := field.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if value != "{\"ok\":true}" {
		t.Fatalf("Value() = %v, want JSON string", value)
	}

	emptyField := JSONField[[]string]{Data: []string{}}
	value, err = emptyField.Value()
	if err != nil {
		t.Fatalf("Value() empty error = %v", err)
	}
	if value != nil {
		t.Fatalf("Value() for empty slice = %v, want nil", value)
	}

	var scanMap JSONField[map[string]any]
	if err := scanMap.Scan("{\"x\":1}"); err != nil {
		t.Fatalf("Scan(string) error = %v", err)
	}
	if scanMap.Data["x"] != float64(1) {
		t.Fatalf("Scan(string) produced %+v", scanMap.Data)
	}

	var scanSlice JSONField[[]string]
	if err := scanSlice.Scan([]byte("[\"a\",\"b\"]")); err != nil {
		t.Fatalf("Scan([]byte) error = %v", err)
	}
	if len(scanSlice.Data) != 2 || scanSlice.Data[1] != "b" {
		t.Fatalf("Scan([]byte) produced %+v", scanSlice.Data)
	}

	if err := scanSlice.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error = %v", err)
	}
	if len(scanSlice.Data) != 0 {
		t.Fatalf("Scan(nil) expected zero value, got %+v", scanSlice.Data)
	}

	if err := scanSlice.Scan(123); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestStoreHelpers(t *testing.T) {
	if startupDBError("test.db", "open", nil) != nil {
		t.Fatal("expected nil error to stay nil")
	}

	msgErr := startupDBError("test.db", "open", sql.ErrConnDone)
	if msgErr == nil || !strings.Contains(msgErr.Error(), "open test.db") {
		t.Fatalf("unexpected startupDBError message: %v", msgErr)
	}

	store := &Store{}
	if store.cloneWithORM(nil) == nil {
		t.Fatal("expected cloneWithORM to return a store clone")
	}
	if (*Store)(nil).cloneWithORM(nil) != nil {
		t.Fatal("expected nil store clone to stay nil")
	}

	if err := store.InTx(t.Context(), func(core.Store) error { return nil }); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("expected nil orm error, got %v", err)
	}
}

func TestModelConversions(t *testing.T) {
	now := time.Now().UTC().Round(0)
	projectID := int64(10)
	issueID := int64(11)
	actionID := int64(12)
	runID := int64(13)
	replyTo := int64(14)
	refID := "ref-1"
	readAt := now.Add(time.Minute)

	thread := &core.Thread{
		ID:        1,
		Title:     "thread",
		Status:    core.ThreadActive,
		OwnerID:   "owner",
		Metadata:  map[string]any{"env": "dev"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if got := threadModelFromCore(thread).toCore(); got.Title != thread.Title || got.Status != thread.Status {
		t.Fatalf("thread round-trip mismatch: %+v", got)
	}
	if threadModelFromCore(nil) != nil || (*ThreadModel)(nil).toCore() != nil {
		t.Fatal("expected nil thread conversions")
	}

	threadMember := &core.ThreadMember{
		ID:             2,
		ThreadID:       1,
		Kind:           core.ThreadMemberKindAgent,
		UserID:         "user-1",
		AgentProfileID: "worker",
		Role:           "member",
		Status:         core.ThreadAgentActive,
		AgentData:      map[string]any{"turns": 3},
		JoinedAt:       now,
		LastActiveAt:   now,
	}
	if got := threadMemberModelFromCore(threadMember).toCore(); got.AgentProfileID != "worker" || got.Status != core.ThreadAgentActive {
		t.Fatalf("thread member round-trip mismatch: %+v", got)
	}

	threadMsg := (&ThreadMessageModel{
		ID:               3,
		ThreadID:         1,
		SenderID:         "user-1",
		Role:             "human",
		Content:          "hello",
		ReplyToMessageID: &replyTo,
		Metadata:         JSONField[map[string]any]{Data: map[string]any{"m": "v"}},
		CreatedAt:        now,
	}).toCore()
	if threadMsg == nil || threadMsg.ReplyToMessageID == nil || *threadMsg.ReplyToMessageID != replyTo {
		t.Fatalf("thread message conversion mismatch: %+v", threadMsg)
	}

	link := (&ThreadWorkItemLinkModel{
		ID:           4,
		ThreadID:     1,
		WorkItemID:   issueID,
		RelationType: "drives",
		IsPrimary:    true,
		CreatedAt:    now,
	}).toCore()
	if link == nil || !link.IsPrimary || link.RelationType != "drives" {
		t.Fatalf("thread link conversion mismatch: %+v", link)
	}

	threadRef := &core.ThreadContextRef{
		ID:        5,
		ThreadID:  1,
		ProjectID: projectID,
		Access:    core.ContextAccessWrite,
		Note:      "shared",
		GrantedBy: "owner",
		CreatedAt: now,
		ExpiresAt: &readAt,
	}
	if got := threadContextRefModelFromCore(threadRef).toCore(); got.Access != core.ContextAccessWrite || got.ExpiresAt == nil {
		t.Fatalf("thread context ref round-trip mismatch: %+v", got)
	}

	project := &core.Project{
		ID:          projectID,
		Name:        "proj",
		Kind:        core.ProjectDev,
		Description: "desc",
		Metadata:    map[string]string{"team": "core"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if got := projectModelFromCore(project).toCore(); got.Kind != core.ProjectDev || got.Metadata["team"] != "core" {
		t.Fatalf("project round-trip mismatch: %+v", got)
	}

	workItem := &core.WorkItem{
		ID:                issueID,
		ProjectID:         &projectID,
		ResourceSpaceID: &projectID,
		Title:             "work",
		Body:              "body",
		Status:            core.WorkItemRunning,
		Priority:          core.PriorityHigh,
		Labels:            []string{"api"},
		DependsOn:         []int64{1, 2},
		Metadata:          map[string]any{"p": "v"},
		ArchivedAt:        &readAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if got := workItemModelFromCore(workItem).toCore(); got.Priority != core.PriorityHigh || len(got.DependsOn) != 2 {
		t.Fatalf("work item round-trip mismatch: %+v", got)
	}

	action := &core.Action{
		ID:                   actionID,
		WorkItemID:           issueID,
		Name:                 "build",
		Description:          "desc",
		Type:                 core.ActionExec,
		Status:               core.ActionRunning,
		Position:             2,
		DependsOn:            []int64{1},
		Input:                "brief",
		AgentRole:            "worker",
		RequiredCapabilities: []string{"go"},
		AcceptanceCriteria:   []string{"pass"},
		Timeout:              5 * time.Second,
		MaxRetries:           2,
		RetryCount:           1,
		Config:               map[string]any{"retry": true},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if got := actionModelFromCore(action).toCore(); got.Timeout != 5*time.Second || got.MaxRetries != 2 {
		t.Fatalf("action round-trip mismatch: %+v", got)
	}

	run := &core.Run{
		ID:               runID,
		ActionID:         actionID,
		WorkItemID:       issueID,
		Status:           core.RunFailed,
		AgentID:          "agent-1",
		AgentContextID:   &actionID,
		BriefingSnapshot: "snapshot",
		Input:            map[string]any{"q": "1"},
		Output:           map[string]any{"a": "2"},
		ErrorMessage:     "boom",
		ErrorKind:        core.ErrKindTransient,
		Attempt:          3,
		StartedAt:        &now,
		FinishedAt:       &readAt,
		CreatedAt:        now,
		ResultMarkdown:   "done",
		ResultMetadata:   map[string]any{"k": "v"},
	}
	if got := runModelFromCore(run).toCore(); got.ErrorKind != core.ErrKindTransient || got.ResultMetadata["k"] != "v" {
		t.Fatalf("run round-trip mismatch: %+v", got)
	}

	agentContext := &core.AgentContext{
		ID:               7,
		AgentID:          "agent-1",
		WorkItemID:       issueID,
		SystemPrompt:     "prompt",
		SessionID:        "session",
		Summary:          "summary",
		TurnCount:        2,
		WorkerID:         "owner",
		WorkerLastSeenAt: &readAt,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if got := agentContextModelFromCore(agentContext).toCore(); got.WorkerID != "owner" || got.WorkerLastSeenAt == nil {
		t.Fatalf("agent context round-trip mismatch: %+v", got)
	}

	eventModel := eventModelFromCore(&core.Event{
		ID:         8,
		Type:       core.EventActionStarted,
		WorkItemID: issueID,
		ActionID:   actionID,
		RunID:      runID,
		Data:       map[string]any{"x": "y"},
		Timestamp:  now,
	})
	if eventModel.Category != core.EventCategoryDomain {
		t.Fatalf("expected default event category, got %q", eventModel.Category)
	}
	if got := eventModel.toCore(); got.RunID != runID || got.WorkItemID != issueID {
		t.Fatalf("event round-trip mismatch: %+v", got)
	}
	if eventModelFromCore(nil) != nil || (*EventModel)(nil).toCore() != nil {
		t.Fatal("expected nil event conversions")
	}

	agentProfile := &core.AgentProfile{
		ID:             "worker",
		Name:           "Worker",
		Driver:         core.DriverConfig{LaunchCommand: "codex"},
		Role:           core.RoleWorker,
		Capabilities:   []string{"go"},
		ActionsAllowed: []core.AgentAction{core.AgentActionTerminal},
		PromptTemplate: "tmpl",
		Skills:         []string{"skill-a"},
		Session:        core.ProfileSession{Reuse: true, MaxTurns: 10, IdleTTL: time.Minute},
		MCP:            core.ProfileMCP{Enabled: true, Tools: []string{"tool-a"}},
	}
	if got := agentProfileModelFromCore(agentProfile).toCore(); got.Session.IdleTTL != time.Minute || !got.MCP.Enabled {
		t.Fatalf("agent profile round-trip mismatch: %+v", got)
	}

	dag := &core.DAGTemplate{
		ID:          9,
		Name:        "template",
		Description: "desc",
		ProjectID:   &projectID,
		Tags:        []string{"go"},
		Metadata:    map[string]string{"team": "backend"},
		Actions: []core.DAGTemplateAction{{
			Name: "build",
			Type: string(core.ActionExec),
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if got := dagTemplateModelFromCore(dag).toCore(); len(got.Actions) != 1 || got.Metadata["team"] != "backend" {
		t.Fatalf("dag template round-trip mismatch: %+v", got)
	}

	usage := &core.UsageRecord{
		ID:               10,
		RunID:            runID,
		WorkItemID:       issueID,
		ActionID:         actionID,
		ProjectID:        &projectID,
		AgentID:          "agent-1",
		ProfileID:        "worker",
		ModelID:          "gpt",
		InputTokens:      10,
		OutputTokens:     20,
		CacheReadTokens:  3,
		CacheWriteTokens: 4,
		ReasoningTokens:  5,
		TotalTokens:      42,
		DurationMs:       600,
		CreatedAt:        now,
	}
	if got := usageRecordModelFromCore(usage).toCore(); got.TotalTokens != 42 || got.ProjectID == nil {
		t.Fatalf("usage round-trip mismatch: %+v", got)
	}

	feature := &core.FeatureEntry{
		ID:          11,
		ProjectID:   projectID,
		Key:         "feat.a",
		Description: "desc",
		Status:      core.FeaturePass,
		WorkItemID:  &issueID,
		ActionID:    &actionID,
		Tags:        []string{"api"},
		Metadata:    map[string]any{"scope": "backend"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if got := featureEntryModelFromCore(feature).toCore(); got.ActionID == nil || got.Metadata["scope"] != "backend" {
		t.Fatalf("feature entry round-trip mismatch: %+v", got)
	}

	journal := &core.JournalEntry{
		ID:             12,
		WorkItemID:     issueID,
		ActionID:       actionID,
		RunID:          runID,
		Kind:           core.JournalSignal,
		Source:         core.JournalSourceAgent,
		Summary:        "summary",
		Payload:        map[string]any{"a": 1},
		Ref:            refID,
		Actor:          "agent-1",
		SourceActionID: actionID,
		CreatedAt:      now,
	}
	if got := journalModelFromCore(journal).toCore(); got.Ref != refID || got.SourceActionID != actionID {
		t.Fatalf("journal round-trip mismatch: %+v", got)
	}

	actionSignal := &core.ActionSignal{
		ID:             13,
		ActionID:       actionID,
		WorkItemID:     issueID,
		RunID:          runID,
		Type:           core.SignalNeedHelp,
		Source:         core.SignalSourceAgent,
		Summary:        "need help",
		Content:        "blocked",
		SourceActionID: actionID,
		Payload:        map[string]any{"reason": "dep"},
		Actor:          "agent-1",
		CreatedAt:      now,
	}
	if got := actionSignalModelFromCore(actionSignal).toCore(); got.RunID != runID || got.Payload["reason"] != "dep" {
		t.Fatalf("action signal round-trip mismatch: %+v", got)
	}

	notification := &core.Notification{
		ID:         15,
		Level:      core.NotificationLevelWarning,
		Title:      "Heads up",
		Body:       "body",
		Category:   "system",
		ActionURL:  "/runs/1",
		ProjectID:  &projectID,
		WorkItemID: &issueID,
		RunID:      &runID,
		Channels:   []core.NotificationChannel{core.ChannelBrowser, core.ChannelInApp},
		Read:       true,
		ReadAt:     &readAt,
		CreatedAt:  now,
	}
	if got := notificationModelFromCore(notification).toCore(); len(got.Channels) != 2 || got.ReadAt == nil {
		t.Fatalf("notification round-trip mismatch: %+v", got)
	}
	if notificationModelFromCore(nil) != nil || (*NotificationModel)(nil).toCore() != nil {
		t.Fatal("expected nil notification conversions")
	}

	if int64PtrIfNonZero(0) != nil {
		t.Fatal("expected zero pointer helper to return nil")
	}
	if ptr := int64PtrIfNonZero(99); ptr == nil || *ptr != 99 {
		t.Fatalf("expected non-zero pointer helper to return 99, got %+v", ptr)
	}

	raw, err := json.Marshal(notificationModelFromCore(notification))
	if err != nil || len(raw) == 0 {
		t.Fatalf("expected notification model to remain JSON-marshalable, err=%v raw=%q", err, raw)
	}
}

func TestNotificationCRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	projectID := int64(101)
	issueID := int64(202)
	level := core.NotificationLevelWarning
	read := false

	first := &core.Notification{
		Level:      core.NotificationLevelInfo,
		Title:      "First",
		Body:       "body-1",
		Category:   "system",
		ActionURL:  "/threads/1",
		ProjectID:  &projectID,
		WorkItemID: &issueID,
		Channels:   []core.NotificationChannel{core.ChannelBrowser},
	}
	second := &core.Notification{
		Level:     level,
		Title:     "Second",
		Body:      "body-2",
		Category:  "system",
		ProjectID: &projectID,
		Channels:  []core.NotificationChannel{core.ChannelInApp},
	}

	firstID, err := store.CreateNotification(ctx, first)
	if err != nil {
		t.Fatalf("CreateNotification(first) error = %v", err)
	}
	secondID, err := store.CreateNotification(ctx, second)
	if err != nil {
		t.Fatalf("CreateNotification(second) error = %v", err)
	}

	got, err := store.GetNotification(ctx, firstID)
	if err != nil {
		t.Fatalf("GetNotification(first) error = %v", err)
	}
	if got.Title != "First" || len(got.Channels) != 1 || got.Channels[0] != core.ChannelBrowser {
		t.Fatalf("unexpected first notification: %+v", got)
	}

	list, err := store.ListNotifications(ctx, core.NotificationFilter{
		ProjectID: &projectID,
		Category:  "system",
		Level:     &level,
		Read:      &read,
	})
	if err != nil {
		t.Fatalf("ListNotifications error = %v", err)
	}
	if len(list) != 1 || list[0].ID != secondID {
		t.Fatalf("unexpected filtered list: %+v", list)
	}

	count, err := store.CountUnreadNotifications(ctx)
	if err != nil {
		t.Fatalf("CountUnreadNotifications error = %v", err)
	}
	if count != 2 {
		t.Fatalf("CountUnreadNotifications = %d, want 2", count)
	}

	if err := store.MarkNotificationRead(ctx, firstID); err != nil {
		t.Fatalf("MarkNotificationRead error = %v", err)
	}
	got, err = store.GetNotification(ctx, firstID)
	if err != nil {
		t.Fatalf("GetNotification(after mark read) error = %v", err)
	}
	if !got.Read || got.ReadAt == nil {
		t.Fatalf("expected notification to be marked read: %+v", got)
	}

	if err := store.MarkAllNotificationsRead(ctx); err != nil {
		t.Fatalf("MarkAllNotificationsRead error = %v", err)
	}
	count, err = store.CountUnreadNotifications(ctx)
	if err != nil {
		t.Fatalf("CountUnreadNotifications(after all read) error = %v", err)
	}
	if count != 0 {
		t.Fatalf("CountUnreadNotifications after mark-all = %d, want 0", count)
	}

	if err := store.DeleteNotification(ctx, secondID); err != nil {
		t.Fatalf("DeleteNotification error = %v", err)
	}
	if _, err := store.GetNotification(ctx, secondID); err != core.ErrNotFound {
		t.Fatalf("expected deleted notification to be missing, got %v", err)
	}
	if err := store.MarkNotificationRead(ctx, 99999); err != core.ErrNotFound {
		t.Fatalf("expected missing notification on mark-read, got %v", err)
	}
	if err := store.DeleteNotification(ctx, 99999); err != core.ErrNotFound {
		t.Fatalf("expected missing notification on delete, got %v", err)
	}
}
