package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

// stubBriefingStore is a minimal in-memory store for BriefingBuilder tests.
// It embeds panicStore to satisfy the full Store interface — only the methods
// actually called by BriefingBuilder are overridden.
type stubBriefingStore struct {
	panicStore
	issues    map[int64]*core.Issue
	steps     map[int64][]*core.Step   // keyed by IssueID
	artifacts map[int64]*core.Artifact // keyed by StepID (latest)
}

func newStubBriefingStore() *stubBriefingStore {
	return &stubBriefingStore{
		issues:    make(map[int64]*core.Issue),
		steps:     make(map[int64][]*core.Step),
		artifacts: make(map[int64]*core.Artifact),
	}
}

func (s *stubBriefingStore) GetIssue(_ context.Context, id int64) (*core.Issue, error) {
	if issue, ok := s.issues[id]; ok {
		return issue, nil
	}
	return nil, core.ErrNotFound
}

func (s *stubBriefingStore) ListStepsByIssue(_ context.Context, issueID int64) ([]*core.Step, error) {
	return s.steps[issueID], nil
}

func (s *stubBriefingStore) GetLatestArtifactByStep(_ context.Context, stepID int64) (*core.Artifact, error) {
	if art, ok := s.artifacts[stepID]; ok {
		return art, nil
	}
	return nil, core.ErrNotFound
}

func (s *stubBriefingStore) GetFeatureManifestByProject(_ context.Context, _ int64) (*core.FeatureManifest, error) {
	return nil, core.ErrNotFound
}

func (s *stubBriefingStore) CreateBriefing(_ context.Context, _ *core.Briefing) (int64, error) {
	return 1, nil
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

func (panicStore) CreateResourceBinding(context.Context, *core.ResourceBinding) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetResourceBinding(context.Context, int64) (*core.ResourceBinding, error) {
	panic("not implemented")
}
func (panicStore) ListResourceBindings(context.Context, int64) ([]*core.ResourceBinding, error) {
	panic("not implemented")
}
func (panicStore) DeleteResourceBinding(context.Context, int64) error { panic("not implemented") }

func (panicStore) CreateIssue(context.Context, *core.Issue) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetIssue(context.Context, int64) (*core.Issue, error) {
	panic("not implemented")
}
func (panicStore) ListIssues(context.Context, core.IssueFilter) ([]*core.Issue, error) {
	panic("not implemented")
}
func (panicStore) UpdateIssue(context.Context, *core.Issue) error { panic("not implemented") }
func (panicStore) UpdateIssueStatus(context.Context, int64, core.IssueStatus) error {
	panic("not implemented")
}
func (panicStore) UpdateIssueMetadata(context.Context, int64, map[string]any) error {
	panic("not implemented")
}
func (panicStore) PrepareIssueRun(context.Context, int64, core.IssueStatus) error {
	panic("not implemented")
}
func (panicStore) SetIssueArchived(context.Context, int64, bool) error { panic("not implemented") }
func (panicStore) DeleteIssue(context.Context, int64) error            { panic("not implemented") }

func (panicStore) CreateStep(context.Context, *core.Step) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetStep(context.Context, int64) (*core.Step, error) { panic("not implemented") }
func (panicStore) ListStepsByIssue(context.Context, int64) ([]*core.Step, error) {
	panic("not implemented")
}
func (panicStore) UpdateStepStatus(context.Context, int64, core.StepStatus) error {
	panic("not implemented")
}
func (panicStore) UpdateStep(context.Context, *core.Step) error { panic("not implemented") }
func (panicStore) DeleteStep(context.Context, int64) error      { panic("not implemented") }

func (panicStore) CreateExecution(context.Context, *core.Execution) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetExecution(context.Context, int64) (*core.Execution, error) {
	panic("not implemented")
}
func (panicStore) ListExecutionsByStep(context.Context, int64) ([]*core.Execution, error) {
	panic("not implemented")
}
func (panicStore) ListExecutionsByStatus(context.Context, core.ExecutionStatus) ([]*core.Execution, error) {
	panic("not implemented")
}
func (panicStore) UpdateExecution(context.Context, *core.Execution) error {
	panic("not implemented")
}

func (panicStore) CreateArtifact(context.Context, *core.Artifact) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetArtifact(context.Context, int64) (*core.Artifact, error) {
	panic("not implemented")
}
func (panicStore) GetLatestArtifactByStep(context.Context, int64) (*core.Artifact, error) {
	panic("not implemented")
}
func (panicStore) ListArtifactsByExecution(context.Context, int64) ([]*core.Artifact, error) {
	panic("not implemented")
}
func (panicStore) UpdateArtifact(context.Context, *core.Artifact) error {
	panic("not implemented")
}

func (panicStore) CreateBriefing(context.Context, *core.Briefing) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetBriefing(context.Context, int64) (*core.Briefing, error) {
	panic("not implemented")
}
func (panicStore) GetBriefingByStep(context.Context, int64) (*core.Briefing, error) {
	panic("not implemented")
}

func (panicStore) CreateFeatureManifest(context.Context, *core.FeatureManifest) (int64, error) {
	panic("not implemented")
}
func (panicStore) GetFeatureManifest(context.Context, int64) (*core.FeatureManifest, error) {
	panic("not implemented")
}
func (panicStore) GetFeatureManifestByProject(context.Context, int64) (*core.FeatureManifest, error) {
	panic("not implemented")
}
func (panicStore) UpdateFeatureManifest(context.Context, *core.FeatureManifest) error {
	panic("not implemented")
}
func (panicStore) DeleteFeatureManifest(context.Context, int64) error { panic("not implemented") }
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

// --- Tests ---

func TestBriefingBuilder_InjectsIssueSummary(t *testing.T) {
	store := newStubBriefingStore()
	store.issues[1] = &core.Issue{
		ID:    1,
		Title: "Implement login page",
		Body:  "Create a login form with email and password fields.",
	}

	step := &core.Step{ID: 10, IssueID: 1, Name: "implement", Position: 0}
	store.steps[1] = []*core.Step{step}

	builder := NewBriefingBuilder(store)
	briefing, err := builder.Build(context.Background(), step)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	var found bool
	for _, ref := range briefing.ContextRefs {
		if ref.Type == core.CtxIssueSummary {
			found = true
			if !strings.Contains(ref.Inline, "Implement login page") {
				t.Errorf("expected issue title in inline, got: %q", ref.Inline)
			}
			if !strings.Contains(ref.Inline, "login form") {
				t.Errorf("expected issue body in inline, got: %q", ref.Inline)
			}
			if ref.Label != "work item" {
				t.Errorf("expected label 'work item', got: %q", ref.Label)
			}
		}
	}
	if !found {
		t.Fatal("expected CtxIssueSummary in context refs, not found")
	}
}

func TestBriefingBuilder_IssueSummaryTruncatesLongBody(t *testing.T) {
	store := newStubBriefingStore()
	longBody := strings.Repeat("x", 1000)
	store.issues[1] = &core.Issue{ID: 1, Title: "T", Body: longBody}

	step := &core.Step{ID: 10, IssueID: 1, Name: "s", Position: 0}
	store.steps[1] = []*core.Step{step}

	builder := NewBriefingBuilder(store)
	briefing, err := builder.Build(context.Background(), step)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, ref := range briefing.ContextRefs {
		if ref.Type == core.CtxIssueSummary {
			if !strings.Contains(ref.Inline, "[...]") {
				t.Error("expected truncation marker for long body")
			}
			if len(ref.Inline) > 600 {
				t.Errorf("inline too long after truncation: %d chars", len(ref.Inline))
			}
			return
		}
	}
	t.Fatal("CtxIssueSummary not found")
}

func TestBriefingBuilder_SkipsIssueSummaryWhenNoTitle(t *testing.T) {
	store := newStubBriefingStore()
	store.issues[1] = &core.Issue{ID: 1, Title: "", Body: "some body"}

	step := &core.Step{ID: 10, IssueID: 1, Name: "s", Position: 0}
	store.steps[1] = []*core.Step{step}

	builder := NewBriefingBuilder(store)
	briefing, err := builder.Build(context.Background(), step)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, ref := range briefing.ContextRefs {
		if ref.Type == core.CtxIssueSummary {
			t.Fatal("expected no CtxIssueSummary when title is empty")
		}
	}
}

func TestBriefingBuilder_ImmediatePredecessorGetsFullContent(t *testing.T) {
	store := newStubBriefingStore()
	store.issues[1] = &core.Issue{ID: 1, Title: "T"}

	fullMarkdown := "Full implementation details with lots of content."
	store.steps[1] = []*core.Step{
		{ID: 100, IssueID: 1, Position: 0, Status: core.StepDone},
		{ID: 101, IssueID: 1, Position: 1, Status: core.StepReady},
	}
	store.artifacts[100] = &core.Artifact{
		ID:             1,
		StepID:         100,
		ResultMarkdown: fullMarkdown,
	}

	step := store.steps[1][1] // Position 1
	builder := NewBriefingBuilder(store)
	briefing, err := builder.Build(context.Background(), step)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, ref := range briefing.ContextRefs {
		if ref.Type == core.CtxUpstreamArtifact {
			if ref.Inline != fullMarkdown {
				t.Errorf("expected full markdown for immediate predecessor, got: %q", ref.Inline)
			}
			if !strings.Contains(ref.Label, "output") {
				t.Errorf("expected 'output' label for immediate predecessor, got: %q", ref.Label)
			}
			return
		}
	}
	t.Fatal("expected upstream artifact ref, not found")
}

func TestBriefingBuilder_DistantPredecessorGetsSummary(t *testing.T) {
	store := newStubBriefingStore()
	store.issues[1] = &core.Issue{ID: 1, Title: "T"}

	store.steps[1] = []*core.Step{
		{ID: 100, IssueID: 1, Position: 0, Status: core.StepDone},
		{ID: 101, IssueID: 1, Position: 1, Status: core.StepDone},
		{ID: 102, IssueID: 1, Position: 2, Status: core.StepReady},
	}
	// Step 100 is distant (position 0), step 101 is immediate (position 1).
	store.artifacts[100] = &core.Artifact{
		ID:             1,
		StepID:         100,
		ResultMarkdown: strings.Repeat("A very detailed output. ", 100),
		Metadata:       map[string]any{"summary": "Completed initial setup."},
	}
	store.artifacts[101] = &core.Artifact{
		ID:             2,
		StepID:         101,
		ResultMarkdown: "Direct predecessor output.",
	}

	step := store.steps[1][2] // Position 2
	builder := NewBriefingBuilder(store)
	briefing, err := builder.Build(context.Background(), step)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	var distantRef, immediateRef *core.ContextRef
	for i, ref := range briefing.ContextRefs {
		if ref.Type != core.CtxUpstreamArtifact {
			continue
		}
		if strings.Contains(ref.Label, "summary") {
			distantRef = &briefing.ContextRefs[i]
		}
		if strings.Contains(ref.Label, "output") {
			immediateRef = &briefing.ContextRefs[i]
		}
	}

	if distantRef == nil {
		t.Fatal("expected distant predecessor summary ref")
	}
	if distantRef.Inline != "Completed initial setup." {
		t.Errorf("expected Metadata summary for distant ref, got: %q", distantRef.Inline)
	}

	if immediateRef == nil {
		t.Fatal("expected immediate predecessor output ref")
	}
	if immediateRef.Inline != "Direct predecessor output." {
		t.Errorf("expected full markdown for immediate ref, got: %q", immediateRef.Inline)
	}
}

func TestBriefingBuilder_DistantPredecessorFallsBackToTruncatedMarkdown(t *testing.T) {
	store := newStubBriefingStore()
	store.issues[1] = &core.Issue{ID: 1, Title: "T"}

	longMarkdown := strings.Repeat("x", 500)
	store.steps[1] = []*core.Step{
		{ID: 100, IssueID: 1, Position: 0, Status: core.StepDone},
		{ID: 101, IssueID: 1, Position: 1, Status: core.StepDone},
		{ID: 102, IssueID: 1, Position: 2, Status: core.StepReady},
	}
	// Distant artifact with no Metadata summary — should fallback to truncated markdown.
	store.artifacts[100] = &core.Artifact{
		ID:             1,
		StepID:         100,
		ResultMarkdown: longMarkdown,
	}
	store.artifacts[101] = &core.Artifact{
		ID:             2,
		StepID:         101,
		ResultMarkdown: "ok",
	}

	step := store.steps[1][2]
	builder := NewBriefingBuilder(store)
	briefing, err := builder.Build(context.Background(), step)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, ref := range briefing.ContextRefs {
		if ref.Type == core.CtxUpstreamArtifact && strings.Contains(ref.Label, "summary") {
			if !strings.Contains(ref.Inline, "[...]") {
				t.Error("expected truncation marker for distant artifact without Metadata summary")
			}
			if len(ref.Inline) > maxSummaryFallbackChars+20 {
				t.Errorf("summary too long: %d chars", len(ref.Inline))
			}
			return
		}
	}
	t.Fatal("expected distant predecessor summary ref with fallback")
}

func TestBriefingBuilder_ContextRefPriorityOrder(t *testing.T) {
	store := newStubBriefingStore()
	store.issues[1] = &core.Issue{ID: 1, Title: "My Issue", Body: "desc"}

	store.steps[1] = []*core.Step{
		{ID: 100, IssueID: 1, Position: 0, Status: core.StepDone},
		{ID: 101, IssueID: 1, Position: 1, Status: core.StepReady},
	}
	store.artifacts[100] = &core.Artifact{
		ID: 1, StepID: 100, ResultMarkdown: "output",
	}

	builder := NewBriefingBuilder(store)
	briefing, err := builder.Build(context.Background(), store.steps[1][1])
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(briefing.ContextRefs) < 2 {
		t.Fatalf("expected at least 2 context refs, got %d", len(briefing.ContextRefs))
	}
	if briefing.ContextRefs[0].Type != core.CtxIssueSummary {
		t.Errorf("expected first ref to be CtxIssueSummary, got %s", briefing.ContextRefs[0].Type)
	}
	if briefing.ContextRefs[1].Type != core.CtxUpstreamArtifact {
		t.Errorf("expected second ref to be CtxUpstreamArtifact, got %s", briefing.ContextRefs[1].Type)
	}
}

func TestExtractArtifactSummary_PrefersMetadata(t *testing.T) {
	art := &core.Artifact{
		ResultMarkdown: strings.Repeat("long content ", 100),
		Metadata:       map[string]any{"summary": "Short summary from collector."},
	}
	got := extractArtifactSummary(art)
	if got != "Short summary from collector." {
		t.Errorf("expected metadata summary, got: %q", got)
	}
}

func TestExtractArtifactSummary_FallbackTruncation(t *testing.T) {
	art := &core.Artifact{
		ResultMarkdown: strings.Repeat("x", 500),
	}
	got := extractArtifactSummary(art)
	if !strings.HasSuffix(got, "[...]") {
		t.Error("expected [...] suffix for truncated fallback")
	}
	if len(got) > maxSummaryFallbackChars+10 {
		t.Errorf("fallback too long: %d", len(got))
	}
}

func TestExtractArtifactSummary_ShortMarkdownNotTruncated(t *testing.T) {
	art := &core.Artifact{
		ResultMarkdown: "Short output.",
	}
	got := extractArtifactSummary(art)
	if got != "Short output." {
		t.Errorf("expected exact short markdown, got: %q", got)
	}
}

func TestExtractArtifactSummary_EmptyArtifact(t *testing.T) {
	art := &core.Artifact{}
	got := extractArtifactSummary(art)
	if got != "" {
		t.Errorf("expected empty string for empty artifact, got: %q", got)
	}
}
