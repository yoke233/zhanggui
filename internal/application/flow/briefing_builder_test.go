package flow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

// stubInputStore is a minimal in-memory store for InputBuilder tests.
// It embeds panicStore to satisfy the full Store interface — only the methods
// actually called by InputBuilder are overridden.
type stubInputStore struct {
	panicStore
	workItems map[int64]*core.WorkItem
	actions   map[int64][]*core.Action // keyed by WorkItemID
	runs      map[int64]*core.Run      // keyed by ActionID (latest run with result)
	projects  map[int64]*core.Project
	spaces    map[int64][]*core.ResourceSpace // keyed by ProjectID
}

func newStubInputStore() *stubInputStore {
	return &stubInputStore{
		workItems: make(map[int64]*core.WorkItem),
		actions:   make(map[int64][]*core.Action),
		runs:      make(map[int64]*core.Run),
		projects:  make(map[int64]*core.Project),
		spaces:    make(map[int64][]*core.ResourceSpace),
	}
}

func (s *stubInputStore) GetWorkItem(_ context.Context, id int64) (*core.WorkItem, error) {
	if workItem, ok := s.workItems[id]; ok {
		return workItem, nil
	}
	return nil, core.ErrNotFound
}

func (s *stubInputStore) ListActionsByWorkItem(_ context.Context, workItemID int64) ([]*core.Action, error) {
	return s.actions[workItemID], nil
}

func (s *stubInputStore) GetLatestRunWithResult(_ context.Context, actionID int64) (*core.Run, error) {
	if run, ok := s.runs[actionID]; ok {
		return run, nil
	}
	return nil, core.ErrNotFound
}

func (s *stubInputStore) ListFeatureEntries(_ context.Context, _ core.FeatureEntryFilter) ([]*core.FeatureEntry, error) {
	return nil, nil
}

func (s *stubInputStore) GetProject(_ context.Context, id int64) (*core.Project, error) {
	if project, ok := s.projects[id]; ok {
		return project, nil
	}
	return nil, core.ErrNotFound
}

func (s *stubInputStore) ListResourceSpaces(_ context.Context, projectID int64) ([]*core.ResourceSpace, error) {
	return s.spaces[projectID], nil
}

// --- panicStore satisfies Store by panicking on any unimplemented method ---

type panicStore struct{}

func (panicStore) CreateProject(context.Context, *core.Project) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetProject(context.Context, int64) (*core.Project, error) {
	panic("not implemented")
}
func (panicStore) ListProjects(context.Context, int, int) ([]*core.Project, error) {
	panic("not implemented")
}
func (panicStore) UpdateProject(context.Context, *core.Project) error {
	panic("not implemented")
}
func (panicStore) DeleteProject(context.Context, int64) error { panic("not implemented") }

func (panicStore) CreateResourceSpace(context.Context, *core.ResourceSpace) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetResourceSpace(context.Context, int64) (*core.ResourceSpace, error) {
	panic("not implemented")
}
func (panicStore) ListResourceSpaces(context.Context, int64) ([]*core.ResourceSpace, error) {
	panic("not implemented")
}
func (panicStore) UpdateResourceSpace(context.Context, *core.ResourceSpace) error {
	panic("not implemented")
}
func (panicStore) DeleteResourceSpace(context.Context, int64) error {
	panic("not implemented")
}

func (panicStore) CreateResource(context.Context, *core.Resource) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetResource(context.Context, int64) (*core.Resource, error) {
	panic("not implemented")
}
func (panicStore) ListResourcesByWorkItem(context.Context, int64) ([]*core.Resource, error) {
	panic("not implemented")
}
func (panicStore) ListResourcesByRun(context.Context, int64) ([]*core.Resource, error) {
	panic("not implemented")
}
func (panicStore) ListResourcesByMessage(context.Context, int64) ([]*core.Resource, error) {
	panic("not implemented")
}
func (panicStore) DeleteResource(context.Context, int64) error { panic("not implemented") }

func (panicStore) CreateActionIODecl(context.Context, *core.ActionIODecl) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetActionIODecl(context.Context, int64) (*core.ActionIODecl, error) {
	panic("not implemented")
}
func (panicStore) ListActionIODecls(context.Context, int64) ([]*core.ActionIODecl, error) {
	panic("not implemented")
}
func (panicStore) ListActionIODeclsByDirection(context.Context, int64, core.IODirection) ([]*core.ActionIODecl, error) {
	panic("not implemented")
}
func (panicStore) DeleteActionIODecl(context.Context, int64) error { panic("not implemented") }

func (panicStore) CreateWorkItem(context.Context, *core.WorkItem) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetWorkItem(context.Context, int64) (*core.WorkItem, error) {
	panic("not implemented")
}
func (panicStore) ListWorkItems(context.Context, core.WorkItemFilter) ([]*core.WorkItem, error) {
	panic("not implemented")
}
func (panicStore) UpdateWorkItem(context.Context, *core.WorkItem) error { panic("not implemented") }
func (panicStore) UpdateWorkItemStatus(context.Context, int64, core.WorkItemStatus) error {
	panic("not implemented")
}
func (panicStore) UpdateWorkItemMetadata(context.Context, int64, map[string]any) error {
	panic("not implemented")
}
func (panicStore) PrepareWorkItemRun(context.Context, int64, core.WorkItemStatus) error {
	panic("not implemented")
}
func (panicStore) SetWorkItemArchived(context.Context, int64, bool) error { panic("not implemented") }
func (panicStore) DeleteWorkItem(context.Context, int64) error            { panic("not implemented") }

func (panicStore) CreateAction(context.Context, *core.Action) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetAction(context.Context, int64) (*core.Action, error) { panic("not implemented") }
func (panicStore) ListActionsByWorkItem(context.Context, int64) ([]*core.Action, error) {
	panic("not implemented")
}
func (panicStore) UpdateActionStatus(context.Context, int64, core.ActionStatus) error {
	panic("not implemented")
}
func (panicStore) UpdateAction(context.Context, *core.Action) error         { panic("not implemented") }
func (panicStore) DeleteAction(context.Context, int64) error                { panic("not implemented") }
func (panicStore) BatchCreateActions(context.Context, []*core.Action) error { panic("not implemented") }
func (panicStore) UpdateActionDependsOn(context.Context, int64, []int64) error {
	panic("not implemented")
}

func (panicStore) CreateRun(context.Context, *core.Run) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetRun(context.Context, int64) (*core.Run, error) {
	panic("not implemented")
}
func (panicStore) ListRunsByAction(context.Context, int64) ([]*core.Run, error) {
	panic("not implemented")
}
func (panicStore) ListRunsByStatus(context.Context, core.RunStatus) ([]*core.Run, error) {
	panic("not implemented")
}
func (panicStore) UpdateRun(context.Context, *core.Run) error {
	panic("not implemented")
}

func (panicStore) GetLatestRunWithResult(context.Context, int64) (*core.Run, error) {
	panic("not implemented")
}

func (panicStore) CreateFeatureEntry(context.Context, *core.FeatureEntry) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetFeatureEntry(context.Context, int64) (*core.FeatureEntry, error) {
	panic("not implemented")
}
func (panicStore) GetFeatureEntryByKey(context.Context, int64, string) (*core.FeatureEntry, error) {
	panic("not implemented")
}
func (panicStore) ListFeatureEntries(context.Context, core.FeatureEntryFilter) ([]*core.FeatureEntry, error) {
	panic("not implemented")
}
func (panicStore) UpdateFeatureEntry(context.Context, *core.FeatureEntry) error {
	panic("not implemented")
}
func (panicStore) UpdateFeatureEntryStatus(context.Context, int64, core.FeatureStatus) error {
	panic("not implemented")
}
func (panicStore) DeleteFeatureEntry(context.Context, int64) error { panic("not implemented") }
func (panicStore) CountFeatureEntriesByStatus(context.Context, int64) (map[core.FeatureStatus]int, error) {
	panic("not implemented")
}

func (panicStore) CreateActionSignal(context.Context, *core.ActionSignal) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetLatestActionSignal(context.Context, int64, ...core.SignalType) (*core.ActionSignal, error) {
	panic("not implemented")
}
func (panicStore) ListActionSignals(context.Context, int64) ([]*core.ActionSignal, error) {
	panic("not implemented")
}
func (panicStore) ListActionSignalsByType(context.Context, int64, ...core.SignalType) ([]*core.ActionSignal, error) {
	panic("not implemented")
}
func (panicStore) CountActionSignals(context.Context, int64, ...core.SignalType) (int, error) {
	panic("not implemented")
}
func (panicStore) ListPendingHumanActions(context.Context, int64) ([]*core.Action, error) {
	panic("not implemented")
}
func (panicStore) ListAllPendingHumanActions(context.Context) ([]*core.Action, error) {
	panic("not implemented")
}
func (panicStore) ListProbeSignalsByRun(context.Context, int64) ([]*core.ActionSignal, error) {
	panic("not implemented")
}
func (panicStore) GetLatestProbeSignal(context.Context, int64) (*core.ActionSignal, error) {
	panic("not implemented")
}
func (panicStore) GetActiveProbeSignal(context.Context, int64) (*core.ActionSignal, error) {
	panic("not implemented")
}
func (panicStore) UpdateProbeSignal(context.Context, *core.ActionSignal) error {
	panic("not implemented")
}
func (panicStore) GetRunProbeRoute(context.Context, int64) (*core.RunProbeRoute, error) {
	panic("not implemented")
}

// --- Tests ---

func TestInputBuilder_InjectsWorkItemSummary(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{
		ID:    1,
		Title: "Implement login page",
		Body:  "Create a login form with email and password fields.",
	}

	action := &core.Action{ID: 10, WorkItemID: 1, Name: "implement", Position: 0}
	store.actions[1] = []*core.Action{action}

	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !strings.Contains(input, "Implement login page") {
		t.Errorf("expected work item title in input, got: %q", input)
	}
	if !strings.Contains(input, "login form") {
		t.Errorf("expected work item body in input, got: %q", input)
	}
	if !strings.Contains(input, "work item") {
		t.Errorf("expected label 'work item' in input, got: %q", input)
	}
}

func TestInputBuilder_WorkItemSummaryTruncatesLongBody(t *testing.T) {
	store := newStubInputStore()
	longBody := strings.Repeat("x", 1000)
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "T", Body: longBody}

	action := &core.Action{ID: 10, WorkItemID: 1, Name: "s", Position: 0}
	store.actions[1] = []*core.Action{action}

	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !strings.Contains(input, "[...]") {
		t.Error("expected truncation marker for long body")
	}
}

func TestInputBuilder_SkipsWorkItemSummaryWhenNoTitle(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "", Body: "some body"}

	action := &core.Action{ID: 10, WorkItemID: 1, Name: "s", Position: 0}
	store.actions[1] = []*core.Action{action}

	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if strings.Contains(input, "work item") {
		t.Fatal("expected no work item summary when title is empty")
	}
}

func TestInputBuilder_ImmediatePredecessorGetsFullContent(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "T"}

	fullMarkdown := "Full implementation details with lots of content."
	store.actions[1] = []*core.Action{
		{ID: 100, WorkItemID: 1, Position: 0, Status: core.ActionDone},
		{ID: 101, WorkItemID: 1, Position: 1, Status: core.ActionReady},
	}
	store.runs[100] = &core.Run{
		ID:             1,
		ActionID:       100,
		ResultMarkdown: fullMarkdown,
	}

	action := store.actions[1][1] // Position 1
	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !strings.Contains(input, fullMarkdown) {
		t.Errorf("expected full markdown for immediate predecessor, got: %q", input)
	}
	if !strings.Contains(input, "output") {
		t.Errorf("expected 'output' label for immediate predecessor, got: %q", input)
	}
}

func TestInputBuilder_DistantPredecessorGetsSummary(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "T"}

	store.actions[1] = []*core.Action{
		{ID: 100, WorkItemID: 1, Position: 0, Status: core.ActionDone},
		{ID: 101, WorkItemID: 1, Position: 1, Status: core.ActionDone},
		{ID: 102, WorkItemID: 1, Position: 2, Status: core.ActionReady},
	}
	// Action 100 is distant (position 0), action 101 is immediate (position 1).
	store.runs[100] = &core.Run{
		ID:             1,
		ActionID:       100,
		ResultMarkdown: strings.Repeat("A very detailed output. ", 100),
		ResultMetadata: map[string]any{"summary": "Completed initial setup."},
	}
	store.runs[101] = &core.Run{
		ID:             2,
		ActionID:       101,
		ResultMarkdown: "Direct predecessor output.",
	}

	action := store.actions[1][2] // Position 2
	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !strings.Contains(input, "Completed initial setup.") {
		t.Errorf("expected Metadata summary for distant ref, got: %q", input)
	}
	if !strings.Contains(input, "Direct predecessor output.") {
		t.Errorf("expected full markdown for immediate ref, got: %q", input)
	}
}

func TestInputBuilder_DistantPredecessorFallsBackToTruncatedMarkdown(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "T"}

	longMarkdown := strings.Repeat("x", 500)
	store.actions[1] = []*core.Action{
		{ID: 100, WorkItemID: 1, Position: 0, Status: core.ActionDone},
		{ID: 101, WorkItemID: 1, Position: 1, Status: core.ActionDone},
		{ID: 102, WorkItemID: 1, Position: 2, Status: core.ActionReady},
	}
	// Distant run with no ResultMetadata summary — should fallback to truncated markdown.
	store.runs[100] = &core.Run{
		ID:             1,
		ActionID:       100,
		ResultMarkdown: longMarkdown,
	}
	store.runs[101] = &core.Run{
		ID:             2,
		ActionID:       101,
		ResultMarkdown: "ok",
	}

	action := store.actions[1][2]
	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !strings.Contains(input, "[...]") {
		t.Error("expected truncation marker for distant deliverable without Metadata summary")
	}
}

func TestInputBuilder_ContextRefPriorityOrder(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "My WorkItem", Body: "desc"}

	store.actions[1] = []*core.Action{
		{ID: 100, WorkItemID: 1, Position: 0, Status: core.ActionDone},
		{ID: 101, WorkItemID: 1, Position: 1, Status: core.ActionReady},
	}
	store.runs[100] = &core.Run{
		ID: 1, ActionID: 100, ResultMarkdown: "output",
	}

	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), store.actions[1][1])
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// The input should contain work item summary before upstream deliverable content
	wiIdx := strings.Index(input, "work item")
	upIdx := strings.Index(input, "upstream")
	if wiIdx < 0 || upIdx < 0 {
		t.Fatalf("expected both work item and upstream sections, got: %q", input)
	}
	if wiIdx > upIdx {
		t.Errorf("expected work item summary before upstream deliverable")
	}
}

func TestExtractRunResultSummary_PrefersMetadata(t *testing.T) {
	run := &core.Run{
		ResultMarkdown: strings.Repeat("long content ", 100),
		ResultMetadata: map[string]any{"summary": "Short summary from collector."},
	}
	got := extractRunResultSummary(run)
	if got != "Short summary from collector." {
		t.Errorf("expected metadata summary, got: %q", got)
	}
}

func TestExtractRunResultSummary_FallbackTruncation(t *testing.T) {
	run := &core.Run{
		ResultMarkdown: strings.Repeat("x", 500),
	}
	got := extractRunResultSummary(run)
	if !strings.HasSuffix(got, "[...]") {
		t.Error("expected [...] suffix for truncated fallback")
	}
	if len(got) > maxSummaryFallbackChars+10 {
		t.Errorf("fallback too long: %d", len(got))
	}
}

func TestExtractRunResultSummary_ShortMarkdownNotTruncated(t *testing.T) {
	run := &core.Run{
		ResultMarkdown: "Short output.",
	}
	got := extractRunResultSummary(run)
	if got != "Short output." {
		t.Errorf("expected exact short markdown, got: %q", got)
	}
}

func TestExtractRunResultSummary_EmptyRun(t *testing.T) {
	run := &core.Run{}
	got := extractRunResultSummary(run)
	if got != "" {
		t.Errorf("expected empty string for empty run, got: %q", got)
	}
}

// --- Project Brief tests ---

func TestInputBuilder_InjectsProjectBrief(t *testing.T) {
	store := newStubInputStore()
	projID := int64(10)
	store.projects[projID] = &core.Project{
		ID:          projID,
		Name:        "my-app",
		Kind:        core.ProjectDev,
		Description: "A sample application for testing.",
	}
	store.spaces[projID] = []*core.ResourceSpace{
		{ID: 1, ProjectID: projID, Kind: "git", RootURI: "https://github.com/example/my-app", Role: "primary_repo", Label: "main repo"},
	}
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "Task", ProjectID: &projID}
	action := &core.Action{ID: 10, WorkItemID: 1, Name: "implement", Position: 0}
	store.actions[1] = []*core.Action{action}

	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !strings.Contains(input, "my-app") {
		t.Errorf("expected project name in input, got: %q", input)
	}
	if !strings.Contains(input, "dev") {
		t.Errorf("expected project kind in input, got: %q", input)
	}
	if !strings.Contains(input, "sample application") {
		t.Errorf("expected project description in input, got: %q", input)
	}
	if !strings.Contains(input, "main repo") {
		t.Errorf("expected resource space label in input, got: %q", input)
	}
	if !strings.Contains(input, "github.com/example/my-app") {
		t.Errorf("expected resource space root URI in input, got: %q", input)
	}
	if !strings.Contains(input, "[primary_repo]") {
		t.Errorf("expected resource space role in input, got: %q", input)
	}
}

func TestInputBuilder_SkipsProjectBriefWhenNoProject(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "Task"}
	action := &core.Action{ID: 10, WorkItemID: 1, Name: "s", Position: 0}
	store.actions[1] = []*core.Action{action}

	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if strings.Contains(input, "project") {
		t.Errorf("expected no project section when no ProjectID, got: %q", input)
	}
}

// --- Progress Summary tests ---

func TestInputBuilder_InjectsProgressSummary(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "Task"}
	store.actions[1] = []*core.Action{
		{ID: 100, WorkItemID: 1, Name: "plan", Position: 0, Status: core.ActionDone},
		{ID: 101, WorkItemID: 1, Name: "implement", Position: 1, Status: core.ActionRunning},
		{ID: 102, WorkItemID: 1, Name: "review", Position: 2, Status: core.ActionPending},
	}

	action := store.actions[1][1] // implement (running)
	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !strings.Contains(input, "1/3 actions completed") {
		t.Errorf("expected progress fraction, got: %q", input)
	}
	if !strings.Contains(input, "[done] plan") {
		t.Errorf("expected done marker for plan, got: %q", input)
	}
	if !strings.Contains(input, "[running] implement") {
		t.Errorf("expected running marker for implement, got: %q", input)
	}
	if !strings.Contains(input, "← current") {
		t.Errorf("expected current marker, got: %q", input)
	}
	if !strings.Contains(input, "[pending] review") {
		t.Errorf("expected pending marker for review, got: %q", input)
	}
}

func TestInputBuilder_SkipsProgressWhenSingleAction(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "Task"}
	action := &core.Action{ID: 10, WorkItemID: 1, Name: "only", Position: 0}
	store.actions[1] = []*core.Action{action}

	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if strings.Contains(input, "Progress:") {
		t.Errorf("expected no progress section for single action, got: %q", input)
	}
}

// --- Context Ref Priority Order with new types ---

func TestInputBuilder_ProjectBriefBeforeWorkItemSummary(t *testing.T) {
	store := newStubInputStore()
	projID := int64(10)
	store.projects[projID] = &core.Project{ID: projID, Name: "my-proj", Kind: core.ProjectDev}
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "My Task", ProjectID: &projID}
	action := &core.Action{ID: 10, WorkItemID: 1, Name: "s", Position: 0}
	store.actions[1] = []*core.Action{action}

	builder := NewInputBuilder(store)
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	projIdx := strings.Index(input, "project")
	wiIdx := strings.Index(input, "work item")
	if projIdx < 0 || wiIdx < 0 {
		t.Fatalf("expected both project and work item sections, got: %q", input)
	}
	if projIdx > wiIdx {
		t.Errorf("expected project brief before work item summary")
	}
}

// --- stubRegistry is a minimal AgentRegistry for skills injection tests ---

type stubRegistry struct {
	profiles []*core.AgentProfile
}

func (r *stubRegistry) GetProfile(_ context.Context, id string) (*core.AgentProfile, error) {
	for _, p := range r.profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, core.ErrProfileNotFound
}
func (r *stubRegistry) ListProfiles(_ context.Context) ([]*core.AgentProfile, error) {
	return r.profiles, nil
}
func (r *stubRegistry) CreateProfile(_ context.Context, _ *core.AgentProfile) error { return nil }
func (r *stubRegistry) UpdateProfile(_ context.Context, _ *core.AgentProfile) error { return nil }
func (r *stubRegistry) DeleteProfile(_ context.Context, _ string) error             { return nil }
func (r *stubRegistry) ResolveForAction(_ context.Context, action *core.Action) (*core.AgentProfile, error) {
	role := strings.TrimSpace(action.AgentRole)
	for _, p := range r.profiles {
		if string(p.Role) == role && p.MatchesRequirements(action.RequiredCapabilities) {
			return p, nil
		}
	}
	return nil, core.ErrProfileNotFound
}
func (r *stubRegistry) ResolveByID(_ context.Context, id string) (*core.AgentProfile, error) {
	for _, p := range r.profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, core.ErrProfileNotFound
}

// createTestSkillDir creates a temp skill directory with a valid SKILL.md.
func createTestSkillDir(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\nSkill body."
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- Skills Injection tests ---

func TestInputBuilder_InjectsSkillsSummary(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "Task"}
	action := &core.Action{ID: 10, WorkItemID: 1, Name: "impl", Position: 0, AgentRole: "worker"}
	store.actions[1] = []*core.Action{action}

	root := t.TempDir()
	createTestSkillDir(t, root, "code-review", "Reviews code for quality issues")
	createTestSkillDir(t, root, "testing", "Writes and runs automated tests")

	registry := &stubRegistry{
		profiles: []*core.AgentProfile{
			{ID: "worker-1", Role: core.RoleWorker, Driver: core.DriverConfig{LaunchCommand: "echo"}, Skills: []string{"code-review", "testing"}},
		},
	}

	builder := NewInputBuilder(store, WithRegistry(registry), WithSkillsRoot(root))
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !strings.Contains(input, "code-review") {
		t.Errorf("expected skill name 'code-review' in input, got: %q", input)
	}
	if !strings.Contains(input, "Reviews code for quality issues") {
		t.Errorf("expected skill description in input, got: %q", input)
	}
	if !strings.Contains(input, "testing") {
		t.Errorf("expected skill name 'testing' in input, got: %q", input)
	}
}

func TestInputBuilder_SkipsSkillsWhenNoRegistry(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "Task"}
	action := &core.Action{ID: 10, WorkItemID: 1, Name: "impl", Position: 0, AgentRole: "worker"}
	store.actions[1] = []*core.Action{action}

	builder := NewInputBuilder(store) // no WithRegistry
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if strings.Contains(input, "available skills") {
		t.Errorf("expected no skills section without registry, got: %q", input)
	}
}

func TestInputBuilder_SkipsSkillsWhenNoRole(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "Task"}
	action := &core.Action{ID: 10, WorkItemID: 1, Name: "impl", Position: 0, AgentRole: ""} // no role
	store.actions[1] = []*core.Action{action}

	root := t.TempDir()
	createTestSkillDir(t, root, "code-review", "Reviews code")

	registry := &stubRegistry{
		profiles: []*core.AgentProfile{
			{ID: "worker-1", Role: core.RoleWorker, Driver: core.DriverConfig{LaunchCommand: "echo"}, Skills: []string{"code-review"}},
		},
	}

	builder := NewInputBuilder(store, WithRegistry(registry), WithSkillsRoot(root))
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if strings.Contains(input, "available skills") {
		t.Errorf("expected no skills section without agent role, got: %q", input)
	}
}

func TestInputBuilder_SkipsSkillsWithTODODescription(t *testing.T) {
	store := newStubInputStore()
	store.workItems[1] = &core.WorkItem{ID: 1, Title: "Task"}
	action := &core.Action{ID: 10, WorkItemID: 1, Name: "impl", Position: 0, AgentRole: "worker"}
	store.actions[1] = []*core.Action{action}

	root := t.TempDir()
	createTestSkillDir(t, root, "wip-skill", "TODO")

	registry := &stubRegistry{
		profiles: []*core.AgentProfile{
			{ID: "worker-1", Role: core.RoleWorker, Driver: core.DriverConfig{LaunchCommand: "echo"}, Skills: []string{"wip-skill"}},
		},
	}

	builder := NewInputBuilder(store, WithRegistry(registry), WithSkillsRoot(root))
	input, err := builder.Build(context.Background(), action)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if strings.Contains(input, "available skills") {
		t.Errorf("expected no skills section when all skills have TODO description, got: %q", input)
	}
}
