package threadapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/threadctx"
)

type Config struct {
	Store     Store
	Tx        Tx
	Runtime   Runtime
	Workspace WorkspaceManager
}

type Service struct {
	store     Store
	tx        Tx
	runtime   Runtime
	workspace WorkspaceManager
}

type projectLister interface {
	ListProjects(ctx context.Context, limit, offset int) ([]*core.Project, error)
}

func New(cfg Config) *Service {
	return &Service{
		store:     cfg.Store,
		tx:        cfg.Tx,
		runtime:   cfg.Runtime,
		workspace: cfg.Workspace,
	}
}

func (s *Service) CreateThread(ctx context.Context, input CreateThreadInput) (*CreateThreadResult, error) {
	thread := &core.Thread{
		Title:    strings.TrimSpace(input.Title),
		Status:   core.ThreadActive,
		OwnerID:  strings.TrimSpace(input.OwnerID),
		Metadata: cloneMetadata(input.Metadata),
	}
	if thread.Title == "" {
		return nil, newError(CodeMissingTitle, "title is required", nil)
	}

	participants := buildParticipants(thread.OwnerID, input.ParticipantUserIDs)
	if err := s.createThreadAggregate(ctx, thread, participants); err != nil {
		return nil, err
	}
	if !skipDefaultThreadContextRefs(thread.Metadata) {
		if err := s.attachDefaultThreadContextRefs(ctx, thread.ID); err != nil {
			_ = s.deleteThreadAggregate(ctx, thread.ID)
			return nil, err
		}
	}
	if err := s.syncThreadWorkspace(ctx, thread.ID); err != nil {
		_ = s.deleteThreadAggregate(ctx, thread.ID)
		return nil, err
	}

	return &CreateThreadResult{
		Thread:       thread,
		Participants: participants,
	}, nil
}

func skipDefaultThreadContextRefs(metadata map[string]any) bool {
	if len(metadata) == 0 {
		return false
	}
	value, _ := metadata["skip_default_context_refs"].(bool)
	return value
}

func (s *Service) DeleteThread(ctx context.Context, threadID int64) error {
	if _, err := s.store.GetThread(ctx, threadID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeThreadNotFound, "thread not found", err)
		}
		return err
	}

	if s.runtime != nil {
		if err := s.runtime.CleanupThread(ctx, threadID); err != nil {
			return newError(CodeCleanupThreadFailed, err.Error(), err)
		}
	}

	return s.deleteThreadAggregate(ctx, threadID)
}

func (s *Service) CreateThreadContextRef(ctx context.Context, input CreateThreadContextRefInput) (*core.ThreadContextRef, error) {
	if _, err := s.store.GetThread(ctx, input.ThreadID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeThreadNotFound, "thread not found", err)
		}
		return nil, err
	}
	if input.ProjectID <= 0 {
		return nil, newError(CodeMissingProjectID, "project_id is required", nil)
	}
	if _, err := s.store.GetProject(ctx, input.ProjectID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeProjectNotFound, "project not found", err)
		}
		return nil, err
	}

	access, err := core.ParseContextAccess(input.Access)
	if err != nil {
		return nil, newError(CodeInvalidContextAccess, err.Error(), err)
	}

	existing, err := s.store.ListThreadContextRefs(ctx, input.ThreadID)
	if err != nil {
		return nil, err
	}
	for _, item := range existing {
		if item != nil && item.ProjectID == input.ProjectID {
			return nil, newError(CodeContextRefConflict, "context ref already exists for project", nil)
		}
	}

	ref := &core.ThreadContextRef{
		ThreadID:  input.ThreadID,
		ProjectID: input.ProjectID,
		Access:    access,
		Note:      strings.TrimSpace(input.Note),
		GrantedBy: strings.TrimSpace(input.GrantedBy),
		ExpiresAt: input.ExpiresAt,
	}
	if _, err := threadctx.ResolveMount(ctx, s.store, ref); err != nil {
		return nil, err
	}

	id, err := s.store.CreateThreadContextRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	ref.ID = id
	if err := s.syncThreadWorkspace(ctx, input.ThreadID); err != nil {
		_ = s.store.DeleteThreadContextRef(ctx, ref.ID)
		return nil, err
	}
	if err := s.setThreadFocus(ctx, input.ThreadID, input.ProjectID); err != nil {
		_ = s.store.DeleteThreadContextRef(ctx, ref.ID)
		_ = s.syncThreadWorkspace(ctx, input.ThreadID)
		return nil, err
	}
	return ref, nil
}

func (s *Service) UpdateThreadContextRef(ctx context.Context, input UpdateThreadContextRefInput) (*core.ThreadContextRef, error) {
	if _, err := s.store.GetThread(ctx, input.ThreadID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeThreadNotFound, "thread not found", err)
		}
		return nil, err
	}

	ref, err := s.store.GetThreadContextRef(ctx, input.RefID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeContextRefNotFound, "context ref not found", err)
		}
		return nil, err
	}
	if ref.ThreadID != input.ThreadID {
		return nil, newError(CodeContextRefNotFound, "context ref not found", core.ErrNotFound)
	}

	access, err := core.ParseContextAccess(input.Access)
	if err != nil {
		return nil, newError(CodeInvalidContextAccess, err.Error(), err)
	}
	ref.Access = access
	if input.Note != nil {
		ref.Note = strings.TrimSpace(*input.Note)
	}
	if strings.TrimSpace(input.GrantedBy) != "" {
		ref.GrantedBy = strings.TrimSpace(input.GrantedBy)
	}
	ref.ExpiresAt = input.ExpiresAt
	if _, err := threadctx.ResolveMount(ctx, s.store, ref); err != nil {
		return nil, err
	}
	if err := s.store.UpdateThreadContextRef(ctx, ref); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeContextRefNotFound, "context ref not found", err)
		}
		return nil, err
	}
	if err := s.ensureThreadFocus(ctx, input.ThreadID, ref.ProjectID); err != nil {
		return nil, err
	}
	if err := s.syncThreadWorkspace(ctx, input.ThreadID); err != nil {
		return nil, err
	}
	return ref, nil
}

func (s *Service) DeleteThreadContextRef(ctx context.Context, threadID, refID int64) error {
	if _, err := s.store.GetThread(ctx, threadID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeThreadNotFound, "thread not found", err)
		}
		return err
	}
	ref, err := s.store.GetThreadContextRef(ctx, refID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeContextRefNotFound, "context ref not found", err)
		}
		return err
	}
	if ref.ThreadID != threadID {
		return newError(CodeContextRefNotFound, "context ref not found", core.ErrNotFound)
	}
	if err := s.store.DeleteThreadContextRef(ctx, refID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeContextRefNotFound, "context ref not found", err)
		}
		return err
	}
	if err := s.reconcileThreadFocusAfterRefDelete(ctx, threadID, ref.ProjectID); err != nil {
		return err
	}
	return s.syncThreadWorkspace(ctx, threadID)
}

func (s *Service) LinkThreadWorkItem(ctx context.Context, input LinkThreadWorkItemInput) (*core.ThreadWorkItemLink, error) {
	if _, err := s.store.GetThread(ctx, input.ThreadID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeThreadNotFound, "thread not found", err)
		}
		return nil, err
	}
	if input.WorkItemID <= 0 {
		return nil, newError(CodeMissingWorkItemID, "work_item_id is required", nil)
	}
	if _, err := s.store.GetWorkItem(ctx, input.WorkItemID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}

	link := &core.ThreadWorkItemLink{
		ThreadID:     input.ThreadID,
		WorkItemID:   input.WorkItemID,
		RelationType: strings.TrimSpace(input.RelationType),
		IsPrimary:    input.IsPrimary,
	}
	if link.RelationType == "" {
		link.RelationType = "related"
	}

	id, err := s.store.CreateThreadWorkItemLink(ctx, link)
	if err != nil {
		return nil, err
	}
	link.ID = id
	return link, nil
}

func (s *Service) FindActiveThreadByWorkItem(ctx context.Context, workItemID int64) (*core.Thread, error) {
	if workItemID <= 0 {
		return nil, newError(CodeMissingWorkItemID, "work_item_id is required", nil)
	}
	if _, err := s.store.GetWorkItem(ctx, workItemID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}

	links, err := s.store.ListThreadsByWorkItem(ctx, workItemID)
	if err != nil {
		return nil, err
	}

	var fallback *core.Thread
	for i := len(links) - 1; i >= 0; i-- {
		link := links[i]
		if link == nil {
			continue
		}
		thread, err := s.store.GetThread(ctx, link.ThreadID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if thread.Status != core.ThreadActive {
			continue
		}
		if link.IsPrimary {
			return thread, nil
		}
		if fallback == nil {
			fallback = thread
		}
	}
	return fallback, nil
}

func (s *Service) EnsureHumanParticipants(ctx context.Context, threadID int64, userIDs []string) ([]*core.ThreadMember, error) {
	if _, err := s.store.GetThread(ctx, threadID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeThreadNotFound, "thread not found", err)
		}
		return nil, err
	}

	existingMembers, err := s.store.ListThreadMembers(ctx, threadID)
	if err != nil {
		return nil, err
	}
	existingUsers := make(map[string]struct{}, len(existingMembers))
	for _, member := range existingMembers {
		if member == nil || member.Kind != core.ThreadMemberKindHuman {
			continue
		}
		userID := strings.TrimSpace(member.UserID)
		if userID == "" {
			continue
		}
		existingUsers[userID] = struct{}{}
	}

	added := make([]*core.ThreadMember, 0, len(userIDs))
	for _, userID := range userIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, exists := existingUsers[userID]; exists {
			continue
		}
		member := &core.ThreadMember{
			ThreadID: threadID,
			Kind:     core.ThreadMemberKindHuman,
			UserID:   userID,
			Role:     "member",
		}
		id, err := s.store.AddThreadMember(ctx, member)
		if err != nil {
			return nil, err
		}
		member.ID = id
		added = append(added, member)
		existingUsers[userID] = struct{}{}
	}
	return added, nil
}

func (s *Service) UnlinkThreadWorkItem(ctx context.Context, threadID, workItemID int64) error {
	if err := s.store.DeleteThreadWorkItemLink(ctx, threadID, workItemID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeLinkNotFound, "link not found", err)
		}
		return err
	}
	return nil
}

func (s *Service) CreateWorkItemFromThread(ctx context.Context, input CreateWorkItemFromThreadInput) (*CreateWorkItemFromThreadResult, error) {
	thread, err := s.store.GetThread(ctx, input.ThreadID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeThreadNotFound, "thread not found", err)
		}
		return nil, err
	}

	var workItem *core.WorkItem
	var link *core.ThreadWorkItemLink
	if s.tx != nil {
		err := s.tx.InTx(ctx, func(ctx context.Context, txStore TxStore) error {
			var err error
			workItem, link, err = createLinkedWorkItemFromThreadData(ctx, txStore, thread, input.WorkItemTitle, input.WorkItemBody, input.ProjectID)
			return err
		})
		if err != nil {
			return nil, err
		}
	} else {
		workItem, link, err = createLinkedWorkItemFromThreadData(ctx, s.store, thread, input.WorkItemTitle, input.WorkItemBody, input.ProjectID)
		if err != nil {
			return nil, err
		}
	}

	return &CreateWorkItemFromThreadResult{
		Thread:   thread,
		WorkItem: workItem,
		Link:     link,
	}, nil
}

func (s *Service) createThreadAggregate(ctx context.Context, thread *core.Thread, participants []*core.ThreadMember) error {
	if s.tx != nil {
		return s.tx.InTx(ctx, func(ctx context.Context, txStore TxStore) error {
			return persistThreadWithParticipants(ctx, txStore, thread, participants)
		})
	}

	if err := persistThreadWithParticipants(ctx, s.store, thread, participants); err != nil {
		if thread.ID > 0 {
			if rollbackErr := deleteThreadAggregateData(ctx, s.store, thread.ID); rollbackErr != nil {
				return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
			}
		}
		return err
	}
	return nil
}

func (s *Service) deleteThreadAggregate(ctx context.Context, threadID int64) error {
	if s.tx != nil {
		return s.tx.InTx(ctx, func(ctx context.Context, txStore TxStore) error {
			return deleteThreadAggregateData(ctx, txStore, threadID)
		})
	}
	return deleteThreadAggregateData(ctx, s.store, threadID)
}

func persistThreadWithParticipants(ctx context.Context, store TxStore, thread *core.Thread, participants []*core.ThreadMember) error {
	threadID, err := store.CreateThread(ctx, thread)
	if err != nil {
		return err
	}
	thread.ID = threadID

	for _, participant := range participants {
		if participant == nil {
			continue
		}
		participant.ThreadID = thread.ID
		id, err := store.AddThreadMember(ctx, participant)
		if err != nil {
			return err
		}
		participant.ID = id
	}
	return nil
}

func deleteThreadAggregateData(ctx context.Context, store TxStore, threadID int64) error {
	if err := store.DeleteThreadWorkItemLinksByThread(ctx, threadID); err != nil {
		return err
	}
	if err := store.DeleteThreadContextRefsByThread(ctx, threadID); err != nil {
		return err
	}
	if err := store.DeleteThreadAttachmentsByThread(ctx, threadID); err != nil {
		return err
	}
	if err := store.DeleteResourcesByThread(ctx, threadID); err != nil {
		return err
	}
	if err := store.DeleteThreadMessagesByThread(ctx, threadID); err != nil {
		return err
	}
	if err := store.DeleteThreadMembersByThread(ctx, threadID); err != nil {
		return err
	}
	if err := store.DeleteThread(ctx, threadID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeThreadNotFound, "thread not found", err)
		}
		return err
	}
	return nil
}

func createLinkedWorkItemFromThreadData(ctx context.Context, store TxStore, thread *core.Thread, title string, body string, projectID *int64) (*core.WorkItem, *core.ThreadWorkItemLink, error) {
	if thread == nil {
		return nil, nil, errors.New("thread is required")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return nil, nil, newError(CodeMissingTitle, "title is required", nil)
	}

	body = strings.TrimSpace(body)
	if body == "" {
		body = thread.Title
	}

	workItem := &core.WorkItem{
		Title:     title,
		Body:      body,
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
		ProjectID: projectID,
		Metadata: map[string]any{
			"source_thread_id": thread.ID,
			"source_type":      "thread_manual",
		},
	}

	id, err := store.CreateWorkItem(ctx, workItem)
	if err != nil {
		return nil, nil, err
	}
	workItem.ID = id

	link := &core.ThreadWorkItemLink{
		ThreadID:     thread.ID,
		WorkItemID:   id,
		RelationType: "drives",
		IsPrimary:    true,
	}
	linkID, err := store.CreateThreadWorkItemLink(ctx, link)
	if err != nil {
		if rollbackErr := store.DeleteWorkItem(ctx, id); rollbackErr != nil {
			return nil, nil, fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
		}
		return nil, nil, err
	}
	link.ID = linkID

	return workItem, link, nil
}

func buildParticipants(ownerID string, memberIDs []string) []*core.ThreadMember {
	participants := make([]*core.ThreadMember, 0, len(memberIDs)+1)
	seen := make(map[string]bool)

	ownerID = strings.TrimSpace(ownerID)
	if ownerID != "" {
		participants = append(participants, &core.ThreadMember{
			Kind:   core.ThreadMemberKindHuman,
			UserID: ownerID,
			Role:   "owner",
		})
		seen[ownerID] = true
	}

	for _, participantID := range memberIDs {
		participantID = strings.TrimSpace(participantID)
		if participantID == "" || seen[participantID] {
			continue
		}
		participants = append(participants, &core.ThreadMember{
			Kind:   core.ThreadMemberKindHuman,
			UserID: participantID,
			Role:   "member",
		})
		seen[participantID] = true
	}

	return participants
}

func cloneMetadata(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *Service) setThreadFocus(ctx context.Context, threadID int64, projectID int64) error {
	thread, err := s.store.GetThread(ctx, threadID)
	if err != nil {
		return err
	}
	core.SetThreadFocusProjectID(thread, projectID)
	return s.store.UpdateThread(ctx, thread)
}

func (s *Service) ensureThreadFocus(ctx context.Context, threadID int64, projectID int64) error {
	thread, err := s.store.GetThread(ctx, threadID)
	if err != nil {
		return err
	}
	if _, ok := core.ReadThreadFocusProjectID(thread); ok {
		return nil
	}
	core.SetThreadFocusProjectID(thread, projectID)
	return s.store.UpdateThread(ctx, thread)
}

func (s *Service) reconcileThreadFocusAfterRefDelete(ctx context.Context, threadID int64, deletedProjectID int64) error {
	thread, err := s.store.GetThread(ctx, threadID)
	if err != nil {
		return err
	}
	focusProjectID, ok := core.ReadThreadFocusProjectID(thread)
	if !ok || focusProjectID != deletedProjectID {
		return nil
	}
	refs, err := s.store.ListThreadContextRefs(ctx, threadID)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		core.ClearThreadFocus(thread)
		return s.store.UpdateThread(ctx, thread)
	}
	core.SetThreadFocusProjectID(thread, refs[0].ProjectID)
	return s.store.UpdateThread(ctx, thread)
}

func (s *Service) syncThreadWorkspace(ctx context.Context, threadID int64) error {
	if s == nil || s.workspace == nil {
		return nil
	}
	if err := s.workspace.EnsureThreadWorkspace(ctx, threadID); err != nil {
		return err
	}
	return s.workspace.SyncThreadWorkspaceContext(ctx, threadID)
}

func (s *Service) attachDefaultThreadContextRefs(ctx context.Context, threadID int64) error {
	if s == nil || s.store == nil {
		return nil
	}
	lister, ok := s.store.(projectLister)
	if !ok {
		return nil
	}

	const batchSize = 200
	offset := 0
	var focusProjectID int64

	for {
		projects, err := lister.ListProjects(ctx, batchSize, offset)
		if err != nil {
			return err
		}
		if len(projects) == 0 {
			break
		}
		for _, project := range projects {
			if project == nil || project.ID <= 0 {
				continue
			}
			ref := &core.ThreadContextRef{
				ThreadID:  threadID,
				ProjectID: project.ID,
				Access:    core.ContextAccessRead,
			}
			if _, err := threadctx.ResolveMount(ctx, s.store, ref); err != nil {
				continue
			}
			refID, err := s.store.CreateThreadContextRef(ctx, ref)
			if err != nil {
				return err
			}
			ref.ID = refID
			if focusProjectID == 0 {
				focusProjectID = project.ID
			}
		}
		if len(projects) < batchSize {
			break
		}
		offset += len(projects)
	}

	if focusProjectID > 0 {
		return s.ensureThreadFocus(ctx, threadID, focusProjectID)
	}
	return nil
}
