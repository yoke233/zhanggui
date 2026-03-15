package threadapp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

type runtimeStub struct {
	err          error
	calls        int
	lastThreadID int64
}

func (r *runtimeStub) CleanupThread(_ context.Context, threadID int64) error {
	r.calls++
	r.lastThreadID = threadID
	return r.err
}

type workspaceStub struct {
	ensureCalls []int64
	syncCalls   []int64
	err         error
}

func (w *workspaceStub) EnsureThreadWorkspace(_ context.Context, threadID int64) error {
	w.ensureCalls = append(w.ensureCalls, threadID)
	return w.err
}

func (w *workspaceStub) SyncThreadWorkspaceContext(_ context.Context, threadID int64) error {
	w.syncCalls = append(w.syncCalls, threadID)
	return w.err
}

type sqliteTxAdapter struct {
	base core.TransactionalStore
	wrap func(core.Store) (TxStore, error)
}

func (a sqliteTxAdapter) InTx(ctx context.Context, fn func(ctx context.Context, store TxStore) error) error {
	return a.base.InTx(ctx, func(store core.Store) error {
		txStore, err := a.bind(store)
		if err != nil {
			return err
		}
		return fn(ctx, txStore)
	})
}

func (a sqliteTxAdapter) bind(store core.Store) (TxStore, error) {
	if a.wrap != nil {
		return a.wrap(store)
	}
	txStore, ok := store.(TxStore)
	if !ok {
		return nil, fmt.Errorf("unexpected tx store type %T", store)
	}
	return txStore, nil
}

type failingLinkStore struct {
	*sqlite.Store
	failCreateLink bool
}

func (s *failingLinkStore) CreateThreadWorkItemLink(ctx context.Context, link *core.ThreadWorkItemLink) (int64, error) {
	if s.failCreateLink {
		return 0, errors.New("create link failed")
	}
	return s.Store.CreateThreadWorkItemLink(ctx, link)
}

type failingDeleteStore struct {
	*sqlite.Store
	failDeleteMembers  bool
	failDeleteMessages bool
	failDeleteContexts bool
}

func (s *failingDeleteStore) DeleteThreadMembersByThread(ctx context.Context, threadID int64) error {
	if s.failDeleteMembers {
		return errors.New("delete members failed")
	}
	return s.Store.DeleteThreadMembersByThread(ctx, threadID)
}

func (s *failingDeleteStore) DeleteThreadMessagesByThread(ctx context.Context, threadID int64) error {
	if s.failDeleteMessages {
		return errors.New("delete messages failed")
	}
	return s.Store.DeleteThreadMessagesByThread(ctx, threadID)
}

func (s *failingDeleteStore) DeleteThreadContextRefsByThread(ctx context.Context, threadID int64) error {
	if s.failDeleteContexts {
		return errors.New("delete contexts failed")
	}
	return s.Store.DeleteThreadContextRefsByThread(ctx, threadID)
}

type failingRefStore struct {
	*sqlite.Store
	failCreateRef bool
	failUpdateRef bool
	failDeleteRef bool
}

func (s *failingRefStore) CreateThreadContextRef(ctx context.Context, ref *core.ThreadContextRef) (int64, error) {
	if s.failCreateRef {
		return 0, errors.New("create ref failed")
	}
	return s.Store.CreateThreadContextRef(ctx, ref)
}

func (s *failingRefStore) UpdateThreadContextRef(ctx context.Context, ref *core.ThreadContextRef) error {
	if s.failUpdateRef {
		return core.ErrNotFound
	}
	return s.Store.UpdateThreadContextRef(ctx, ref)
}

func (s *failingRefStore) DeleteThreadContextRef(ctx context.Context, id int64) error {
	if s.failDeleteRef {
		return core.ErrNotFound
	}
	return s.Store.DeleteThreadContextRef(ctx, id)
}

type failingCreateThreadStore struct {
	*sqlite.Store
	failCreateThread bool
	failAddMember    bool
	failCreateItem   bool
	failDeleteItem   bool
	failDeleteThread bool
}

func (s *failingCreateThreadStore) CreateThread(ctx context.Context, thread *core.Thread) (int64, error) {
	if s.failCreateThread {
		return 0, errors.New("create thread failed")
	}
	return s.Store.CreateThread(ctx, thread)
}

func (s *failingCreateThreadStore) AddThreadMember(ctx context.Context, member *core.ThreadMember) (int64, error) {
	if s.failAddMember {
		return 0, errors.New("add member failed")
	}
	return s.Store.AddThreadMember(ctx, member)
}

func (s *failingCreateThreadStore) CreateWorkItem(ctx context.Context, workItem *core.WorkItem) (int64, error) {
	if s.failCreateItem {
		return 0, errors.New("create work item failed")
	}
	return s.Store.CreateWorkItem(ctx, workItem)
}

func (s *failingCreateThreadStore) DeleteWorkItem(ctx context.Context, id int64) error {
	if s.failDeleteItem {
		return errors.New("delete work item failed")
	}
	return s.Store.DeleteWorkItem(ctx, id)
}

func (s *failingCreateThreadStore) DeleteThread(ctx context.Context, id int64) error {
	if s.failDeleteThread {
		return core.ErrNotFound
	}
	return s.Store.DeleteThread(ctx, id)
}

type failingLinkCleanupStore struct {
	*failingCreateThreadStore
}

func (s *failingLinkCleanupStore) CreateThreadWorkItemLink(context.Context, *core.ThreadWorkItemLink) (int64, error) {
	return 0, errors.New("create link failed")
}

type failingGetRefStore struct {
	*sqlite.Store
	failGetRef bool
}

func (s *failingGetRefStore) GetThreadContextRef(ctx context.Context, id int64) (*core.ThreadContextRef, error) {
	if s.failGetRef {
		return nil, errors.New("get ref failed")
	}
	return s.Store.GetThreadContextRef(ctx, id)
}

func newThreadAppTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "threadapp-test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newSQLiteThreadAppService(store Store, tx Tx, runtime Runtime) *Service {
	return New(Config{
		Store:   store,
		Tx:      tx,
		Runtime: runtime,
	})
}

func newSQLiteThreadAppServiceWithWorkspace(store Store, tx Tx, runtime Runtime, workspace WorkspaceManager) *Service {
	return New(Config{
		Store:     store,
		Tx:        tx,
		Runtime:   runtime,
		Workspace: workspace,
	})
}

func newSQLiteTxAdapter(store *sqlite.Store, wrap func(core.Store) (TxStore, error)) Tx {
	return sqliteTxAdapter{
		base: store,
		wrap: wrap,
	}
}

func createThreadFixture(t *testing.T, store *sqlite.Store, withMessage bool, withLink bool) (threadID int64, workItemID int64) {
	t.Helper()
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{
		Title:   "fixture-thread",
		Summary: "fixture summary",
		Status:  core.ThreadActive,
		OwnerID: "owner-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := store.AddThreadMember(ctx, &core.ThreadMember{
		ThreadID: threadID,
		Kind:     core.ThreadMemberKindHuman,
		UserID:   "owner-1",
		Role:     "owner",
	}); err != nil {
		t.Fatalf("add owner member: %v", err)
	}
	if _, err := store.AddThreadMember(ctx, &core.ThreadMember{
		ThreadID: threadID,
		Kind:     core.ThreadMemberKindHuman,
		UserID:   "member-2",
		Role:     "member",
	}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if withMessage {
		if _, err := store.CreateThreadMessage(ctx, &core.ThreadMessage{
			ThreadID: threadID,
			SenderID: "owner-1",
			Role:     "human",
			Content:  "hello",
		}); err != nil {
			t.Fatalf("create thread message: %v", err)
		}
	}
	if withLink {
		workItemID, err = store.CreateWorkItem(ctx, &core.WorkItem{
			Title:    "linked work item",
			Body:     "linked body",
			Status:   core.WorkItemOpen,
			Priority: core.PriorityMedium,
		})
		if err != nil {
			t.Fatalf("create work item: %v", err)
		}
		if _, err := store.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{
			ThreadID:     threadID,
			WorkItemID:   workItemID,
			RelationType: "related",
			IsPrimary:    true,
		}); err != nil {
			t.Fatalf("create link: %v", err)
		}
	}
	return threadID, workItemID
}

func TestServiceCreateWorkItemFromThreadUsesSummaryAndCreatesPrimaryLink(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	thread := &core.Thread{Title: "thread-1", Summary: "Ship the feature from summary."}
	threadID, err := store.CreateThread(ctx, thread)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	thread.ID = threadID

	result, err := svc.CreateWorkItemFromThread(ctx, CreateWorkItemFromThreadInput{
		ThreadID:      thread.ID,
		WorkItemTitle: "spawned work item",
	})
	if err != nil {
		t.Fatalf("CreateWorkItemFromThread: %v", err)
	}
	if result.Thread == nil || result.Thread.ID != thread.ID {
		t.Fatalf("unexpected thread result: %+v", result.Thread)
	}
	if result.WorkItem == nil {
		t.Fatal("expected work item result")
	}
	if result.WorkItem.Body != "Ship the feature from summary." {
		t.Fatalf("expected summary-backed body, got %q", result.WorkItem.Body)
	}
	if result.WorkItem.Metadata["source_type"] != "thread_summary" {
		t.Fatalf("expected source_type thread_summary, got %#v", result.WorkItem.Metadata["source_type"])
	}
	if result.Link == nil || result.Link.ThreadID != thread.ID || result.Link.WorkItemID != result.WorkItem.ID {
		t.Fatalf("unexpected link result: %+v", result.Link)
	}
	if !result.Link.IsPrimary || result.Link.RelationType != "drives" {
		t.Fatalf("unexpected link metadata: %+v", result.Link)
	}

	links, err := store.ListWorkItemsByThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("list links by thread: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 persisted link, got %d", len(links))
	}
}

func TestServiceCreateWorkItemFromThreadExplicitBodyMarksManualSource(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "thread-2"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	result, err := svc.CreateWorkItemFromThread(ctx, CreateWorkItemFromThreadInput{
		ThreadID:      threadID,
		WorkItemTitle: "spawned work item",
		WorkItemBody:  "Manual body from request.",
	})
	if err != nil {
		t.Fatalf("CreateWorkItemFromThread: %v", err)
	}
	if result.WorkItem == nil {
		t.Fatal("expected work item result")
	}
	if result.WorkItem.Body != "Manual body from request." {
		t.Fatalf("unexpected work item body: %q", result.WorkItem.Body)
	}
	if result.WorkItem.Metadata["source_type"] != "thread_manual" {
		t.Fatalf("expected source_type thread_manual, got %#v", result.WorkItem.Metadata["source_type"])
	}
	if result.WorkItem.Metadata["body_from_summary"] != false {
		t.Fatalf("expected body_from_summary false, got %#v", result.WorkItem.Metadata["body_from_summary"])
	}
}

func TestServiceCreateWorkItemFromThreadRequiresSummaryWhenBodyMissing(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "thread-3"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	_, err = svc.CreateWorkItemFromThread(ctx, CreateWorkItemFromThreadInput{
		ThreadID:      threadID,
		WorkItemTitle: "spawned work item",
	})
	if CodeOf(err) != CodeMissingThreadSummary {
		t.Fatalf("expected %s, got %v", CodeMissingThreadSummary, err)
	}
}

func TestServiceCreateWorkItemFromThreadRollsBackOnLinkFailure(t *testing.T) {
	base := newThreadAppTestStore(t)
	store := &failingLinkStore{Store: base, failCreateLink: true}
	tx := newSQLiteTxAdapter(base, func(txStore core.Store) (TxStore, error) {
		sqliteStore, ok := txStore.(*sqlite.Store)
		if !ok {
			return nil, fmt.Errorf("unexpected tx store type %T", txStore)
		}
		return &failingLinkStore{Store: sqliteStore, failCreateLink: true}, nil
	})
	svc := newSQLiteThreadAppService(store, tx, nil)
	ctx := context.Background()

	threadID, err := base.CreateThread(ctx, &core.Thread{
		Title:   "thread-4",
		Summary: "Use me as the body.",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	_, err = svc.CreateWorkItemFromThread(ctx, CreateWorkItemFromThreadInput{
		ThreadID:      threadID,
		WorkItemTitle: "spawned work item",
	})
	if err == nil {
		t.Fatal("expected create work item from thread to fail")
	}

	items, err := base.ListWorkItems(ctx, core.WorkItemFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list work items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 work items after rollback, got %d", len(items))
	}

	links, err := base.ListWorkItemsByThread(ctx, threadID)
	if err != nil {
		t.Fatalf("list thread links: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected 0 links after rollback, got %d", len(links))
	}
}

func TestServiceDeleteThreadFailsFastWhenRuntimeCleanupFails(t *testing.T) {
	store := newThreadAppTestStore(t)
	threadID, _ := createThreadFixture(t, store, true, true)
	runtime := &runtimeStub{err: errors.New("cleanup failed")}
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), runtime)
	ctx := context.Background()

	err := svc.DeleteThread(ctx, threadID)
	if CodeOf(err) != CodeCleanupThreadFailed {
		t.Fatalf("expected %s, got %v", CodeCleanupThreadFailed, err)
	}
	if runtime.calls != 1 || runtime.lastThreadID != threadID {
		t.Fatalf("unexpected runtime cleanup call state: %+v", runtime)
	}

	if _, err := store.GetThread(ctx, threadID); err != nil {
		t.Fatalf("expected thread to remain after runtime cleanup failure: %v", err)
	}
	messages, err := store.ListThreadMessages(ctx, threadID, 20, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected messages to remain, got %d", len(messages))
	}
	members, err := store.ListThreadMembers(ctx, threadID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected members to remain, got %d", len(members))
	}
	links, err := store.ListWorkItemsByThread(ctx, threadID)
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected links to remain, got %d", len(links))
	}
}

func TestServiceDeleteThreadRollsBackWhenAggregateDeleteFails(t *testing.T) {
	base := newThreadAppTestStore(t)
	threadID, _ := createThreadFixture(t, base, true, true)
	store := &failingDeleteStore{Store: base, failDeleteMembers: true}
	tx := newSQLiteTxAdapter(base, func(txStore core.Store) (TxStore, error) {
		sqliteStore, ok := txStore.(*sqlite.Store)
		if !ok {
			return nil, fmt.Errorf("unexpected tx store type %T", txStore)
		}
		return &failingDeleteStore{Store: sqliteStore, failDeleteMembers: true}, nil
	})
	svc := newSQLiteThreadAppService(store, tx, nil)
	ctx := context.Background()

	err := svc.DeleteThread(ctx, threadID)
	if err == nil {
		t.Fatal("expected delete thread to fail")
	}

	if _, err := base.GetThread(ctx, threadID); err != nil {
		t.Fatalf("expected thread to remain after rollback: %v", err)
	}
	messages, err := base.ListThreadMessages(ctx, threadID, 20, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected messages rollback, got %d", len(messages))
	}
	members, err := base.ListThreadMembers(ctx, threadID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected members rollback, got %d", len(members))
	}
	links, err := base.ListWorkItemsByThread(ctx, threadID)
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected links rollback, got %d", len(links))
	}
}

func TestServiceCrystallizeChatSessionCreatesThreadOnly(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	result, err := svc.CrystallizeChatSession(ctx, CrystallizeChatSessionInput{
		SessionID:          "chat-1",
		ThreadTitle:        "Design API shape",
		ThreadSummary:      "Discuss API structure",
		OwnerID:            "owner-1",
		ParticipantUserIDs: []string{"owner-1", "member-2"},
	})
	if err != nil {
		t.Fatalf("CrystallizeChatSession: %v", err)
	}
	if result.Thread == nil || result.Thread.ID == 0 {
		t.Fatalf("expected persisted thread, got %+v", result.Thread)
	}
	if result.WorkItem != nil {
		t.Fatalf("expected no work item, got %+v", result.WorkItem)
	}
	if result.Thread.Metadata["source_chat_session_id"] != "chat-1" {
		t.Fatalf("unexpected thread metadata: %#v", result.Thread.Metadata)
	}
	if len(result.Participants) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(result.Participants))
	}
	members, err := store.ListThreadMembers(ctx, result.Thread.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 persisted members, got %d", len(members))
	}
	items, err := store.ListWorkItems(ctx, core.WorkItemFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list work items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no work items, got %d", len(items))
	}
}

func TestServiceCrystallizeChatSessionCreatesThreadAndWorkItem(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	result, err := svc.CrystallizeChatSession(ctx, CrystallizeChatSessionInput{
		SessionID:      "chat-2",
		ThreadTitle:    "Ship feature",
		ThreadSummary:  "Use summary as work item body",
		OwnerID:        "owner-1",
		CreateWorkItem: true,
		WorkItemTitle:  "Implement feature",
	})
	if err != nil {
		t.Fatalf("CrystallizeChatSession: %v", err)
	}
	if result.Thread == nil || result.Thread.ID == 0 {
		t.Fatalf("expected thread result, got %+v", result.Thread)
	}
	if result.WorkItem == nil || result.WorkItem.ID == 0 {
		t.Fatalf("expected work item result, got %+v", result.WorkItem)
	}
	if result.WorkItem.Body != "Use summary as work item body" {
		t.Fatalf("expected summary-backed work item body, got %q", result.WorkItem.Body)
	}
	links, err := store.ListWorkItemsByThread(ctx, result.Thread.ID)
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 persisted link, got %d", len(links))
	}
	if !links[0].IsPrimary || links[0].RelationType != "drives" {
		t.Fatalf("unexpected link: %+v", links[0])
	}
}

func TestServiceCrystallizeChatSessionRollsBackWhenLinkCreationFails(t *testing.T) {
	base := newThreadAppTestStore(t)
	store := &failingLinkStore{Store: base, failCreateLink: true}
	tx := newSQLiteTxAdapter(base, func(txStore core.Store) (TxStore, error) {
		sqliteStore, ok := txStore.(*sqlite.Store)
		if !ok {
			return nil, fmt.Errorf("unexpected tx store type %T", txStore)
		}
		return &failingLinkStore{Store: sqliteStore, failCreateLink: true}, nil
	})
	svc := newSQLiteThreadAppService(store, tx, nil)
	ctx := context.Background()

	_, err := svc.CrystallizeChatSession(ctx, CrystallizeChatSessionInput{
		SessionID:      "chat-3",
		ThreadTitle:    "Broken materialization",
		ThreadSummary:  "summary body",
		OwnerID:        "owner-1",
		CreateWorkItem: true,
		WorkItemTitle:  "Should rollback",
	})
	if err == nil {
		t.Fatal("expected crystallize chat session to fail")
	}

	threads, err := base.ListThreads(ctx, core.ThreadFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(threads) != 0 {
		t.Fatalf("expected 0 threads after rollback, got %d", len(threads))
	}
	items, err := base.ListWorkItems(ctx, core.WorkItemFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list work items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 work items after rollback, got %d", len(items))
	}
}

func TestServiceCreateThreadSyncsWorkspace(t *testing.T) {
	store := newThreadAppTestStore(t)
	workspace := &workspaceStub{}
	svc := newSQLiteThreadAppServiceWithWorkspace(store, newSQLiteTxAdapter(store, nil), nil, workspace)

	result, err := svc.CreateThread(context.Background(), CreateThreadInput{
		Title:   "workspace-thread",
		OwnerID: "owner-1",
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if result.Thread == nil || result.Thread.ID == 0 {
		t.Fatalf("expected created thread, got %+v", result.Thread)
	}
	if len(workspace.ensureCalls) != 1 || workspace.ensureCalls[0] != result.Thread.ID {
		t.Fatalf("unexpected workspace ensure calls: %+v", workspace.ensureCalls)
	}
	if len(workspace.syncCalls) != 1 || workspace.syncCalls[0] != result.Thread.ID {
		t.Fatalf("unexpected workspace sync calls: %+v", workspace.syncCalls)
	}
}

func TestServiceCreateThreadRollsBackWhenWorkspaceSyncFails(t *testing.T) {
	store := newThreadAppTestStore(t)
	workspace := &workspaceStub{err: errors.New("sync failed")}
	svc := newSQLiteThreadAppServiceWithWorkspace(store, newSQLiteTxAdapter(store, nil), nil, workspace)

	_, err := svc.CreateThread(context.Background(), CreateThreadInput{
		Title:   "workspace-fail-thread",
		OwnerID: "owner-1",
	})
	if err == nil {
		t.Fatal("expected CreateThread to fail when workspace sync fails")
	}

	threads, err := store.ListThreads(context.Background(), core.ThreadFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(threads) != 0 {
		t.Fatalf("expected thread rollback after workspace failure, got %d threads", len(threads))
	}
}

func TestServiceCreateThreadContextRef(t *testing.T) {
	store := newThreadAppTestStore(t)
	workspace := &workspaceStub{}
	svc := newSQLiteThreadAppServiceWithWorkspace(store, newSQLiteTxAdapter(store, nil), nil, workspace)
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "context-thread"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	projectID, err := store.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   t.TempDir(),
		Label:     "workspace",
	}); err != nil {
		t.Fatalf("create resource space: %v", err)
	}

	ref, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{
		ThreadID:  threadID,
		ProjectID: projectID,
		Access:    "check",
		Note:      "run checks",
		GrantedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateThreadContextRef: %v", err)
	}
	if ref.Access != core.ContextAccessCheck || ref.ProjectID != projectID {
		t.Fatalf("unexpected context ref: %+v", ref)
	}
	if len(workspace.syncCalls) != 1 || workspace.syncCalls[0] != threadID {
		t.Fatalf("expected workspace sync for thread %d, got %+v", threadID, workspace.syncCalls)
	}
	thread, err := store.GetThread(ctx, threadID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if focusProjectID, ok := core.ReadThreadFocusProjectID(thread); !ok || focusProjectID != projectID {
		t.Fatalf("expected focus project %d, got (%d, %v)", projectID, focusProjectID, ok)
	}
}

func TestServiceCreateThreadContextRefRollsBackWhenWorkspaceSyncFails(t *testing.T) {
	store := newThreadAppTestStore(t)
	workspace := &workspaceStub{err: errors.New("sync failed")}
	svc := newSQLiteThreadAppServiceWithWorkspace(store, newSQLiteTxAdapter(store, nil), nil, workspace)
	ctx := context.Background()

	threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "context-thread"})
	projectID, _ := store.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
	if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   t.TempDir(),
		Label:     "workspace",
	}); err != nil {
		t.Fatalf("create resource space: %v", err)
	}

	_, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{
		ThreadID:  threadID,
		ProjectID: projectID,
		Access:    "read",
	})
	if err == nil {
		t.Fatal("expected CreateThreadContextRef to fail when workspace sync fails")
	}

	refs, err := store.ListThreadContextRefs(ctx, threadID)
	if err != nil {
		t.Fatalf("list context refs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected context ref rollback, got %+v", refs)
	}
}

func TestServiceCreateThreadContextRefRejectsDuplicate(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "context-thread"})
	projectID, _ := store.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
	if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   t.TempDir(),
		Label:     "workspace",
	}); err != nil {
		t.Fatalf("create resource space: %v", err)
	}
	if _, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{
		ThreadID:  threadID,
		ProjectID: projectID,
		Access:    "read",
	}); err != nil {
		t.Fatalf("create first context ref: %v", err)
	}
	_, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{
		ThreadID:  threadID,
		ProjectID: projectID,
		Access:    "check",
	})
	if CodeOf(err) != CodeContextRefConflict {
		t.Fatalf("expected %s, got %v", CodeContextRefConflict, err)
	}
}

func TestServiceUpdateThreadContextRefRejectsInvalidAccess(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "context-thread"})
	projectID, _ := store.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
	if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   t.TempDir(),
		Label:     "workspace",
	}); err != nil {
		t.Fatalf("create resource space: %v", err)
	}
	refID, err := store.CreateThreadContextRef(ctx, &core.ThreadContextRef{
		ThreadID:  threadID,
		ProjectID: projectID,
		Access:    core.ContextAccessRead,
	})
	if err != nil {
		t.Fatalf("create context ref: %v", err)
	}

	_, err = svc.UpdateThreadContextRef(ctx, UpdateThreadContextRefInput{
		ThreadID: threadID,
		RefID:    refID,
		Access:   "broken",
	})
	if CodeOf(err) != CodeInvalidContextAccess {
		t.Fatalf("expected %s, got %v", CodeInvalidContextAccess, err)
	}
}

func TestServiceDeleteThreadRemovesContextRefs(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "context-cleanup"})
	projectID, _ := store.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
	if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   t.TempDir(),
	}); err != nil {
		t.Fatalf("create resource space: %v", err)
	}
	if _, err := store.CreateThreadContextRef(ctx, &core.ThreadContextRef{
		ThreadID:  threadID,
		ProjectID: projectID,
		Access:    core.ContextAccessRead,
	}); err != nil {
		t.Fatalf("create context ref: %v", err)
	}

	if err := svc.DeleteThread(ctx, threadID); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}

	refs, err := store.ListThreadContextRefs(ctx, threadID)
	if err != nil {
		t.Fatalf("list context refs after delete: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected context refs to be removed, got %+v", refs)
	}
}

func TestServiceDeleteThreadContextRefNotFound(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "context-thread"})
	err := svc.DeleteThreadContextRef(ctx, threadID, 9999)
	if CodeOf(err) != CodeContextRefNotFound {
		t.Fatalf("expected %s, got %v", CodeContextRefNotFound, err)
	}
}

func TestServiceDeleteThreadContextRefReconcilesFocus(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "context-thread"})
	projectA, _ := store.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
	projectB, _ := store.CreateProject(ctx, &core.Project{Name: "Project Beta", Kind: core.ProjectGeneral})
	for _, projectID := range []int64{projectA, projectB} {
		if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
			ProjectID: projectID,
			Kind:      core.ResourceKindLocalFS,
			RootURI:   t.TempDir(),
		}); err != nil {
			t.Fatalf("create resource space: %v", err)
		}
	}

	refA, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{ThreadID: threadID, ProjectID: projectA, Access: "read"})
	if err != nil {
		t.Fatalf("create ref A: %v", err)
	}
	if _, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{ThreadID: threadID, ProjectID: projectB, Access: "read"}); err != nil {
		t.Fatalf("create ref B: %v", err)
	}
	thread, err := store.GetThread(ctx, threadID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if focusProjectID, ok := core.ReadThreadFocusProjectID(thread); !ok || focusProjectID != projectB {
		t.Fatalf("expected focus to move to latest project %d, got (%d, %v)", projectB, focusProjectID, ok)
	}

	if err := svc.DeleteThreadContextRef(ctx, threadID, refA.ID); err != nil {
		t.Fatalf("DeleteThreadContextRef(non-focused): %v", err)
	}
	thread, _ = store.GetThread(ctx, threadID)
	if focusProjectID, ok := core.ReadThreadFocusProjectID(thread); !ok || focusProjectID != projectB {
		t.Fatalf("expected focus to remain on project %d, got (%d, %v)", projectB, focusProjectID, ok)
	}

	refs, err := store.ListThreadContextRefs(ctx, threadID)
	if err != nil || len(refs) != 1 {
		t.Fatalf("list refs: refs=%+v err=%v", refs, err)
	}
	if err := svc.DeleteThreadContextRef(ctx, threadID, refs[0].ID); err != nil {
		t.Fatalf("DeleteThreadContextRef(focused): %v", err)
	}
	thread, _ = store.GetThread(ctx, threadID)
	if _, ok := core.ReadThreadFocusProjectID(thread); ok {
		t.Fatalf("expected focus to be cleared after deleting last ref, got %+v", thread.Metadata)
	}
}

func TestServiceCreateThreadContextRefRequiresResolvableBinding(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "context-thread"})
	projectID, _ := store.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})

	_, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{
		ThreadID:  threadID,
		ProjectID: projectID,
		Access:    "read",
	})
	if err == nil {
		t.Fatal("expected CreateThreadContextRef to fail without resolvable binding")
	}
}

func TestServiceHelpersAndValidationBranches(t *testing.T) {
	t.Run("new clones config", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		runtime := &runtimeStub{}
		workspace := &workspaceStub{}
		svc := New(Config{Store: store, Runtime: runtime, Workspace: workspace})
		if svc.store != store || svc.runtime != runtime || svc.workspace != workspace {
			t.Fatalf("New() did not keep config dependencies: %+v", svc)
		}
	})

	t.Run("create thread requires title", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
		if _, err := svc.CreateThread(context.Background(), CreateThreadInput{OwnerID: "owner"}); CodeOf(err) != CodeMissingTitle {
			t.Fatalf("expected %s, got %v", CodeMissingTitle, err)
		}
	})

	t.Run("build participants trims and deduplicates", func(t *testing.T) {
		got := buildParticipants(" owner ", []string{"owner", " member ", "", "member"})
		if len(got) != 2 || got[0].Role != "owner" || got[1].UserID != "member" {
			t.Fatalf("unexpected participants: %+v", got)
		}
	})

	t.Run("clone metadata", func(t *testing.T) {
		original := map[string]any{"a": "b"}
		cloned := cloneMetadata(original)
		cloned["a"] = "changed"
		if original["a"] != "b" {
			t.Fatalf("expected cloneMetadata to copy map, got %+v", original)
		}
		if cloneMetadata(nil) != nil {
			t.Fatal("expected nil metadata clone to stay nil")
		}
	})

	t.Run("sync thread workspace handles nils", func(t *testing.T) {
		if err := (*Service)(nil).syncThreadWorkspace(context.Background(), 1); err != nil {
			t.Fatalf("nil service syncThreadWorkspace error = %v", err)
		}
		svc := &Service{}
		if err := svc.syncThreadWorkspace(context.Background(), 1); err != nil {
			t.Fatalf("nil workspace syncThreadWorkspace error = %v", err)
		}
	})
}

func TestServiceLinkAndUnlinkWorkItemBranches(t *testing.T) {
	store := newThreadAppTestStore(t)
	svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "thread", OwnerID: "owner"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{Title: "item", Status: core.WorkItemOpen, Priority: core.PriorityMedium})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}

	if _, err := svc.LinkThreadWorkItem(ctx, LinkThreadWorkItemInput{ThreadID: 999, WorkItemID: workItemID}); CodeOf(err) != CodeThreadNotFound {
		t.Fatalf("expected %s, got %v", CodeThreadNotFound, err)
	}
	if _, err := svc.LinkThreadWorkItem(ctx, LinkThreadWorkItemInput{ThreadID: threadID}); CodeOf(err) != CodeMissingWorkItemID {
		t.Fatalf("expected %s, got %v", CodeMissingWorkItemID, err)
	}
	if _, err := svc.LinkThreadWorkItem(ctx, LinkThreadWorkItemInput{ThreadID: threadID, WorkItemID: 999}); CodeOf(err) != CodeWorkItemNotFound {
		t.Fatalf("expected %s, got %v", CodeWorkItemNotFound, err)
	}

	link, err := svc.LinkThreadWorkItem(ctx, LinkThreadWorkItemInput{ThreadID: threadID, WorkItemID: workItemID})
	if err != nil {
		t.Fatalf("LinkThreadWorkItem: %v", err)
	}
	if link.RelationType != "related" {
		t.Fatalf("expected default relation type, got %+v", link)
	}

	if err := svc.UnlinkThreadWorkItem(ctx, threadID, workItemID); err != nil {
		t.Fatalf("UnlinkThreadWorkItem: %v", err)
	}
	if err := svc.UnlinkThreadWorkItem(ctx, threadID, workItemID); CodeOf(err) != CodeLinkNotFound {
		t.Fatalf("expected %s, got %v", CodeLinkNotFound, err)
	}
}

func TestServiceDeleteThreadAndContextRefBranches(t *testing.T) {
	t.Run("delete thread not found", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
		if err := svc.DeleteThread(context.Background(), 999); CodeOf(err) != CodeThreadNotFound {
			t.Fatalf("expected %s, got %v", CodeThreadNotFound, err)
		}
	})

	t.Run("delete thread context ref mismatched thread", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
		ctx := context.Background()

		threadA, _ := store.CreateThread(ctx, &core.Thread{Title: "a"})
		threadB, _ := store.CreateThread(ctx, &core.Thread{Title: "b"})
		projectID, _ := store.CreateProject(ctx, &core.Project{Name: "Project", Kind: core.ProjectGeneral})
		if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
			ProjectID: projectID,
			Kind:      core.ResourceKindLocalFS,
			RootURI:   t.TempDir(),
		}); err != nil {
			t.Fatalf("create resource space: %v", err)
		}
		refID, err := store.CreateThreadContextRef(ctx, &core.ThreadContextRef{
			ThreadID:  threadA,
			ProjectID: projectID,
			Access:    core.ContextAccessRead,
		})
		if err != nil {
			t.Fatalf("create context ref: %v", err)
		}

		if err := svc.DeleteThreadContextRef(ctx, threadB, refID); CodeOf(err) != CodeContextRefNotFound {
			t.Fatalf("expected %s, got %v", CodeContextRefNotFound, err)
		}
	})
}

func TestServiceUpdateContextRefAndWorkItemHelpers(t *testing.T) {
	t.Run("update context ref trims note and granted by", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		workspace := &workspaceStub{}
		svc := newSQLiteThreadAppServiceWithWorkspace(store, newSQLiteTxAdapter(store, nil), nil, workspace)
		ctx := context.Background()

		threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "context-thread"})
		projectID, _ := store.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
		if _, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
			ProjectID: projectID,
			Kind:      core.ResourceKindLocalFS,
			RootURI:   t.TempDir(),
		}); err != nil {
			t.Fatalf("create resource space: %v", err)
		}
		refID, err := store.CreateThreadContextRef(ctx, &core.ThreadContextRef{
			ThreadID:  threadID,
			ProjectID: projectID,
			Access:    core.ContextAccessRead,
			Note:      "old",
			GrantedBy: "old-user",
		})
		if err != nil {
			t.Fatalf("create context ref: %v", err)
		}

		note := "  new note  "
		ref, err := svc.UpdateThreadContextRef(ctx, UpdateThreadContextRefInput{
			ThreadID:  threadID,
			RefID:     refID,
			Access:    "check",
			Note:      &note,
			GrantedBy: "  user-2  ",
		})
		if err != nil {
			t.Fatalf("UpdateThreadContextRef: %v", err)
		}
		if ref.Access != core.ContextAccessCheck || ref.Note != "new note" || ref.GrantedBy != "user-2" {
			t.Fatalf("unexpected updated ref: %+v", ref)
		}
	})

	t.Run("update context ref missing thread/ref", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
		ctx := context.Background()

		if _, err := svc.UpdateThreadContextRef(ctx, UpdateThreadContextRefInput{ThreadID: 999, RefID: 1, Access: "read"}); CodeOf(err) != CodeThreadNotFound {
			t.Fatalf("expected %s, got %v", CodeOf(err), err)
		}
		threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "context-thread"})
		if _, err := svc.UpdateThreadContextRef(ctx, UpdateThreadContextRefInput{ThreadID: threadID, RefID: 999, Access: "read"}); CodeOf(err) != CodeContextRefNotFound {
			t.Fatalf("expected %s, got %v", CodeContextRefNotFound, err)
		}
	})

	t.Run("create linked work item helper validation", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		ctx := context.Background()
		if _, _, err := createLinkedWorkItemFromThreadData(ctx, store, nil, "title", "body", nil); err == nil {
			t.Fatal("expected nil thread helper to fail")
		}
		thread := &core.Thread{ID: 1, Summary: "summary"}
		if _, _, err := createLinkedWorkItemFromThreadData(ctx, store, thread, " ", "body", nil); CodeOf(err) != CodeMissingTitle {
			t.Fatalf("expected %s, got %v", CodeMissingTitle, err)
		}
	})

	t.Run("delete thread aggregate data not found maps error", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		err := deleteThreadAggregateData(context.Background(), store, 999)
		if CodeOf(err) != CodeThreadNotFound {
			t.Fatalf("expected %s, got %v", CodeThreadNotFound, err)
		}
	})
}

func TestServiceErrorAndAggregateFailureBranches(t *testing.T) {
	t.Run("error helpers", func(t *testing.T) {
		if (*Error)(nil).Error() != "" {
			t.Fatal("expected nil Error() to return empty string")
		}
		errWithMessage := &Error{Code: "X", Message: "message"}
		if errWithMessage.Error() != "message" {
			t.Fatalf("Error() = %q", errWithMessage.Error())
		}
		wrapped := errors.New("wrapped")
		errWithWrap := &Error{Code: "X", Err: wrapped}
		if errWithWrap.Error() != "wrapped" || !errors.Is(errWithWrap, wrapped) {
			t.Fatalf("unexpected wrapped error behaviour: %+v", errWithWrap)
		}
		errWithCode := &Error{Code: "ONLY_CODE"}
		if errWithCode.Error() != "ONLY_CODE" {
			t.Fatalf("Error() = %q", errWithCode.Error())
		}
		if CodeOf(errors.New("plain")) != "" {
			t.Fatal("expected CodeOf on plain error to be empty")
		}
		if (*Error)(nil).Unwrap() != nil {
			t.Fatal("expected nil Unwrap() to return nil")
		}
	})

	t.Run("create thread context ref missing inputs", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
		ctx := context.Background()

		if _, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{ThreadID: 999, ProjectID: 1, Access: "read"}); CodeOf(err) != CodeThreadNotFound {
			t.Fatalf("expected %s, got %v", CodeThreadNotFound, err)
		}
		threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "thread"})
		if _, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{ThreadID: threadID, Access: "read"}); CodeOf(err) != CodeMissingProjectID {
			t.Fatalf("expected %s, got %v", CodeMissingProjectID, err)
		}
		if _, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{ThreadID: threadID, ProjectID: 123, Access: "read"}); CodeOf(err) != CodeProjectNotFound {
			t.Fatalf("expected %s, got %v", CodeProjectNotFound, err)
		}
	})

	t.Run("create thread context ref invalid access and create failure", func(t *testing.T) {
		base := newThreadAppTestStore(t)
		store := &failingRefStore{Store: base, failCreateRef: true}
		tx := newSQLiteTxAdapter(base, func(txStore core.Store) (TxStore, error) {
			sqliteStore, ok := txStore.(*sqlite.Store)
			if !ok {
				return nil, fmt.Errorf("unexpected tx store type %T", txStore)
			}
			return &failingRefStore{Store: sqliteStore, failCreateRef: true}, nil
		})
		svc := newSQLiteThreadAppService(store, tx, nil)
		ctx := context.Background()

		threadID, _ := base.CreateThread(ctx, &core.Thread{Title: "thread"})
		projectID, _ := base.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
		if _, err := base.CreateResourceSpace(ctx, &core.ResourceSpace{ProjectID: projectID, Kind: core.ResourceKindLocalFS, RootURI: t.TempDir()}); err != nil {
			t.Fatalf("create resource space: %v", err)
		}
		if _, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{ThreadID: threadID, ProjectID: projectID, Access: "bad"}); CodeOf(err) != CodeInvalidContextAccess {
			t.Fatalf("expected %s, got %v", CodeInvalidContextAccess, err)
		}
		if _, err := svc.CreateThreadContextRef(ctx, CreateThreadContextRefInput{ThreadID: threadID, ProjectID: projectID, Access: "read"}); err == nil || err.Error() != "create ref failed" {
			t.Fatalf("expected create ref failure, got %v", err)
		}
	})

	t.Run("update and delete thread context ref branches", func(t *testing.T) {
		base := newThreadAppTestStore(t)
		store := &failingRefStore{Store: base, failUpdateRef: true, failDeleteRef: true}
		tx := newSQLiteTxAdapter(base, nil)
		svc := newSQLiteThreadAppService(store, tx, nil)
		ctx := context.Background()

		threadA, _ := base.CreateThread(ctx, &core.Thread{Title: "a"})
		threadB, _ := base.CreateThread(ctx, &core.Thread{Title: "b"})
		projectID, _ := base.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
		if _, err := base.CreateResourceSpace(ctx, &core.ResourceSpace{ProjectID: projectID, Kind: core.ResourceKindLocalFS, RootURI: t.TempDir()}); err != nil {
			t.Fatalf("create resource space: %v", err)
		}
		refID, _ := base.CreateThreadContextRef(ctx, &core.ThreadContextRef{ThreadID: threadA, ProjectID: projectID, Access: core.ContextAccessRead})

		if _, err := svc.UpdateThreadContextRef(ctx, UpdateThreadContextRefInput{ThreadID: threadB, RefID: refID, Access: "read"}); CodeOf(err) != CodeContextRefNotFound {
			t.Fatalf("expected %s, got %v", CodeContextRefNotFound, err)
		}
		if _, err := svc.UpdateThreadContextRef(ctx, UpdateThreadContextRefInput{ThreadID: threadA, RefID: refID, Access: "read"}); CodeOf(err) != CodeContextRefNotFound {
			t.Fatalf("expected %s for update not found, got %v", CodeContextRefNotFound, err)
		}
		if err := svc.DeleteThreadContextRef(ctx, threadA, refID); CodeOf(err) != CodeContextRefNotFound {
			t.Fatalf("expected %s for delete not found, got %v", CodeContextRefNotFound, err)
		}
	})

	t.Run("delete thread context ref thread/ref/workspace branches", func(t *testing.T) {
		base := newThreadAppTestStore(t)
		workspace := &workspaceStub{err: errors.New("sync failed")}
		svc := newSQLiteThreadAppServiceWithWorkspace(base, newSQLiteTxAdapter(base, nil), nil, workspace)
		ctx := context.Background()

		if err := svc.DeleteThreadContextRef(ctx, 999, 1); CodeOf(err) != CodeThreadNotFound {
			t.Fatalf("expected %s, got %v", CodeThreadNotFound, err)
		}
		threadID, _ := base.CreateThread(ctx, &core.Thread{Title: "thread"})
		if err := svc.DeleteThreadContextRef(ctx, threadID, 999); CodeOf(err) != CodeContextRefNotFound {
			t.Fatalf("expected %s, got %v", CodeContextRefNotFound, err)
		}

		projectID, _ := base.CreateProject(ctx, &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
		if _, err := base.CreateResourceSpace(ctx, &core.ResourceSpace{ProjectID: projectID, Kind: core.ResourceKindLocalFS, RootURI: t.TempDir()}); err != nil {
			t.Fatalf("create resource space: %v", err)
		}
		refID, _ := base.CreateThreadContextRef(ctx, &core.ThreadContextRef{ThreadID: threadID, ProjectID: projectID, Access: core.ContextAccessRead})
		if err := svc.DeleteThreadContextRef(ctx, threadID, refID); err == nil || err.Error() != "sync failed" {
			t.Fatalf("expected workspace sync failure, got %v", err)
		}
	})

	t.Run("delete thread aggregate failure propagates", func(t *testing.T) {
		base := newThreadAppTestStore(t)
		threadID, _ := createThreadFixture(t, base, true, false)
		projectID, _ := base.CreateProject(context.Background(), &core.Project{Name: "Project Alpha", Kind: core.ProjectGeneral})
		if _, err := base.CreateResourceSpace(context.Background(), &core.ResourceSpace{
			ProjectID: projectID,
			Kind:      core.ResourceKindLocalFS,
			RootURI:   t.TempDir(),
		}); err != nil {
			t.Fatalf("create resource space: %v", err)
		}
		if _, err := base.CreateThreadContextRef(context.Background(), &core.ThreadContextRef{
			ThreadID:  threadID,
			ProjectID: projectID,
			Access:    core.ContextAccessRead,
		}); err != nil {
			t.Fatalf("create context ref: %v", err)
		}
		store := &failingDeleteStore{Store: base, failDeleteContexts: true}
		if err := deleteThreadAggregateData(context.Background(), store, threadID); err == nil || err.Error() != "delete contexts failed" {
			t.Fatalf("expected delete contexts failure, got %v", err)
		}
	})

	t.Run("persist thread with participants failure and nil participant skip", func(t *testing.T) {
		base := newThreadAppTestStore(t)
		store := &failingCreateThreadStore{Store: base, failAddMember: true}
		thread := &core.Thread{Title: "thread"}
		participants := []*core.ThreadMember{
			nil,
			{Kind: core.ThreadMemberKindHuman, UserID: "owner", Role: "owner"},
		}
		if err := persistThreadWithParticipants(context.Background(), store, thread, participants); err == nil || err.Error() != "add member failed" {
			t.Fatalf("expected add member failure, got %v", err)
		}

		okStore := &failingCreateThreadStore{Store: base}
		thread = &core.Thread{Title: "thread-ok"}
		participants = []*core.ThreadMember{
			nil,
			{Kind: core.ThreadMemberKindHuman, UserID: "owner", Role: "owner"},
		}
		if err := persistThreadWithParticipants(context.Background(), okStore, thread, participants); err != nil {
			t.Fatalf("expected persist success, got %v", err)
		}
		if thread.ID == 0 || participants[1].ID == 0 {
			t.Fatalf("expected persisted ids, thread=%+v participants=%+v", thread, participants)
		}
	})

	t.Run("create thread aggregate and crystallize failure branches", func(t *testing.T) {
		base := newThreadAppTestStore(t)
		store := &failingCreateThreadStore{Store: base, failCreateThread: true}
		svc := newSQLiteThreadAppService(store, nil, nil)
		thread := &core.Thread{Title: "thread"}
		if err := svc.createThreadAggregate(context.Background(), thread, nil); err == nil || err.Error() != "create thread failed" {
			t.Fatalf("expected create thread failure, got %v", err)
		}
		if _, err := svc.CrystallizeChatSession(context.Background(), CrystallizeChatSessionInput{OwnerID: "owner"}); CodeOf(err) != CodeMissingTitle {
			t.Fatalf("expected %s, got %v", CodeMissingTitle, err)
		}

		workspace := &workspaceStub{err: errors.New("sync failed")}
		okSvc := New(Config{
			Store:     base,
			Workspace: workspace,
		})
		if _, err := okSvc.CrystallizeChatSession(context.Background(), CrystallizeChatSessionInput{
			SessionID:   "chat",
			ThreadTitle: "title",
			OwnerID:     "owner",
		}); err == nil || err.Error() != "sync failed" {
			t.Fatalf("expected workspace sync failure, got %v", err)
		}

		workItemFailSvc := New(Config{Store: &failingCreateThreadStore{Store: base, failCreateItem: true}})
		if _, err := workItemFailSvc.CrystallizeChatSession(context.Background(), CrystallizeChatSessionInput{
			SessionID:      "chat-2",
			ThreadTitle:    "title-2",
			ThreadSummary:  "summary",
			OwnerID:        "owner",
			CreateWorkItem: true,
			WorkItemTitle:  "work",
		}); err == nil || err.Error() != "create work item failed" {
			t.Fatalf("expected create work item failure, got %v", err)
		}

		threads, err := base.ListThreads(context.Background(), core.ThreadFilter{Limit: 20})
		if err != nil {
			t.Fatalf("list threads after rollback: %v", err)
		}
		for _, thread := range threads {
			if thread.Title == "title-2" {
				t.Fatalf("expected crystallize rollback for title-2, got threads %+v", threads)
			}
		}
	})

	t.Run("create work item from thread not found", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		svc := newSQLiteThreadAppService(store, newSQLiteTxAdapter(store, nil), nil)
		if _, err := svc.CreateWorkItemFromThread(context.Background(), CreateWorkItemFromThreadInput{ThreadID: 999, WorkItemTitle: "title"}); CodeOf(err) != CodeThreadNotFound {
			t.Fatalf("expected %s, got %v", CodeThreadNotFound, err)
		}
	})

	t.Run("create work item from thread without tx", func(t *testing.T) {
		store := newThreadAppTestStore(t)
		svc := New(Config{Store: store})
		ctx := context.Background()
		threadID, err := store.CreateThread(ctx, &core.Thread{Title: "thread", Summary: "summary"})
		if err != nil {
			t.Fatalf("create thread: %v", err)
		}
		result, err := svc.CreateWorkItemFromThread(ctx, CreateWorkItemFromThreadInput{
			ThreadID:      threadID,
			WorkItemTitle: "work item",
		})
		if err != nil {
			t.Fatalf("CreateWorkItemFromThread: %v", err)
		}
		if result.WorkItem == nil || result.Link == nil || result.WorkItem.Body != "summary" {
			t.Fatalf("unexpected non-tx work item result: %+v", result)
		}
	})

	t.Run("aggregate helpers rollback branches", func(t *testing.T) {
		base := newThreadAppTestStore(t)
		thread := &core.Thread{Title: "aggregate-thread"}
		participants := []*core.ThreadMember{{Kind: core.ThreadMemberKindHuman, UserID: "owner", Role: "owner"}}
		store := &failingCreateThreadStore{Store: base, failAddMember: true}
		svc := New(Config{Store: store})
		err := svc.createThreadAggregate(context.Background(), thread, participants)
		if err == nil || err.Error() != "add member failed" {
			t.Fatalf("expected add member failure, got %v", err)
		}

		txSvc := New(Config{
			Store: base,
			Tx: newSQLiteTxAdapter(base, func(txStore core.Store) (TxStore, error) {
				sqliteStore, ok := txStore.(*sqlite.Store)
				if !ok {
					return nil, fmt.Errorf("unexpected tx store type %T", txStore)
				}
				return &failingDeleteStore{Store: sqliteStore, failDeleteMessages: true}, nil
			}),
		})
		threadID, _ := createThreadFixture(t, base, true, false)
		if err := txSvc.deleteThreadAggregate(context.Background(), threadID); err == nil || err.Error() != "delete messages failed" {
			t.Fatalf("expected delete messages failure, got %v", err)
		}

		notFoundStore := &failingCreateThreadStore{Store: base, failDeleteThread: true}
		if err := deleteThreadAggregateData(context.Background(), notFoundStore, threadID); CodeOf(err) != CodeThreadNotFound {
			t.Fatalf("expected %s, got %v", CodeThreadNotFound, err)
		}

		workItemStore := &failingLinkCleanupStore{failingCreateThreadStore: &failingCreateThreadStore{Store: base, failDeleteItem: true}}
		_, _, err = createLinkedWorkItemFromThreadData(context.Background(), workItemStore, &core.Thread{ID: 1, Summary: "summary"}, "title", "", nil)
		if err == nil || !strings.Contains(err.Error(), "rollback failed") {
			t.Fatalf("expected rollback failed error, got %v", err)
		}

		refStore := &failingGetRefStore{Store: base, failGetRef: true}
		refSvc := New(Config{Store: refStore})
		threadID, _ = base.CreateThread(context.Background(), &core.Thread{Title: "thread-ref"})
		if err := refSvc.DeleteThreadContextRef(context.Background(), threadID, 1); err == nil || err.Error() != "get ref failed" {
			t.Fatalf("expected generic get ref failure, got %v", err)
		}
	})
}
