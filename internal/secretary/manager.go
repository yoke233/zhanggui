package secretary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/user/ai-workflow/internal/core"
)

const (
	PlanActionApprove = "approve"
	PlanActionReject  = "reject"
	PlanActionAbandon = "abandon"
)

type CreateDraftInput struct {
	ProjectID    string
	SessionID    string
	Name         string
	FailPolicy   core.FailurePolicy
	Request      Request
	SourceFiles  []string
	FileContents map[string]string
}

type PlanAction struct {
	Action   string
	Feedback *HumanFeedback
}

type managerAgent interface {
	Decompose(ctx context.Context, req Request) (*core.TaskPlan, error)
}

type managerReviewOrchestrator interface {
	Run(ctx context.Context, plan *core.TaskPlan, input ReviewInput) (*ReviewResult, error)
	HandleHumanReject(ctx context.Context, plan *core.TaskPlan, feedback HumanFeedback, regenerator Regenerator) (*core.TaskPlan, error)
}

type managerScheduler interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	RecoverExecutingPlans(ctx context.Context) error
	StartPlan(ctx context.Context, plan *core.TaskPlan) error
}

type planTaskReplacer interface {
	ReplaceTaskPlanAndItems(plan *core.TaskPlan, items []core.TaskItem) error
}

type taskItemsCleaner interface {
	DeleteTaskItemsByPlan(planID string) error
}

type managerPlanContext struct {
	Request              Request
	ReviewInput          ReviewInput
	SourceFiles          []string
	FileContents         map[string]string
	ParseFailedFeedbacks []HumanFeedback
}

type ManagerOption func(*Manager)

func WithReviewGate(gate core.ReviewGate) ManagerOption {
	return func(m *Manager) {
		if m == nil {
			return
		}
		m.reviewGate = gate
	}
}

type Manager struct {
	store      core.Store
	agent      managerAgent
	review     managerReviewOrchestrator
	reviewGate core.ReviewGate
	scheduler  managerScheduler

	mu       sync.RWMutex
	planMeta map[string]managerPlanContext
}

func NewManager(store core.Store, agent managerAgent, review managerReviewOrchestrator, scheduler managerScheduler, opts ...ManagerOption) (*Manager, error) {
	if store == nil {
		return nil, errors.New("manager store is required")
	}
	if agent == nil {
		return nil, errors.New("manager agent is required")
	}
	if review == nil {
		return nil, errors.New("manager review orchestrator is required")
	}
	if scheduler == nil {
		return nil, errors.New("manager scheduler is required")
	}

	manager := &Manager{
		store:     store,
		agent:     agent,
		review:    review,
		scheduler: scheduler,
		planMeta:  make(map[string]managerPlanContext),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(manager)
	}

	return manager, nil
}

func (m *Manager) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.scheduler.Start(ctx); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	if err := m.scheduler.RecoverExecutingPlans(ctx); err != nil {
		return fmt.Errorf("recover executing plans: %w", err)
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.scheduler.Stop(ctx); err != nil {
		return fmt.Errorf("stop scheduler: %w", err)
	}
	return nil
}

func (m *Manager) CreateDraft(ctx context.Context, input CreateDraftInput) (*core.TaskPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return nil, errors.New("project id is required")
	}

	decomposed, err := m.agent.Decompose(ctx, input.Request)
	if err != nil {
		return nil, fmt.Errorf("agent decompose: %w", err)
	}
	if decomposed == nil {
		return nil, errors.New("agent returned nil task plan")
	}

	planID := strings.TrimSpace(decomposed.ID)
	if planID == "" {
		planID = core.NewTaskPlanID()
	}

	planName := strings.TrimSpace(input.Name)
	if planName == "" {
		planName = strings.TrimSpace(decomposed.Name)
	}
	if planName == "" {
		planName = planID
	}

	failPolicy := input.FailPolicy
	if failPolicy == "" {
		failPolicy = decomposed.FailPolicy
	}
	if failPolicy == "" {
		failPolicy = core.FailBlock
	}

	draft := &core.TaskPlan{
		ID:         planID,
		ProjectID:  projectID,
		SessionID:  strings.TrimSpace(input.SessionID),
		Name:       planName,
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: failPolicy,
		Tasks:      cloneManagerTaskItems(decomposed.Tasks),
	}
	if err := m.savePlanAndTasks(draft); err != nil {
		return nil, err
	}

	m.updatePlanMeta(planID, func(meta *managerPlanContext) {
		meta.Request = cloneManagerRequest(input.Request)
	})

	return m.GetPlan(ctx, planID)
}

func (m *Manager) CreateDraftFromFiles(ctx context.Context, input CreateDraftInput) (*core.TaskPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return nil, errors.New("project id is required")
	}

	sourceFiles, fileContents := normalizeSourceInputs(input.SourceFiles, input.FileContents)
	if len(fileContents) == 0 {
		return nil, errors.New("file contents are required")
	}

	planID := core.NewTaskPlanID()
	planName := strings.TrimSpace(input.Name)
	if planName == "" {
		planName = planID
	}

	failPolicy := input.FailPolicy
	if failPolicy == "" {
		failPolicy = core.FailBlock
	}

	draft := &core.TaskPlan{
		ID:         planID,
		ProjectID:  projectID,
		SessionID:  strings.TrimSpace(input.SessionID),
		Name:       planName,
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: failPolicy,
		Tasks:      nil,
	}
	setPlanSourceFiles(draft, sourceFiles)
	setPlanFileContents(draft, fileContents)
	if err := m.savePlanAndTasks(draft); err != nil {
		return nil, err
	}

	m.updatePlanMeta(planID, func(meta *managerPlanContext) {
		meta.Request = cloneManagerRequest(input.Request)
		meta.SourceFiles = cloneStringSlice(sourceFiles)
		meta.FileContents = cloneStringMap(fileContents)
	})

	return m.GetPlan(ctx, planID)
}

func (m *Manager) SubmitReview(ctx context.Context, planID string, input ReviewInput) (*core.TaskPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	plan, err := m.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan.Status != core.PlanDraft && plan.Status != core.PlanReviewing {
		return nil, fmt.Errorf("submit review requires draft/reviewing, got %s", plan.Status)
	}
	if hasPendingFileContents(plan, m.loadPlanFileContents(plan.ID)) && len(input.PlanFileContents) == 0 {
		planFileContents := m.loadPlanFileContents(plan.ID)
		if len(planFileContents) == 0 {
			planFileContents = loadPlanFileContents(plan)
		}
		input.PlanFileContents = cloneStringMap(planFileContents)
	}

	if m.reviewGate != nil {
		gatedPlan, gateErr := m.submitReviewWithGate(ctx, plan, input)
		if gateErr == nil {
			return gatedPlan, nil
		}

		fallbackPlan, fallbackErr := m.submitReviewWithPanel(ctx, plan, input)
		if fallbackErr != nil {
			return nil, fmt.Errorf("submit review gate failed: %v; fallback review orchestrator failed: %w", gateErr, fallbackErr)
		}
		return fallbackPlan, nil
	}

	return m.submitReviewWithPanel(ctx, plan, input)
}

func (m *Manager) ApplyPlanAction(ctx context.Context, planID string, action PlanAction) (*core.TaskPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	plan, err := m.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}

	switch normalizePlanAction(action.Action) {
	case PlanActionApprove:
		return m.applyApprove(ctx, plan)
	case PlanActionReject:
		return m.applyReject(ctx, plan, action)
	case PlanActionAbandon:
		return m.applyAbandon(ctx, plan)
	default:
		return nil, fmt.Errorf("unsupported plan action %q", strings.TrimSpace(action.Action))
	}
}

func (m *Manager) GetPlan(_ context.Context, planID string) (*core.TaskPlan, error) {
	trimmedID := strings.TrimSpace(planID)
	if trimmedID == "" {
		return nil, errors.New("plan id is required")
	}

	plan, err := m.store.GetTaskPlan(trimmedID)
	if err != nil {
		return nil, fmt.Errorf("get task plan %s: %w", trimmedID, err)
	}
	if plan.FailPolicy == "" {
		plan.FailPolicy = core.FailBlock
	}
	return plan, nil
}

func (m *Manager) applyApprove(ctx context.Context, plan *core.TaskPlan) (*core.TaskPlan, error) {
	if plan.Status != core.PlanWaitingHuman || plan.WaitReason != core.WaitFinalApproval {
		return nil, fmt.Errorf(
			"approve requires waiting_human/final_approval, got %s/%s",
			plan.Status,
			plan.WaitReason,
		)
	}
	if hasPendingFileContents(plan, m.loadPlanFileContents(plan.ID)) {
		return m.parseAndSchedule(ctx, plan)
	}
	if err := m.scheduler.StartPlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("start plan scheduler: %w", err)
	}
	return m.GetPlan(ctx, plan.ID)
}

func (m *Manager) applyReject(ctx context.Context, plan *core.TaskPlan, action PlanAction) (*core.TaskPlan, error) {
	if plan.Status != core.PlanWaitingHuman {
		return nil, fmt.Errorf("reject requires waiting_human, got %s", plan.Status)
	}
	if plan.WaitReason != core.WaitFinalApproval && plan.WaitReason != core.WaitFeedbackReq && !isWaitParseFailed(plan.WaitReason) {
		return nil, fmt.Errorf(
			"reject requires waiting_human/final_approval|feedback_required|parse_failed, got %s/%s",
			plan.Status,
			plan.WaitReason,
		)
	}
	if err := validateRejectFeedback(action.Feedback); err != nil {
		return nil, err
	}
	if isWaitParseFailed(plan.WaitReason) {
		feedback := HumanFeedback{
			Category:          action.Feedback.Category,
			Detail:            strings.TrimSpace(action.Feedback.Detail),
			ExpectedDirection: strings.TrimSpace(action.Feedback.ExpectedDirection),
		}
		m.updatePlanMeta(plan.ID, func(meta *managerPlanContext) {
			meta.ParseFailedFeedbacks = append(meta.ParseFailedFeedbacks, feedback)
		})

		updated := cloneManagerPlan(plan)
		updated.Status = core.PlanWaitingHuman
		updated.WaitReason = core.WaitFinalApproval
		if err := m.store.SaveTaskPlan(updated); err != nil {
			return nil, fmt.Errorf("save parse_failed reject reset: %w", err)
		}
		return m.GetPlan(ctx, updated.ID)
	}

	regenerator, err := m.newRegenerator(plan.ID)
	if err != nil {
		return nil, err
	}

	regenerated, err := m.review.HandleHumanReject(ctx, cloneManagerPlan(plan), *action.Feedback, regenerator)
	if err != nil {
		return nil, fmt.Errorf("handle human reject: %w", err)
	}
	if regenerated == nil {
		return nil, errors.New("handle human reject returned nil plan")
	}

	if err := m.savePlanAndTasks(regenerated); err != nil {
		return nil, err
	}

	reviewInput := m.loadReviewInput(plan.ID)
	if m.reviewGate != nil {
		resubmitted, gateErr := m.submitReviewWithGate(ctx, regenerated, reviewInput)
		if gateErr == nil {
			return resubmitted, nil
		}

		fallbackPlan, fallbackErr := m.submitReviewWithPanel(ctx, regenerated, reviewInput)
		if fallbackErr != nil {
			return nil, fmt.Errorf("resubmit review gate failed: %v; fallback review orchestrator failed: %w", gateErr, fallbackErr)
		}
		return fallbackPlan, nil
	}
	return m.submitReviewWithPanel(ctx, regenerated, reviewInput)
}

func (m *Manager) parseAndSchedule(ctx context.Context, plan *core.TaskPlan) (*core.TaskPlan, error) {
	if plan == nil {
		return nil, errors.New("task plan is required")
	}
	planID := strings.TrimSpace(plan.ID)
	if planID == "" {
		return nil, errors.New("task plan id is required")
	}

	meta, ok := m.loadPlanMeta(planID)
	baseRequest := cloneManagerRequest(meta.Request)
	var err error
	if !ok {
		baseRequest, err = m.hydrateRequestFromStore(planID, Request{})
		if err != nil {
			return nil, err
		}
	} else {
		baseRequest, err = m.hydrateRequestFromStore(planID, baseRequest)
		if err != nil {
			return nil, err
		}
	}

	sourceFiles := cloneStringSlice(meta.SourceFiles)
	fileContents := cloneStringMap(meta.FileContents)
	if len(sourceFiles) == 0 {
		sourceFiles = loadPlanSourceFiles(plan)
	}
	if len(fileContents) == 0 {
		fileContents = loadPlanFileContents(plan)
	}
	sourceFiles, fileContents = normalizeSourceInputs(sourceFiles, fileContents)
	if len(fileContents) == 0 {
		return m.markParseFailed(ctx, plan)
	}

	baseRequest.SourceFiles = cloneStringSlice(sourceFiles)
	baseRequest.FileContents = cloneStringMap(fileContents)
	if len(meta.ParseFailedFeedbacks) > 0 {
		lastFeedback := meta.ParseFailedFeedbacks[len(meta.ParseFailedFeedbacks)-1]
		feedbackJSON, marshalErr := marshalCompactJSON(lastFeedback)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal parse_failed feedback: %w", marshalErr)
		}
		baseRequest.HumanFeedbackJSON = feedbackJSON
	}

	m.updatePlanMeta(planID, func(next *managerPlanContext) {
		next.Request = cloneManagerRequest(baseRequest)
		next.SourceFiles = cloneStringSlice(sourceFiles)
		next.FileContents = cloneStringMap(fileContents)
	})

	parsedPlan, err := m.agent.Decompose(ctx, baseRequest)
	if err != nil {
		return m.markParseFailed(ctx, plan)
	}
	if parsedPlan == nil {
		return m.markParseFailed(ctx, plan)
	}

	normalizedTasks, err := m.normalizeTaskSet(planID, parsedPlan.Tasks)
	if err != nil {
		return m.markParseFailed(ctx, plan)
	}

	graph := Build(normalizedTasks)
	if err := graph.Validate(); err != nil {
		return m.markParseFailed(ctx, plan)
	}

	updated := cloneManagerPlan(plan)
	if parsedName := strings.TrimSpace(parsedPlan.Name); parsedName != "" {
		updated.Name = parsedName
	}
	updated.Tasks = normalizedTasks
	updated.Status = core.PlanApproved
	updated.WaitReason = core.WaitNone
	setPlanSourceFiles(updated, sourceFiles)
	setPlanFileContents(updated, fileContents)
	if err := m.savePlanAndTasks(updated); err != nil {
		return nil, err
	}

	if err := m.scheduler.StartPlan(ctx, updated); err != nil {
		return nil, fmt.Errorf("start plan scheduler: %w", err)
	}
	m.updatePlanMeta(planID, func(next *managerPlanContext) {
		next.ParseFailedFeedbacks = nil
	})
	return m.GetPlan(ctx, updated.ID)
}

func (m *Manager) markParseFailed(ctx context.Context, plan *core.TaskPlan) (*core.TaskPlan, error) {
	updated := cloneManagerPlan(plan)
	updated.Status = core.PlanWaitingHuman
	updated.WaitReason = waitReasonParseFailed
	if err := m.store.SaveTaskPlan(updated); err != nil {
		return nil, fmt.Errorf("save parse_failed plan: %w", err)
	}
	latest, err := m.GetPlan(ctx, updated.ID)
	if err != nil {
		return nil, err
	}
	return latest, nil
}

func (m *Manager) applyAbandon(ctx context.Context, plan *core.TaskPlan) (*core.TaskPlan, error) {
	if plan.Status != core.PlanWaitingHuman {
		return nil, fmt.Errorf("abandon requires waiting_human, got %s", plan.Status)
	}

	updated := cloneManagerPlan(plan)
	updated.Status = core.PlanAbandoned
	updated.WaitReason = core.WaitNone
	if err := m.store.SaveTaskPlan(updated); err != nil {
		return nil, fmt.Errorf("save abandoned plan: %w", err)
	}
	return m.GetPlan(ctx, updated.ID)
}

func validateRejectFeedback(feedback *HumanFeedback) error {
	if feedback == nil {
		return errors.New("reject action requires feedback")
	}

	// 第一段校验：字段存在性，避免直接进入语义校验时报错不直观。
	if strings.TrimSpace(string(feedback.Category)) == "" {
		return errors.New("reject action requires feedback.category")
	}
	if strings.TrimSpace(feedback.Detail) == "" {
		return errors.New("reject action requires feedback.detail")
	}

	// 第二段校验：复用领域规则（类别枚举 + detail 最小长度）。
	if err := feedback.Validate(); err != nil {
		return err
	}
	return nil
}

func normalizePlanAction(action string) string {
	return strings.ToLower(strings.TrimSpace(action))
}

func (m *Manager) submitReviewWithPanel(ctx context.Context, plan *core.TaskPlan, input ReviewInput) (*core.TaskPlan, error) {
	result, err := m.review.Run(ctx, cloneManagerPlan(plan), input)
	if err != nil {
		return nil, fmt.Errorf("run review orchestrator: %w", err)
	}
	if result == nil || result.Plan == nil {
		return nil, errors.New("review orchestrator returned nil plan")
	}

	if err := m.savePlanAndTasks(result.Plan); err != nil {
		return nil, err
	}

	m.updatePlanMeta(result.Plan.ID, func(meta *managerPlanContext) {
		meta.ReviewInput = cloneManagerReviewInput(input)
	})

	return m.GetPlan(ctx, result.Plan.ID)
}

func (m *Manager) submitReviewWithGate(ctx context.Context, plan *core.TaskPlan, input ReviewInput) (*core.TaskPlan, error) {
	reviewID, err := m.reviewGate.Submit(ctx, cloneManagerPlan(plan))
	if err != nil {
		return nil, fmt.Errorf("submit review gate: %w", err)
	}

	targetPlanID := strings.TrimSpace(reviewID)
	if targetPlanID == "" {
		targetPlanID = strings.TrimSpace(plan.ID)
	}

	reviewingPlan, err := m.GetPlan(ctx, targetPlanID)
	if err != nil && targetPlanID != strings.TrimSpace(plan.ID) {
		reviewingPlan, err = m.GetPlan(ctx, plan.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("load review gate plan %s: %w", targetPlanID, err)
	}

	if reviewingPlan.Status == core.PlanDraft {
		updated := cloneManagerPlan(reviewingPlan)
		updated.Status = core.PlanReviewing
		updated.WaitReason = core.WaitNone
		if err := m.savePlanAndTasks(updated); err != nil {
			return nil, err
		}
		reviewingPlan = updated
	}

	m.updatePlanMeta(reviewingPlan.ID, func(meta *managerPlanContext) {
		meta.ReviewInput = cloneManagerReviewInput(input)
	})

	return m.GetPlan(ctx, reviewingPlan.ID)
}

func (m *Manager) savePlanAndTasks(plan *core.TaskPlan) error {
	if plan == nil {
		return errors.New("task plan is required")
	}
	planID := strings.TrimSpace(plan.ID)
	if planID == "" {
		return errors.New("task plan id is required")
	}
	if strings.TrimSpace(plan.ProjectID) == "" {
		return errors.New("task plan project_id is required")
	}
	if strings.TrimSpace(plan.Name) == "" {
		plan.Name = planID
	}
	if plan.FailPolicy == "" {
		plan.FailPolicy = core.FailBlock
	}

	normalizedTasks, err := m.normalizeTaskSet(planID, plan.Tasks)
	if err != nil {
		return err
	}
	plan.Tasks = normalizedTasks

	if replacer, ok := m.store.(planTaskReplacer); ok {
		if err := replacer.ReplaceTaskPlanAndItems(plan, normalizedTasks); err != nil {
			return fmt.Errorf("replace task plan/items %s: %w", planID, err)
		}
		return nil
	}

	if err := m.store.SaveTaskPlan(plan); err != nil {
		return fmt.Errorf("save task plan %s: %w", planID, err)
	}

	if cleaner, ok := m.store.(taskItemsCleaner); ok {
		if err := cleaner.DeleteTaskItemsByPlan(planID); err != nil {
			return fmt.Errorf("clear old task items for plan %s: %w", planID, err)
		}
	}

	for i := range normalizedTasks {
		item := normalizedTasks[i]
		if err := m.store.SaveTaskItem(&item); err != nil {
			return fmt.Errorf("upsert task item %s: %w", item.ID, err)
		}
	}
	return nil
}

func (m *Manager) normalizeTaskSet(planID string, tasks []core.TaskItem) ([]core.TaskItem, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	normalized := make([]core.TaskItem, len(tasks))
	usedIDs := make(map[string]struct{}, len(tasks))
	idMap := make(map[string]string, len(tasks)*2)

	for i := range tasks {
		item := tasks[i]
		originalID := strings.TrimSpace(item.ID)
		targetID := originalID
		if targetID == "" {
			targetID = core.NewTaskItemID(planID, i+1)
		}
		targetID = m.resolveTaskIDCollision(planID, targetID, i+1, usedIDs)

		item.ID = targetID
		item.PlanID = planID
		if strings.TrimSpace(item.Template) == "" {
			item.Template = "standard"
		}
		if item.Status == "" {
			item.Status = core.ItemPending
		}
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("validate task %q: %w", targetID, err)
		}

		if originalID != "" {
			idMap[originalID] = targetID
		}
		idMap[targetID] = targetID
		usedIDs[targetID] = struct{}{}
		normalized[i] = item
	}

	for i := range normalized {
		remapped := make([]string, 0, len(normalized[i].DependsOn))
		seen := map[string]struct{}{}
		for _, dep := range normalized[i].DependsOn {
			key := strings.TrimSpace(dep)
			if key == "" {
				continue
			}
			if mapped, ok := idMap[key]; ok {
				key = mapped
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			remapped = append(remapped, key)
		}
		normalized[i].DependsOn = remapped
	}

	return normalized, nil
}

func (m *Manager) resolveTaskIDCollision(planID, candidate string, seq int, used map[string]struct{}) string {
	current := strings.TrimSpace(candidate)
	if current == "" {
		current = core.NewTaskItemID(planID, seq)
	}

	nextSeq := seq + 1
	for {
		if _, inBatch := used[current]; inBatch {
			current = core.NewTaskItemID(planID, nextSeq)
			nextSeq++
			continue
		}

		existing, err := m.store.GetTaskItem(current)
		if err == nil && existing != nil && strings.TrimSpace(existing.PlanID) != planID {
			current = core.NewTaskItemID(planID, nextSeq)
			nextSeq++
			continue
		}
		return current
	}
}

func (m *Manager) updatePlanMeta(planID string, mutate func(meta *managerPlanContext)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	meta := cloneManagerPlanContext(m.planMeta[planID])
	mutate(&meta)
	m.planMeta[planID] = meta
}

func (m *Manager) loadReviewInput(planID string) ReviewInput {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneManagerReviewInput(m.planMeta[planID].ReviewInput)
}

func (m *Manager) loadPlanMeta(planID string) (managerPlanContext, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	meta, ok := m.planMeta[planID]
	return cloneManagerPlanContext(meta), ok
}

func (m *Manager) loadPlanFileContents(planID string) map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneStringMap(m.planMeta[planID].FileContents)
}

func (m *Manager) newRegenerator(planID string) (Regenerator, error) {
	m.mu.RLock()
	meta, ok := m.planMeta[planID]
	m.mu.RUnlock()

	baseRequest := cloneManagerRequest(meta.Request)
	var err error
	if !ok {
		baseRequest, err = m.hydrateRequestFromStore(planID, Request{})
		if err != nil {
			return nil, err
		}
	} else {
		baseRequest, err = m.hydrateRequestFromStore(planID, baseRequest)
		if err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(baseRequest.Conversation) == "" &&
		strings.TrimSpace(baseRequest.OriginalConversationSummary) == "" {
		return nil, fmt.Errorf("regeneration conversation context is empty for plan %s", planID)
	}

	return managerRegenerator{
		store:       m.store,
		agent:       m.agent,
		baseRequest: baseRequest,
	}, nil
}

func (m *Manager) hydrateRequestFromStore(planID string, request Request) (Request, error) {
	plan, err := m.store.GetTaskPlan(planID)
	if err != nil {
		return request, fmt.Errorf("load task plan %s for regeneration context: %w", planID, err)
	}

	if strings.TrimSpace(request.ProjectName) == "" || strings.TrimSpace(request.RepoPath) == "" {
		project, projectErr := m.store.GetProject(plan.ProjectID)
		if projectErr == nil && project != nil {
			if strings.TrimSpace(request.ProjectName) == "" {
				request.ProjectName = strings.TrimSpace(project.Name)
			}
			if strings.TrimSpace(request.RepoPath) == "" {
				request.RepoPath = strings.TrimSpace(project.RepoPath)
			}
		}
	}

	if strings.TrimSpace(request.Conversation) == "" && strings.TrimSpace(request.OriginalConversationSummary) == "" {
		sessionID := strings.TrimSpace(plan.SessionID)
		if sessionID != "" {
			session, sessionErr := m.store.GetChatSession(sessionID)
			if sessionErr == nil && session != nil {
				request.Conversation = summarizeSessionMessages(session.Messages)
			}
		}
	}

	if strings.TrimSpace(request.Conversation) == "" && strings.TrimSpace(request.OriginalConversationSummary) == "" {
		request.Conversation = strings.TrimSpace(plan.Name)
	}
	if strings.TrimSpace(request.OriginalConversationSummary) == "" {
		request.OriginalConversationSummary = strings.TrimSpace(request.Conversation)
	}
	if strings.TrimSpace(request.RepoPath) == "" {
		request.RepoPath = "."
	}

	return request, nil
}

func summarizeSessionMessages(messages []core.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	lines := make([]string, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, content))
	}
	return strings.Join(lines, "\n")
}

type managerRegenerator struct {
	store       core.Store
	agent       managerAgent
	baseRequest Request
}

func (r managerRegenerator) Regenerate(ctx context.Context, req RegenerationRequest) (*core.TaskPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	planID := strings.TrimSpace(req.PlanID)
	if planID == "" {
		return nil, errors.New("regeneration request plan_id is required")
	}

	previousPlan, err := r.store.GetTaskPlan(planID)
	if err != nil {
		return nil, fmt.Errorf("load previous task plan %s: %w", planID, err)
	}

	previousTaskPlanJSON, err := marshalCompactJSON(previousPlan)
	if err != nil {
		return nil, fmt.Errorf("marshal previous task plan json: %w", err)
	}
	aiReviewSummaryJSON, err := marshalCompactJSON(req.AIReviewSummary)
	if err != nil {
		return nil, fmt.Errorf("marshal ai review summary json: %w", err)
	}
	humanFeedbackJSON, err := marshalCompactJSON(req.Feedback)
	if err != nil {
		return nil, fmt.Errorf("marshal human feedback json: %w", err)
	}

	request := r.baseRequest
	if strings.TrimSpace(request.OriginalConversationSummary) == "" {
		request.OriginalConversationSummary = strings.TrimSpace(request.Conversation)
	}
	request.PreviousTaskPlanJSON = previousTaskPlanJSON
	request.AIReviewSummaryJSON = aiReviewSummaryJSON
	request.HumanFeedbackJSON = humanFeedbackJSON

	nextPlan, err := r.agent.Decompose(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("agent decompose regenerated plan: %w", err)
	}
	if nextPlan == nil {
		return nil, errors.New("agent returned nil task plan for regeneration")
	}
	return cloneManagerPlan(nextPlan), nil
}

func marshalCompactJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func cloneManagerPlan(plan *core.TaskPlan) *core.TaskPlan {
	if plan == nil {
		return nil
	}
	cp := *plan
	cp.Tasks = cloneManagerTaskItems(plan.Tasks)
	copyPlanOptionalFileFields(&cp, plan)
	return &cp
}

func cloneManagerTaskItems(items []core.TaskItem) []core.TaskItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]core.TaskItem, len(items))
	for i, item := range items {
		out[i] = item
		out[i].Labels = append([]string(nil), item.Labels...)
		out[i].DependsOn = append([]string(nil), item.DependsOn...)
		out[i].Inputs = append([]string(nil), item.Inputs...)
		out[i].Outputs = append([]string(nil), item.Outputs...)
		out[i].Acceptance = append([]string(nil), item.Acceptance...)
		out[i].Constraints = append([]string(nil), item.Constraints...)
	}
	return out
}

func cloneManagerRequest(req Request) Request {
	cp := req
	cp.Env = copyMap(req.Env)
	cp.SourceFiles = cloneStringSlice(req.SourceFiles)
	cp.FileContents = cloneStringMap(req.FileContents)
	return cp
}

func cloneManagerReviewInput(input ReviewInput) ReviewInput {
	cp := input
	cp.PlanFileContents = cloneStringMap(input.PlanFileContents)
	return cp
}

func cloneManagerPlanContext(meta managerPlanContext) managerPlanContext {
	return managerPlanContext{
		Request:              cloneManagerRequest(meta.Request),
		ReviewInput:          cloneManagerReviewInput(meta.ReviewInput),
		SourceFiles:          cloneStringSlice(meta.SourceFiles),
		FileContents:         cloneStringMap(meta.FileContents),
		ParseFailedFeedbacks: append([]HumanFeedback(nil), meta.ParseFailedFeedbacks...),
	}
}

func normalizeSourceInputs(sourceFiles []string, fileContents map[string]string) ([]string, map[string]string) {
	normalizedContents := make(map[string]string, len(fileContents))
	for path, content := range fileContents {
		trimmedPath := strings.TrimSpace(path)
		if trimmedPath == "" {
			continue
		}
		normalizedContents[trimmedPath] = content
	}

	seen := map[string]struct{}{}
	normalizedSources := make([]string, 0, len(sourceFiles)+len(normalizedContents))
	for _, path := range sourceFiles {
		trimmedPath := strings.TrimSpace(path)
		if trimmedPath == "" {
			continue
		}
		if _, ok := seen[trimmedPath]; ok {
			continue
		}
		seen[trimmedPath] = struct{}{}
		normalizedSources = append(normalizedSources, trimmedPath)
	}
	for path := range normalizedContents {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		normalizedSources = append(normalizedSources, path)
	}
	slices.Sort(normalizedSources)

	return normalizedSources, normalizedContents
}
