package executor

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestStoreBuiltinArtifact(t *testing.T) {
	step := &core.Action{ID: 2, WorkItemID: 1}
	execRec := &core.Run{ID: 3}
	bus := NewMemBus()
	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 4})
	defer sub.Cancel()

	if err := storeBuiltinArtifact(context.Background(), nil, nil, step, execRec, "done", nil); err == nil {
		t.Fatal("expected nil store to fail")
	}
	if err := storeBuiltinArtifact(context.Background(), &noopStore{}, nil, nil, execRec, "done", nil); err == nil {
		t.Fatal("expected nil step to fail")
	}

	err := storeBuiltinArtifact(context.Background(), &noopStore{}, bus, step, execRec, "  done  ", map[string]any{"source": "builtin"})
	if err != nil {
		t.Fatalf("storeBuiltinArtifact() error = %v", err)
	}
	if execRec.ResultMarkdown != "done" {
		t.Fatalf("ResultMarkdown = %q, want trimmed value", execRec.ResultMarkdown)
	}
	if execRec.ResultMetadata["source"] != "builtin" {
		t.Fatalf("ResultMetadata = %+v", execRec.ResultMetadata)
	}
	if execRec.Output["stop_reason"] != "builtin" {
		t.Fatalf("Output = %+v", execRec.Output)
	}

	select {
	case ev := <-sub.C:
		if ev.Type != core.EventRunAgentOutput || ev.Data["type"] != "done" || ev.Data["content"] != "done" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	default:
		t.Fatal("expected done event to be published")
	}
}

func TestWriteAskPassCmd(t *testing.T) {
	path, cleanup, err := writeAskPassCmd("secret-token")
	if err != nil {
		t.Fatalf("writeAskPassCmd() error = %v", err)
	}
	defer cleanup()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	content := string(raw)
	if !strings.Contains(content, "secret-token") || !strings.Contains(content, "x-access-token") {
		t.Fatalf("unexpected askpass content: %q", content)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove file, got err=%v", err)
	}
}

func TestAsJSONAndIsAuthErrorAndMinLen(t *testing.T) {
	if got := asJSON(map[string]any{"ok": true}); got != "{\"ok\":true}" {
		t.Fatalf("asJSON() = %q", got)
	}

	tests := []struct {
		err  error
		want bool
	}{
		{err: nil, want: false},
		{err: assertErr("401 Bad credentials"), want: true},
		{err: assertErr("authentication failed"), want: true},
		{err: assertErr("403 forbidden"), want: true},
		{err: assertErr("network timeout"), want: false},
	}
	for _, tt := range tests {
		if got := isAuthError(tt.err); got != tt.want {
			t.Fatalf("isAuthError(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}

	if got := minLen("abcdef", 3); got != 3 {
		t.Fatalf("minLen(longer) = %d, want 3", got)
	}
	if got := minLen("ab", 3); got != 2 {
		t.Fatalf("minLen(shorter) = %d, want 2", got)
	}
}

type noopStore struct{}

func (n *noopStore) CreateProject(context.Context, *core.Project) (int64, error)     { panic("unused") }
func (n *noopStore) GetProject(context.Context, int64) (*core.Project, error)        { panic("unused") }
func (n *noopStore) ListProjects(context.Context, int, int) ([]*core.Project, error) { panic("unused") }
func (n *noopStore) UpdateProject(context.Context, *core.Project) error              { panic("unused") }
func (n *noopStore) DeleteProject(context.Context, int64) error                      { panic("unused") }
func (n *noopStore) CreateResourceSpace(context.Context, *core.ResourceSpace) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetResourceSpace(context.Context, int64) (*core.ResourceSpace, error) {
	panic("unused")
}
func (n *noopStore) ListResourceSpaces(context.Context, int64) ([]*core.ResourceSpace, error) {
	panic("unused")
}
func (n *noopStore) UpdateResourceSpace(context.Context, *core.ResourceSpace) error { panic("unused") }
func (n *noopStore) DeleteResourceSpace(context.Context, int64) error               { panic("unused") }
func (n *noopStore) CreateResource(context.Context, *core.Resource) (int64, error)  { panic("unused") }
func (n *noopStore) GetResource(context.Context, int64) (*core.Resource, error)     { panic("unused") }
func (n *noopStore) ListResourcesByWorkItem(context.Context, int64) ([]*core.Resource, error) {
	panic("unused")
}
func (n *noopStore) ListResourcesByRun(context.Context, int64) ([]*core.Resource, error) {
	panic("unused")
}
func (n *noopStore) ListResourcesByMessage(context.Context, int64) ([]*core.Resource, error) {
	panic("unused")
}
func (n *noopStore) DeleteResource(context.Context, int64) error { panic("unused") }
func (n *noopStore) CreateActionIODecl(context.Context, *core.ActionIODecl) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetActionIODecl(context.Context, int64) (*core.ActionIODecl, error) {
	panic("unused")
}
func (n *noopStore) ListActionIODecls(context.Context, int64) ([]*core.ActionIODecl, error) {
	panic("unused")
}
func (n *noopStore) ListActionIODeclsByDirection(context.Context, int64, core.IODirection) ([]*core.ActionIODecl, error) {
	panic("unused")
}
func (n *noopStore) DeleteActionIODecl(context.Context, int64) error { panic("unused") }
func (n *noopStore) CreateResourceBinding(context.Context, *core.ResourceBinding) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetResourceBinding(context.Context, int64) (*core.ResourceBinding, error) {
	panic("unused")
}
func (n *noopStore) ListResourceBindings(context.Context, int64) ([]*core.ResourceBinding, error) {
	panic("unused")
}
func (n *noopStore) ListResourceBindingsByIssue(context.Context, int64, string) ([]*core.ResourceBinding, error) {
	panic("unused")
}
func (n *noopStore) UpdateResourceBinding(context.Context, *core.ResourceBinding) error {
	panic("unused")
}
func (n *noopStore) DeleteResourceBinding(context.Context, int64) error { panic("unused") }
func (n *noopStore) CreateActionResource(context.Context, *core.ActionResource) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetActionResource(context.Context, int64) (*core.ActionResource, error) {
	panic("unused")
}
func (n *noopStore) ListActionResources(context.Context, int64) ([]*core.ActionResource, error) {
	panic("unused")
}
func (n *noopStore) ListActionResourcesByDirection(context.Context, int64, core.ActionResourceDirection) ([]*core.ActionResource, error) {
	panic("unused")
}
func (n *noopStore) DeleteActionResource(context.Context, int64) error             { panic("unused") }
func (n *noopStore) CreateWorkItem(context.Context, *core.WorkItem) (int64, error) { panic("unused") }
func (n *noopStore) GetWorkItem(context.Context, int64) (*core.WorkItem, error)    { panic("unused") }
func (n *noopStore) ListWorkItems(context.Context, core.WorkItemFilter) ([]*core.WorkItem, error) {
	panic("unused")
}
func (n *noopStore) UpdateWorkItem(context.Context, *core.WorkItem) error { panic("unused") }
func (n *noopStore) UpdateWorkItemStatus(context.Context, int64, core.WorkItemStatus) error {
	panic("unused")
}
func (n *noopStore) UpdateWorkItemMetadata(context.Context, int64, map[string]any) error {
	panic("unused")
}
func (n *noopStore) PrepareWorkItemRun(context.Context, int64, core.WorkItemStatus) error {
	panic("unused")
}
func (n *noopStore) SetWorkItemArchived(context.Context, int64, bool) error { panic("unused") }
func (n *noopStore) DeleteWorkItem(context.Context, int64) error            { panic("unused") }
func (n *noopStore) CreateThread(context.Context, *core.Thread) (int64, error) { panic("unused") }
func (n *noopStore) GetThread(context.Context, int64) (*core.Thread, error)    { panic("unused") }
func (n *noopStore) ListThreads(context.Context, core.ThreadFilter) ([]*core.Thread, error) {
	panic("unused")
}
func (n *noopStore) UpdateThread(context.Context, *core.Thread) error { panic("unused") }
func (n *noopStore) DeleteThread(context.Context, int64) error        { panic("unused") }
func (n *noopStore) CreateThreadMessage(context.Context, *core.ThreadMessage) (int64, error) {
	panic("unused")
}
func (n *noopStore) ListThreadMessages(context.Context, int64, int, int) ([]*core.ThreadMessage, error) {
	panic("unused")
}
func (n *noopStore) DeleteThreadMessagesByThread(context.Context, int64) error { panic("unused") }
func (n *noopStore) AddThreadMember(context.Context, *core.ThreadMember) (int64, error) {
	panic("unused")
}
func (n *noopStore) ListThreadMembers(context.Context, int64) ([]*core.ThreadMember, error) {
	panic("unused")
}
func (n *noopStore) GetThreadMember(context.Context, int64) (*core.ThreadMember, error) {
	panic("unused")
}
func (n *noopStore) UpdateThreadMember(context.Context, *core.ThreadMember) error  { panic("unused") }
func (n *noopStore) RemoveThreadMember(context.Context, int64) error               { panic("unused") }
func (n *noopStore) RemoveThreadMemberByUser(context.Context, int64, string) error { panic("unused") }
func (n *noopStore) DeleteThreadMembersByThread(context.Context, int64) error      { panic("unused") }
func (n *noopStore) CreateThreadWorkItemLink(context.Context, *core.ThreadWorkItemLink) (int64, error) {
	panic("unused")
}
func (n *noopStore) ListWorkItemsByThread(context.Context, int64) ([]*core.ThreadWorkItemLink, error) {
	panic("unused")
}
func (n *noopStore) ListThreadsByWorkItem(context.Context, int64) ([]*core.ThreadWorkItemLink, error) {
	panic("unused")
}
func (n *noopStore) DeleteThreadWorkItemLink(context.Context, int64, int64) error   { panic("unused") }
func (n *noopStore) DeleteThreadWorkItemLinksByThread(context.Context, int64) error { panic("unused") }
func (n *noopStore) DeleteThreadWorkItemLinksByWorkItem(context.Context, int64) error {
	panic("unused")
}
func (n *noopStore) CreateThreadContextRef(context.Context, *core.ThreadContextRef) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetThreadContextRef(context.Context, int64) (*core.ThreadContextRef, error) {
	panic("unused")
}
func (n *noopStore) ListThreadContextRefs(context.Context, int64) ([]*core.ThreadContextRef, error) {
	panic("unused")
}
func (n *noopStore) UpdateThreadContextRef(context.Context, *core.ThreadContextRef) error {
	panic("unused")
}
func (n *noopStore) DeleteThreadContextRef(context.Context, int64) error          { panic("unused") }
func (n *noopStore) DeleteThreadContextRefsByThread(context.Context, int64) error { panic("unused") }
func (n *noopStore) CreateThreadAttachment(context.Context, *core.ThreadAttachment) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetThreadAttachment(context.Context, int64) (*core.ThreadAttachment, error) {
	panic("unused")
}
func (n *noopStore) ListThreadAttachments(context.Context, int64) ([]*core.ThreadAttachment, error) {
	panic("unused")
}
func (n *noopStore) DeleteThreadAttachment(context.Context, int64) error          { panic("unused") }
func (n *noopStore) DeleteThreadAttachmentsByThread(context.Context, int64) error { panic("unused") }
func (n *noopStore) CreateAction(context.Context, *core.Action) (int64, error)    { panic("unused") }
func (n *noopStore) GetAction(context.Context, int64) (*core.Action, error)       { panic("unused") }
func (n *noopStore) ListActionsByWorkItem(context.Context, int64) ([]*core.Action, error) {
	panic("unused")
}
func (n *noopStore) UpdateActionStatus(context.Context, int64, core.ActionStatus) error {
	panic("unused")
}
func (n *noopStore) UpdateAction(context.Context, *core.Action) error             { panic("unused") }
func (n *noopStore) DeleteAction(context.Context, int64) error                    { panic("unused") }
func (n *noopStore) BatchCreateActions(context.Context, []*core.Action) error     { panic("unused") }
func (n *noopStore) UpdateActionDependsOn(context.Context, int64, []int64) error  { panic("unused") }
func (n *noopStore) CreateRun(context.Context, *core.Run) (int64, error)          { panic("unused") }
func (n *noopStore) GetRun(context.Context, int64) (*core.Run, error)             { panic("unused") }
func (n *noopStore) ListRunsByAction(context.Context, int64) ([]*core.Run, error) { panic("unused") }
func (n *noopStore) ListRunsByStatus(context.Context, core.RunStatus) ([]*core.Run, error) {
	panic("unused")
}
func (n *noopStore) UpdateRun(context.Context, *core.Run) error { panic("unused") }
func (n *noopStore) GetLatestRunWithResult(context.Context, int64) (*core.Run, error) {
	panic("unused")
}
func (n *noopStore) CreateAgentContext(context.Context, *core.AgentContext) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetAgentContext(context.Context, int64) (*core.AgentContext, error) {
	panic("unused")
}
func (n *noopStore) FindAgentContext(context.Context, string, int64) (*core.AgentContext, error) {
	panic("unused")
}
func (n *noopStore) UpdateAgentContext(context.Context, *core.AgentContext) error { panic("unused") }
func (n *noopStore) CreateEvent(context.Context, *core.Event) (int64, error)      { panic("unused") }
func (n *noopStore) ListEvents(context.Context, core.EventFilter) ([]*core.Event, error) {
	panic("unused")
}
func (n *noopStore) GetLatestRunEventTime(context.Context, int64, core.EventType) (*time.Time, error) {
	panic("unused")
}
func (n *noopStore) CreateToolCallAudit(context.Context, *core.ToolCallAudit) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetToolCallAudit(context.Context, int64) (*core.ToolCallAudit, error) {
	panic("unused")
}
func (n *noopStore) GetToolCallAuditByToolCallID(context.Context, int64, string) (*core.ToolCallAudit, error) {
	panic("unused")
}
func (n *noopStore) ListToolCallAuditsByRun(context.Context, int64) ([]*core.ToolCallAudit, error) {
	panic("unused")
}
func (n *noopStore) UpdateToolCallAudit(context.Context, *core.ToolCallAudit) error { panic("unused") }
func (n *noopStore) ProjectErrorRanking(context.Context, core.AnalyticsFilter) ([]core.ProjectErrorRank, error) {
	panic("unused")
}
func (n *noopStore) WorkItemBottleneckActions(context.Context, core.AnalyticsFilter) ([]core.ActionBottleneck, error) {
	panic("unused")
}
func (n *noopStore) RunDurationStats(context.Context, core.AnalyticsFilter) ([]core.WorkItemDurationStat, error) {
	panic("unused")
}
func (n *noopStore) ErrorBreakdown(context.Context, core.AnalyticsFilter) ([]core.ErrorKindCount, error) {
	panic("unused")
}
func (n *noopStore) RecentFailures(context.Context, core.AnalyticsFilter) ([]core.FailureRecord, error) {
	panic("unused")
}
func (n *noopStore) WorkItemStatusDistribution(context.Context, core.AnalyticsFilter) ([]core.StatusCount, error) {
	panic("unused")
}
func (n *noopStore) CreateDAGTemplate(context.Context, *core.DAGTemplate) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetDAGTemplate(context.Context, int64) (*core.DAGTemplate, error) {
	panic("unused")
}
func (n *noopStore) ListDAGTemplates(context.Context, core.DAGTemplateFilter) ([]*core.DAGTemplate, error) {
	panic("unused")
}
func (n *noopStore) UpdateDAGTemplate(context.Context, *core.DAGTemplate) error { panic("unused") }
func (n *noopStore) DeleteDAGTemplate(context.Context, int64) error             { panic("unused") }
func (n *noopStore) CreateUsageRecord(context.Context, *core.UsageRecord) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetUsageRecord(context.Context, int64) (*core.UsageRecord, error) {
	panic("unused")
}
func (n *noopStore) GetUsageByRun(context.Context, int64) (*core.UsageRecord, error) { panic("unused") }
func (n *noopStore) UsageByProject(context.Context, core.AnalyticsFilter) ([]core.ProjectUsageSummary, error) {
	panic("unused")
}
func (n *noopStore) UsageByAgent(context.Context, core.AnalyticsFilter) ([]core.AgentUsageSummary, error) {
	panic("unused")
}
func (n *noopStore) UsageByProfile(context.Context, core.AnalyticsFilter) ([]core.ProfileUsageSummary, error) {
	panic("unused")
}
func (n *noopStore) UsageTotals(context.Context, core.AnalyticsFilter) (*core.UsageTotalSummary, error) {
	panic("unused")
}
func (n *noopStore) CreateFeatureEntry(context.Context, *core.FeatureEntry) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetFeatureEntry(context.Context, int64) (*core.FeatureEntry, error) {
	panic("unused")
}
func (n *noopStore) GetFeatureEntryByKey(context.Context, int64, string) (*core.FeatureEntry, error) {
	panic("unused")
}
func (n *noopStore) ListFeatureEntries(context.Context, core.FeatureEntryFilter) ([]*core.FeatureEntry, error) {
	panic("unused")
}
func (n *noopStore) UpdateFeatureEntry(context.Context, *core.FeatureEntry) error { panic("unused") }
func (n *noopStore) UpdateFeatureEntryStatus(context.Context, int64, core.FeatureStatus) error {
	panic("unused")
}
func (n *noopStore) DeleteFeatureEntry(context.Context, int64) error { panic("unused") }
func (n *noopStore) CountFeatureEntriesByStatus(context.Context, int64) (map[core.FeatureStatus]int, error) {
	panic("unused")
}
func (n *noopStore) CreateActionSignal(context.Context, *core.ActionSignal) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetLatestActionSignal(context.Context, int64, ...core.SignalType) (*core.ActionSignal, error) {
	panic("unused")
}
func (n *noopStore) ListActionSignals(context.Context, int64) ([]*core.ActionSignal, error) {
	panic("unused")
}
func (n *noopStore) ListActionSignalsByType(context.Context, int64, ...core.SignalType) ([]*core.ActionSignal, error) {
	panic("unused")
}
func (n *noopStore) CountActionSignals(context.Context, int64, ...core.SignalType) (int, error) {
	panic("unused")
}
func (n *noopStore) ListPendingHumanActions(context.Context, int64) ([]*core.Action, error) {
	panic("unused")
}
func (n *noopStore) ListAllPendingHumanActions(context.Context) ([]*core.Action, error) {
	panic("unused")
}
func (n *noopStore) ListProbeSignalsByRun(context.Context, int64) ([]*core.ActionSignal, error) {
	panic("unused")
}
func (n *noopStore) GetLatestProbeSignal(context.Context, int64) (*core.ActionSignal, error) {
	panic("unused")
}
func (n *noopStore) GetActiveProbeSignal(context.Context, int64) (*core.ActionSignal, error) {
	panic("unused")
}
func (n *noopStore) UpdateProbeSignal(context.Context, *core.ActionSignal) error { panic("unused") }
func (n *noopStore) GetRunProbeRoute(context.Context, int64) (*core.RunProbeRoute, error) {
	panic("unused")
}
func (n *noopStore) AppendJournal(context.Context, *core.JournalEntry) (int64, error) {
	panic("unused")
}
func (n *noopStore) BatchAppendJournal(context.Context, []*core.JournalEntry) error { panic("unused") }
func (n *noopStore) ListJournal(context.Context, core.JournalFilter) ([]*core.JournalEntry, error) {
	panic("unused")
}
func (n *noopStore) CountJournal(context.Context, core.JournalFilter) (int, error) { panic("unused") }
func (n *noopStore) GetLatestSignal(context.Context, int64, ...string) (*core.JournalEntry, error) {
	panic("unused")
}
func (n *noopStore) CountSignals(context.Context, int64, ...string) (int, error) { panic("unused") }
func (n *noopStore) CreateNotification(context.Context, *core.Notification) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetNotification(context.Context, int64) (*core.Notification, error) {
	panic("unused")
}
func (n *noopStore) ListNotifications(context.Context, core.NotificationFilter) ([]*core.Notification, error) {
	panic("unused")
}
func (n *noopStore) MarkNotificationRead(context.Context, int64) error { panic("unused") }
func (n *noopStore) MarkAllNotificationsRead(context.Context) error    { panic("unused") }
func (n *noopStore) DeleteNotification(context.Context, int64) error   { panic("unused") }
func (n *noopStore) CountUnreadNotifications(context.Context) (int, error) {
	panic("unused")
}
func (n *noopStore) CreateInspection(context.Context, *core.InspectionReport) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetInspection(context.Context, int64) (*core.InspectionReport, error) {
	panic("unused")
}
func (n *noopStore) ListInspections(context.Context, core.InspectionFilter) ([]*core.InspectionReport, error) {
	panic("unused")
}
func (n *noopStore) UpdateInspection(context.Context, *core.InspectionReport) error { panic("unused") }
func (n *noopStore) CreateFinding(context.Context, *core.InspectionFinding) (int64, error) {
	panic("unused")
}
func (n *noopStore) ListFindingsByInspection(context.Context, int64) ([]*core.InspectionFinding, error) {
	panic("unused")
}
func (n *noopStore) ListRecentFindings(context.Context, core.FindingCategory, int) ([]*core.InspectionFinding, error) {
	panic("unused")
}
func (n *noopStore) CreateInsight(context.Context, *core.InspectionInsight) (int64, error) {
	panic("unused")
}
func (n *noopStore) ListInsightsByInspection(context.Context, int64) ([]*core.InspectionInsight, error) {
	panic("unused")
}
func (n *noopStore) GetFindingRecurrenceCount(context.Context, core.FindingCategory, *int64, *int64) (int, error) {
	panic("unused")
}
func (n *noopStore) CreateThreadTaskGroup(context.Context, *core.ThreadTaskGroup) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetThreadTaskGroup(context.Context, int64) (*core.ThreadTaskGroup, error) {
	panic("unused")
}
func (n *noopStore) ListThreadTaskGroups(context.Context, core.ThreadTaskGroupFilter) ([]*core.ThreadTaskGroup, error) {
	panic("unused")
}
func (n *noopStore) UpdateThreadTaskGroup(context.Context, *core.ThreadTaskGroup) error {
	panic("unused")
}
func (n *noopStore) DeleteThreadTaskGroup(context.Context, int64) error { panic("unused") }
func (n *noopStore) CreateThreadTask(context.Context, *core.ThreadTask) (int64, error) {
	panic("unused")
}
func (n *noopStore) GetThreadTask(context.Context, int64) (*core.ThreadTask, error) {
	panic("unused")
}
func (n *noopStore) ListThreadTasksByGroup(context.Context, int64) ([]*core.ThreadTask, error) {
	panic("unused")
}
func (n *noopStore) UpdateThreadTask(context.Context, *core.ThreadTask) error { panic("unused") }
func (n *noopStore) DeleteThreadTasksByGroup(context.Context, int64) error    { panic("unused") }
func (n *noopStore) Close() error                                            { return nil }

func assertErr(message string) error { return &errString{message: message} }

type errString struct{ message string }

func (e *errString) Error() string { return e.message }
