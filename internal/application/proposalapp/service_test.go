package proposalapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	"github.com/yoke233/zhanggui/internal/core"
)

type proposalTx struct {
	base core.TransactionalStore
}

func (t proposalTx) InTx(ctx context.Context, fn func(ctx context.Context, store Store) error) error {
	return t.base.InTx(ctx, func(store core.Store) error {
		txStore, ok := store.(Store)
		if !ok {
			return core.ErrInvalidTransition
		}
		return fn(ctx, txStore)
	})
}

type recordingBus struct {
	events []core.Event
}

func (b *recordingBus) Publish(_ context.Context, event core.Event) {
	b.events = append(b.events, event)
}

func (b *recordingBus) Subscribe(_ core.SubscribeOpts) *core.Subscription {
	ch := make(chan core.Event)
	close(ch)
	return &core.Subscription{C: ch, Cancel: func() {}}
}

func newProposalServiceTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.New(filepath.Join(t.TempDir(), "proposal-service.db"))
	if err != nil {
		t.Fatalf("sqlite.New() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestServiceApproveProposalMaterializesInitiative(t *testing.T) {
	store := newProposalServiceTestStore(t)
	bus := &recordingBus{}
	svc := New(Config{Store: store, Tx: proposalTx{base: store}, Bus: bus})
	ctx := context.Background()

	projectA, err := store.CreateProject(ctx, &core.Project{Name: "project-a"})
	if err != nil {
		t.Fatalf("CreateProject(project-a): %v", err)
	}
	projectB, err := store.CreateProject(ctx, &core.Project{Name: "project-b"})
	if err != nil {
		t.Fatalf("CreateProject(project-b): %v", err)
	}
	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "multi-project design", Status: core.ThreadActive, OwnerID: "user-1"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	proposal, err := svc.CreateProposal(ctx, CreateProposalInput{
		ThreadID:   threadID,
		Title:      "跨项目 rollout 提案",
		Summary:    "总结讨论结论",
		Content:    "拆成两个 work item 执行",
		ProposedBy: "lead-1",
		WorkItemDrafts: []core.ProposalWorkItemDraft{
			{TempID: "backend", ProjectID: &projectA, Title: "后端改造", Priority: core.PriorityHigh},
			{TempID: "frontend", ProjectID: &projectB, Title: "前端接入", Priority: core.PriorityMedium, DependsOn: []string{"backend"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if _, err := svc.Submit(ctx, proposal.ID); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	proposal, err = svc.Approve(ctx, proposal.ID, ReviewInput{ReviewedBy: "reviewer-1", ReviewNote: "可以执行"})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if proposal.Status != core.ProposalMerged {
		t.Fatalf("proposal status = %s, want merged", proposal.Status)
	}
	if proposal.InitiativeID == nil || *proposal.InitiativeID <= 0 {
		t.Fatalf("proposal initiative_id = %v, want non-nil", proposal.InitiativeID)
	}

	initiative, err := store.GetInitiative(ctx, *proposal.InitiativeID)
	if err != nil {
		t.Fatalf("GetInitiative: %v", err)
	}
	if initiative.Status != core.InitiativeDraft {
		t.Fatalf("initiative status = %s, want draft", initiative.Status)
	}

	items, err := store.ListInitiativeItems(ctx, initiative.ID)
	if err != nil {
		t.Fatalf("ListInitiativeItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("initiative items = %d, want 2", len(items))
	}
	workItems := make(map[int64]*core.WorkItem, len(items))
	for _, item := range items {
		workItem, err := store.GetWorkItem(ctx, item.WorkItemID)
		if err != nil {
			t.Fatalf("GetWorkItem(%d): %v", item.WorkItemID, err)
		}
		workItems[item.WorkItemID] = workItem
	}
	var backend, frontend *core.WorkItem
	for _, workItem := range workItems {
		switch workItem.Title {
		case "后端改造":
			backend = workItem
		case "前端接入":
			frontend = workItem
		}
	}
	if backend == nil || frontend == nil {
		t.Fatalf("materialized work items = %+v", workItems)
	}
	if len(frontend.DependsOn) != 1 || frontend.DependsOn[0] != backend.ID {
		t.Fatalf("frontend depends_on = %+v, want [%d]", frontend.DependsOn, backend.ID)
	}

	links, err := store.ListThreadsByInitiative(ctx, initiative.ID)
	if err != nil {
		t.Fatalf("ListThreadsByInitiative: %v", err)
	}
	if len(links) != 1 || links[0].ThreadID != threadID {
		t.Fatalf("thread links = %+v, want thread %d", links, threadID)
	}

	msgs, err := store.ListThreadMessages(ctx, threadID, 20, 0)
	if err != nil {
		t.Fatalf("ListThreadMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("thread messages = %d, want 3 system messages", len(msgs))
	}
	lastType, _ := msgs[len(msgs)-1].Metadata["type"].(string)
	if lastType != "proposal_merged" {
		t.Fatalf("last message type = %q, want proposal_merged", lastType)
	}

	var sawApproved, sawMerged bool
	for _, event := range bus.events {
		switch event.Type {
		case core.EventThreadProposalApproved:
			sawApproved = true
		case core.EventThreadProposalMerged:
			sawMerged = true
		}
	}
	if !sawApproved || !sawMerged {
		t.Fatalf("proposal events missing approved/merged: %+v", bus.events)
	}
}

func TestServiceRejectAndReviseProposal(t *testing.T) {
	store := newProposalServiceTestStore(t)
	svc := New(Config{Store: store, Tx: proposalTx{base: store}, Bus: &recordingBus{}})
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "proposal lifecycle", Status: core.ThreadActive, OwnerID: "user-1"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	proposal, err := svc.CreateProposal(ctx, CreateProposalInput{
		ThreadID:       threadID,
		Title:          "需要修订的提案",
		Summary:        "初稿",
		WorkItemDrafts: []core.ProposalWorkItemDraft{{TempID: "draft-a", Title: "任务 A"}},
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if _, err := svc.Submit(ctx, proposal.ID); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	proposal, err = svc.Reject(ctx, proposal.ID, ReviewInput{ReviewedBy: "reviewer-1", ReviewNote: "请补依赖关系"})
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if proposal.Status != core.ProposalRejected {
		t.Fatalf("proposal status = %s, want rejected", proposal.Status)
	}
	if proposal.ReviewNote != "请补依赖关系" {
		t.Fatalf("proposal review note = %q", proposal.ReviewNote)
	}

	proposal, err = svc.Revise(ctx, proposal.ID, ReviseInput{ReviewedBy: "lead-1", ReviewNote: "开始修订"})
	if err != nil {
		t.Fatalf("Revise: %v", err)
	}
	if proposal.Status != core.ProposalRevised {
		t.Fatalf("proposal status = %s, want revised", proposal.Status)
	}
}

func TestServiceSubmitRequiresDrafts(t *testing.T) {
	store := newProposalServiceTestStore(t)
	svc := New(Config{Store: store, Tx: proposalTx{base: store}})
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "empty draft", Status: core.ThreadActive, OwnerID: "user-1"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	proposal, err := svc.CreateProposal(ctx, CreateProposalInput{
		ThreadID: threadID,
		Title:    "空草案",
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if _, err := svc.Submit(ctx, proposal.ID); err == nil {
		t.Fatal("expected Submit to fail without work item drafts")
	}
}

func TestServiceSubmitRejectsDependencyCycle(t *testing.T) {
	store := newProposalServiceTestStore(t)
	svc := New(Config{Store: store, Tx: proposalTx{base: store}})
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "cycle draft", Status: core.ThreadActive, OwnerID: "user-1"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	proposal, err := svc.CreateProposal(ctx, CreateProposalInput{
		ThreadID: threadID,
		Title:    "循环依赖",
		WorkItemDrafts: []core.ProposalWorkItemDraft{
			{TempID: "a", Title: "任务 A", DependsOn: []string{"b"}},
			{TempID: "b", Title: "任务 B", DependsOn: []string{"a"}},
		},
	})
	if err == nil {
		t.Fatal("expected CreateProposal to reject draft dependency cycle")
	}
	if proposal != nil {
		t.Fatalf("proposal = %+v, want nil", proposal)
	}
}

func TestServiceSubmitRejectsUnknownProject(t *testing.T) {
	store := newProposalServiceTestStore(t)
	svc := New(Config{Store: store, Tx: proposalTx{base: store}})
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "unknown project", Status: core.ThreadActive, OwnerID: "user-1"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	missingProjectID := int64(999)
	proposal, err := svc.CreateProposal(ctx, CreateProposalInput{
		ThreadID: threadID,
		Title:    "无效项目",
		WorkItemDrafts: []core.ProposalWorkItemDraft{
			{TempID: "draft-a", Title: "任务 A", ProjectID: &missingProjectID},
		},
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if _, err := svc.Submit(ctx, proposal.ID); err == nil {
		t.Fatal("expected Submit to fail for unknown project_id")
	}
}
