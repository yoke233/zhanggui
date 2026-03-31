package orchestrateapp

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	"github.com/yoke233/zhanggui/internal/application/planning"
	"github.com/yoke233/zhanggui/internal/application/threadapp"
	"github.com/yoke233/zhanggui/internal/application/workitemapp"
	"github.com/yoke233/zhanggui/internal/core"
)

type testEnv struct {
	store *sqlite.Store
	svc   *Service
}

type fakePlanner struct {
	dag *planning.GeneratedDAG
	err error
}

func (p *fakePlanner) Generate(context.Context, planning.GenerateInput) (*planning.GeneratedDAG, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.dag, nil
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "orchestrateapp-test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("sqlite.New() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	seedOrchestrationProfiles(t, store)

	workItems := workitemapp.New(workitemapp.Config{Store: store, Registry: store})
	svc := New(Config{
		Store:           store,
		WorkItemCreator: workItems,
		Planner: &fakePlanner{dag: &planning.GeneratedDAG{
			Actions: []planning.GeneratedAction{
				{Name: "implement", Type: "exec", AgentRole: "lead"},
			},
		}},
		Threads:  threadapp.New(threadapp.Config{Store: store}),
		Registry: store,
	})
	return &testEnv{store: store, svc: svc}
}

func seedOrchestrationProfiles(t *testing.T, store *sqlite.Store) {
	t.Helper()

	ctx := context.Background()
	driver := core.DriverConfig{
		ID:            "codex",
		LaunchCommand: "codex",
		CapabilitiesMax: core.DriverCapabilities{
			FSRead:   true,
			FSWrite:  true,
			Terminal: true,
		},
	}
	profiles := []*core.AgentProfile{
		{ID: "ceo", Name: "CEO", Driver: driver, Role: core.RoleLead},
		{ID: "lead", Name: "Lead", ManagerProfileID: "ceo", Driver: driver, Role: core.RoleLead},
		{ID: "architect", Name: "Architect", ManagerProfileID: "ceo", Driver: driver, Role: core.RoleLead},
	}
	for _, profile := range profiles {
		if err := store.CreateProfile(ctx, profile); err != nil {
			t.Fatalf("CreateProfile(%s) error = %v", profile.ID, err)
		}
	}
}

func TestServiceCreateTaskReturnsExistingOpenWorkItemForSameDedupeKey(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)

	first, err := env.svc.CreateTask(context.Background(), CreateTaskInput{
		Title:     "CEO bootstrap",
		DedupeKey: "chat:42:goal:bootstrap",
	})
	if err != nil {
		t.Fatalf("CreateTask(first) error = %v", err)
	}

	second, err := env.svc.CreateTask(context.Background(), CreateTaskInput{
		Title:     "CEO bootstrap",
		DedupeKey: "chat:42:goal:bootstrap",
	})
	if err != nil {
		t.Fatalf("CreateTask(second) error = %v", err)
	}

	if second.WorkItem.ID != first.WorkItem.ID {
		t.Fatalf("CreateTask(second).WorkItem.ID = %d, want %d", second.WorkItem.ID, first.WorkItem.ID)
	}
	if second.Created {
		t.Fatal("CreateTask(second).Created = true, want false")
	}
}

func TestServiceCreateTaskReturnsExistingOpenWorkItemForSameSourceGoalRef(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)

	first, err := env.svc.CreateTask(context.Background(), CreateTaskInput{
		Title:         "CEO bootstrap",
		SourceGoalRef: "goal:bootstrap",
	})
	if err != nil {
		t.Fatalf("CreateTask(first) error = %v", err)
	}

	second, err := env.svc.CreateTask(context.Background(), CreateTaskInput{
		Title:         "CEO bootstrap duplicate",
		SourceGoalRef: "goal:bootstrap",
	})
	if err != nil {
		t.Fatalf("CreateTask(second) error = %v", err)
	}

	if second.WorkItem.ID != first.WorkItem.ID {
		t.Fatalf("CreateTask(second).WorkItem.ID = %d, want %d", second.WorkItem.ID, first.WorkItem.ID)
	}
	if second.Created {
		t.Fatal("CreateTask(second).Created = true, want false")
	}
}

func TestServiceCreateTaskSeedsExecutorReviewerAndSponsor(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)

	parentID := int64(7)
	rootID := int64(3)
	result, err := env.svc.CreateTask(context.Background(), CreateTaskInput{
		Title:            "Ship login",
		ParentWorkItemID: &parentID,
		RootWorkItemID:   &rootID,
		ExecutorProfile:  "lead",
		SourceGoalRef:    "goal:login",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if !result.Created {
		t.Fatal("CreateTask().Created = false, want true")
	}
	item := result.WorkItem
	if item.ExecutorProfileID != "lead" {
		t.Fatalf("ExecutorProfileID = %q, want lead", item.ExecutorProfileID)
	}
	if item.ActiveProfileID != "lead" {
		t.Fatalf("ActiveProfileID = %q, want lead", item.ActiveProfileID)
	}
	if item.ReviewerProfileID != "ceo" {
		t.Fatalf("ReviewerProfileID = %q, want ceo", item.ReviewerProfileID)
	}
	if item.SponsorProfileID != "ceo" {
		t.Fatalf("SponsorProfileID = %q, want ceo", item.SponsorProfileID)
	}
	if item.CreatedByProfileID != "ceo" {
		t.Fatalf("CreatedByProfileID = %q, want ceo", item.CreatedByProfileID)
	}
	if item.ParentWorkItemID == nil || *item.ParentWorkItemID != parentID {
		t.Fatalf("ParentWorkItemID = %v, want %d", item.ParentWorkItemID, parentID)
	}
	if item.RootWorkItemID == nil || *item.RootWorkItemID != rootID {
		t.Fatalf("RootWorkItemID = %v, want %d", item.RootWorkItemID, rootID)
	}
	if got := metadataValue(item.Metadata, "orchestrate", "source_goal_ref"); got != "goal:login" {
		t.Fatalf("metadata orchestrate source_goal_ref = %q, want goal:login", got)
	}
}

func TestServiceCreateTaskDedupeIgnoresLegacyCEONamespace(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	legacyID, err := env.store.CreateWorkItem(context.Background(), &core.WorkItem{
		Title:    "legacy task",
		Status:   core.WorkItemOpen,
		Priority: core.PriorityMedium,
		Metadata: map[string]any{
			"ceo": map[string]any{
				"dedupe_key": "chat:legacy",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	result, err := env.svc.CreateTask(context.Background(), CreateTaskInput{
		Title:     "new task",
		DedupeKey: "chat:legacy",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if !result.Created {
		t.Fatal("CreateTask().Created = false, want true")
	}
	if result.WorkItem.ID == legacyID {
		t.Fatalf("CreateTask().WorkItem.ID = %d, want new work item", result.WorkItem.ID)
	}
}

func TestServiceFollowUpTaskUsesActiveProfileAndFinalDeliverable(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	workItemID, err := env.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:             "assigned task",
		Status:            core.WorkItemOpen,
		Priority:          core.PriorityMedium,
		ExecutorProfileID: "lead",
		ActiveProfileID:   "lead",
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}
	actionID, err := env.store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "implement",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
	})
	if err != nil {
		t.Fatalf("CreateAction() error = %v", err)
	}
	_, err = env.store.CreateRun(ctx, &core.Run{
		ActionID:       actionID,
		WorkItemID:     workItemID,
		Status:         core.RunSucceeded,
		Attempt:        1,
		ResultMarkdown: "This run summary should be ignored in favor of the final deliverable",
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	deliverableID, err := env.store.CreateDeliverable(ctx, &core.Deliverable{
		WorkItemID:   &workItemID,
		Kind:         core.DeliverableDocument,
		Title:        "Design update",
		Summary:      "Deliverable summary should win",
		ProducerType: core.DeliverableProducerWorkItem,
		ProducerID:   workItemID,
		Status:       core.DeliverableFinal,
	})
	if err != nil {
		t.Fatalf("CreateDeliverable() error = %v", err)
	}
	workItem, err := env.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("GetWorkItem() error = %v", err)
	}
	workItem.FinalDeliverableID = &deliverableID
	if err := env.store.UpdateWorkItem(ctx, workItem); err != nil {
		t.Fatalf("UpdateWorkItem() error = %v", err)
	}

	result, err := env.svc.FollowUpTask(ctx, FollowUpTaskInput{WorkItemID: workItemID})
	if err != nil {
		t.Fatalf("FollowUpTask() error = %v", err)
	}
	if result.ActiveProfileID != "lead" {
		t.Fatalf("ActiveProfileID = %q, want lead", result.ActiveProfileID)
	}
	if result.RecommendedNextStep != "run_work_item" {
		t.Fatalf("RecommendedNextStep = %q, want run_work_item", result.RecommendedNextStep)
	}
	if result.LatestRunSummary != "Deliverable summary should win" {
		t.Fatalf("LatestRunSummary = %q, want deliverable summary", result.LatestRunSummary)
	}
}

func TestServiceReassignTaskDoesNotReadAssignedProfileFromLegacyMetadata(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	workItemID, err := env.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:             "reassign me",
		Status:            core.WorkItemOpen,
		Priority:          core.PriorityMedium,
		ExecutorProfileID: "lead",
		ActiveProfileID:   "lead",
		ReviewerProfileID: "ceo",
		SponsorProfileID:  "ceo",
		Metadata: map[string]any{
			"ceo": map[string]any{"assigned_profile": "worker"},
		},
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	result, err := env.svc.ReassignTask(ctx, ReassignTaskInput{
		WorkItemID:    workItemID,
		NewProfile:    "architect",
		Reason:        "需要改派给架构 owner",
		ActorProfile:  "ceo",
		SourceSession: "chat-42",
	})
	if err != nil {
		t.Fatalf("ReassignTask() error = %v", err)
	}
	if result.OldProfile != "lead" || result.NewProfile != "architect" {
		t.Fatalf("unexpected reassign result: %+v", result)
	}

	workItem, err := env.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("GetWorkItem() error = %v", err)
	}
	if workItem.ExecutorProfileID != "architect" {
		t.Fatalf("ExecutorProfileID = %q, want architect", workItem.ExecutorProfileID)
	}
	if workItem.ActiveProfileID != "architect" {
		t.Fatalf("ActiveProfileID = %q, want architect", workItem.ActiveProfileID)
	}
	if workItem.ReviewerProfileID != "ceo" {
		t.Fatalf("ReviewerProfileID = %q, want ceo", workItem.ReviewerProfileID)
	}
	if workItem.SponsorProfileID != "ceo" {
		t.Fatalf("SponsorProfileID = %q, want ceo", workItem.SponsorProfileID)
	}
	wantPath := []string{"ceo", workitemapp.HumanEscalationTarget}
	if !reflect.DeepEqual(workItem.EscalationPath, wantPath) {
		t.Fatalf("EscalationPath = %#v, want %#v", workItem.EscalationPath, wantPath)
	}
	entries, err := env.store.ListJournal(ctx, core.JournalFilter{
		WorkItemID: &workItemID,
		Kinds:      []core.JournalKind{core.JournalAssignment},
	})
	if err != nil {
		t.Fatalf("ListJournal() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("assignment journal len = %d, want 1", len(entries))
	}
	if got := entries[0].Payload["from_profile_id"]; got != "lead" {
		t.Fatalf("from_profile_id = %v, want lead", got)
	}
	if got := entries[0].Payload["to_profile_id"]; got != "architect" {
		t.Fatalf("to_profile_id = %v, want architect", got)
	}
}

func TestServiceReassignPropagatesPreferredProfileToPendingExecutableActions(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	workItemID, err := env.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:             "propagate assignee",
		Status:            core.WorkItemOpen,
		Priority:          core.PriorityMedium,
		ExecutorProfileID: "lead",
		ActiveProfileID:   "lead",
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}
	pendingExecID, err := env.store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "pending-exec",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
	})
	if err != nil {
		t.Fatalf("CreateAction(pending exec) error = %v", err)
	}
	readyCompositeID, err := env.store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "ready-composite",
		Type:       core.ActionComposite,
		Status:     core.ActionReady,
		Position:   1,
	})
	if err != nil {
		t.Fatalf("CreateAction(ready composite) error = %v", err)
	}
	runningExecID, err := env.store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "running-exec",
		Type:       core.ActionExec,
		Status:     core.ActionRunning,
		Position:   2,
		Config:     map[string]any{"preferred_profile_id": "worker"},
	})
	if err != nil {
		t.Fatalf("CreateAction(running exec) error = %v", err)
	}
	gateID, err := env.store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "gate",
		Type:       core.ActionGate,
		Status:     core.ActionPending,
		Position:   3,
	})
	if err != nil {
		t.Fatalf("CreateAction(gate) error = %v", err)
	}

	_, err = env.svc.ReassignTask(ctx, ReassignTaskInput{
		WorkItemID:    workItemID,
		NewProfile:    "architect",
		Reason:        "改派给更合适的执行角色",
		ActorProfile:  "ceo",
		SourceSession: "chat-77",
	})
	if err != nil {
		t.Fatalf("ReassignTask() error = %v", err)
	}

	pendingExec, err := env.store.GetAction(ctx, pendingExecID)
	if err != nil {
		t.Fatalf("GetAction(pending exec) error = %v", err)
	}
	if pendingExec.Config["preferred_profile_id"] != "architect" {
		t.Fatalf("pending exec preferred_profile_id = %v, want architect", pendingExec.Config["preferred_profile_id"])
	}

	readyComposite, err := env.store.GetAction(ctx, readyCompositeID)
	if err != nil {
		t.Fatalf("GetAction(ready composite) error = %v", err)
	}
	if readyComposite.Config["preferred_profile_id"] != "architect" {
		t.Fatalf("ready composite preferred_profile_id = %v, want architect", readyComposite.Config["preferred_profile_id"])
	}

	runningExec, err := env.store.GetAction(ctx, runningExecID)
	if err != nil {
		t.Fatalf("GetAction(running exec) error = %v", err)
	}
	if runningExec.Config["preferred_profile_id"] != "worker" {
		t.Fatalf("running exec preferred_profile_id = %v, want worker", runningExec.Config["preferred_profile_id"])
	}

	gate, err := env.store.GetAction(ctx, gateID)
	if err != nil {
		t.Fatalf("GetAction(gate) error = %v", err)
	}
	if gate.Config != nil {
		if _, exists := gate.Config["preferred_profile_id"]; exists {
			t.Fatalf("gate preferred_profile_id = %v, want unset", gate.Config["preferred_profile_id"])
		}
	}
}

func TestServiceReassignRejectsMissingProfile(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	workItemID, err := env.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "missing profile",
		Status:   core.WorkItemOpen,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	_, err = env.svc.ReassignTask(ctx, ReassignTaskInput{
		WorkItemID: workItemID,
		NewProfile: "   ",
	})
	if CodeOf(err) != CodeMissingProfile {
		t.Fatalf("CodeOf(err) = %q, want %q (err=%v)", CodeOf(err), CodeMissingProfile, err)
	}
}

func TestServiceDecomposeRejectsOverwriteWhenActiveActionsExist(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	workItemID, err := env.store.CreateWorkItem(context.Background(), &core.WorkItem{
		Title:    "replan me",
		Status:   core.WorkItemOpen,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}
	_, err = env.store.CreateAction(context.Background(), &core.Action{
		WorkItemID: workItemID,
		Name:       "running-action",
		Type:       core.ActionExec,
		Status:     core.ActionRunning,
		Position:   0,
	})
	if err != nil {
		t.Fatalf("CreateAction() error = %v", err)
	}

	_, err = env.svc.DecomposeTask(context.Background(), DecomposeTaskInput{
		WorkItemID:        workItemID,
		Objective:         "replan",
		OverwriteExisting: true,
	})
	if CodeOf(err) != CodeDecomposeConflict {
		t.Fatalf("CodeOf(err) = %q, want %q (err=%v)", CodeOf(err), CodeDecomposeConflict, err)
	}
}

func TestServiceDecomposePropagatesActiveProfileToCreatedActions(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	workItemID, err := env.store.CreateWorkItem(context.Background(), &core.WorkItem{
		Title:             "assigned decompose",
		Status:            core.WorkItemOpen,
		Priority:          core.PriorityMedium,
		ExecutorProfileID: "lead",
		ActiveProfileID:   "lead",
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	result, err := env.svc.DecomposeTask(context.Background(), DecomposeTaskInput{
		WorkItemID: workItemID,
		Objective:  "build implementation plan",
	})
	if err != nil {
		t.Fatalf("DecomposeTask() error = %v", err)
	}
	if result.ActionCount != 1 {
		t.Fatalf("ActionCount = %d, want 1", result.ActionCount)
	}
	actions, err := env.store.ListActionsByWorkItem(context.Background(), workItemID)
	if err != nil {
		t.Fatalf("ListActionsByWorkItem() error = %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions len = %d, want 1", len(actions))
	}
	if actions[0].Config["preferred_profile_id"] != "lead" {
		t.Fatalf("preferred_profile_id = %v, want lead", actions[0].Config["preferred_profile_id"])
	}
}

func TestServiceEscalateThreadReturnsExistingActiveThreadLink(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	workItemID, err := env.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "coordination task",
		Status:   core.WorkItemBlocked,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	threadResult, err := threadapp.New(threadapp.Config{Store: env.store}).CreateThread(ctx, threadapp.CreateThreadInput{
		Title:   "CEO escalation",
		OwnerID: "ceo",
	})
	if err != nil {
		t.Fatalf("CreateThread() error = %v", err)
	}
	if _, err := env.store.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{
		ThreadID:     threadResult.Thread.ID,
		WorkItemID:   workItemID,
		RelationType: "drives",
		IsPrimary:    true,
	}); err != nil {
		t.Fatalf("CreateThreadWorkItemLink() error = %v", err)
	}

	result, err := env.svc.EscalateThread(ctx, EscalateThreadInput{
		WorkItemID:    workItemID,
		Reason:        "needs coordination",
		ThreadTitle:   "CEO escalation",
		ActorProfile:  "ceo",
		SourceSession: "chat-1",
	})
	if err != nil {
		t.Fatalf("EscalateThread() error = %v", err)
	}
	if result.Thread == nil || result.Thread.ID != threadResult.Thread.ID {
		t.Fatalf("thread id = %+v, want %d", result.Thread, threadResult.Thread.ID)
	}
	if result.Created {
		t.Fatal("EscalateThread().Created = true, want false")
	}
}

func TestServiceEscalateThreadDoesNotAppendCEOJournal(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	workItemID, err := env.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:             "blocked task",
		Status:            core.WorkItemBlocked,
		Priority:          core.PriorityMedium,
		ExecutorProfileID: "lead",
		ActiveProfileID:   "lead",
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	result, err := env.svc.EscalateThread(ctx, EscalateThreadInput{
		WorkItemID:     workItemID,
		Reason:         "stuck on integration",
		ThreadTitle:    "CEO escalation",
		ActorProfile:   "ceo",
		SourceSession:  "chat-2",
		InviteProfiles: []string{"architect"},
		InviteHumans:   []string{"alice"},
	})
	if err != nil {
		t.Fatalf("EscalateThread() error = %v", err)
	}
	if result.Thread == nil || result.Thread.ID == 0 {
		t.Fatalf("expected created thread, got %+v", result.Thread)
	}
	if !result.Created {
		t.Fatal("EscalateThread().Created = false, want true")
	}

	workItem, err := env.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("GetWorkItem() error = %v", err)
	}
	if _, exists := workItem.Metadata["ceo_journal"]; exists {
		t.Fatalf("ceo_journal should be absent, got %#v", workItem.Metadata["ceo_journal"])
	}
	entries, err := env.store.ListJournal(ctx, core.JournalFilter{
		WorkItemID: &workItemID,
		Kinds:      []core.JournalKind{core.JournalSystem},
	})
	if err != nil {
		t.Fatalf("ListJournal() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("system journal len = %d, want 1", len(entries))
	}
	var gotThreadID int64
	switch value := entries[0].Payload["thread_id"].(type) {
	case int64:
		gotThreadID = value
	case int:
		gotThreadID = int64(value)
	case float64:
		gotThreadID = int64(value)
	default:
		t.Fatalf("thread_id payload type = %T, want numeric", entries[0].Payload["thread_id"])
	}
	if gotThreadID != result.Thread.ID {
		t.Fatalf("thread_id payload = %d, want %d", gotThreadID, result.Thread.ID)
	}
}

func TestServiceEscalateThreadInvitesProfilesIntoThreadMembers(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	workItemID, err := env.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "blocked task with profiles",
		Status:   core.WorkItemBlocked,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	result, err := env.svc.EscalateThread(ctx, EscalateThreadInput{
		WorkItemID:     workItemID,
		Reason:         "need lead review in thread",
		ThreadTitle:    "CEO escalation",
		ActorProfile:   "ceo",
		SourceSession:  "chat-4",
		InviteProfiles: []string{"lead"},
	})
	if err != nil {
		t.Fatalf("EscalateThread() error = %v", err)
	}

	members, err := env.store.ListThreadMembers(ctx, result.Thread.ID)
	if err != nil {
		t.Fatalf("ListThreadMembers() error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members len = %d, want 2", len(members))
	}
	if members[1].Kind != core.ThreadMemberKindAgent || members[1].AgentProfileID != "lead" {
		t.Fatalf("unexpected invited agent member: %+v", members[1])
	}
}

func TestServiceEscalateThreadTreatsInviteHumansAsMeetingParticipantsOnly(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	workItemID, err := env.store.CreateWorkItem(ctx, &core.WorkItem{
		Title:             "meeting task",
		Status:            core.WorkItemBlocked,
		Priority:          core.PriorityMedium,
		ExecutorProfileID: "lead",
		ActiveProfileID:   "lead",
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	result, err := env.svc.EscalateThread(ctx, EscalateThreadInput{
		WorkItemID:    workItemID,
		Reason:        "need product sync",
		ThreadTitle:   "coordination room",
		ActorProfile:  "ceo",
		InviteHumans:  []string{"alice", "bob"},
		SourceSession: "chat-3",
	})
	if err != nil {
		t.Fatalf("EscalateThread() error = %v", err)
	}

	members, err := env.store.ListThreadMembers(ctx, result.Thread.ID)
	if err != nil {
		t.Fatalf("ListThreadMembers() error = %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("members len = %d, want 3", len(members))
	}

	workItem, err := env.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("GetWorkItem() error = %v", err)
	}
	if workItem.ActiveProfileID != "lead" {
		t.Fatalf("ActiveProfileID = %q, want lead", workItem.ActiveProfileID)
	}
	if _, exists := workItem.Metadata["ceo_journal"]; exists {
		t.Fatalf("ceo_journal should be absent, got %#v", workItem.Metadata["ceo_journal"])
	}
}
