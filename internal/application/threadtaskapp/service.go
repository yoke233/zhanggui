package threadtaskapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Config holds all dependencies for the ThreadTask service.
type Config struct {
	Store        Store
	Bus          EventPublisher
	Notifier     NotificationSender
	AgentPool    AgentDispatcher
}

// Service implements ThreadTask group lifecycle and scheduling.
type Service struct {
	store     Store
	bus       EventPublisher
	notifier  NotificationSender
	agentPool AgentDispatcher
}

// New creates a ThreadTask service.
func New(cfg Config) *Service {
	return &Service{
		store:     cfg.Store,
		bus:       cfg.Bus,
		notifier:  cfg.Notifier,
		agentPool: cfg.AgentPool,
	}
}

// CreateTaskGroup creates a new TaskGroup with its tasks and kicks off scheduling.
func (s *Service) CreateTaskGroup(ctx context.Context, input CreateTaskGroupInput) (*core.ThreadTaskGroupDetail, error) {
	if input.ThreadID <= 0 {
		return nil, newError(CodeMissingThreadID, "thread_id is required", nil)
	}
	if _, err := s.store.GetThread(ctx, input.ThreadID); err != nil {
		if err == core.ErrNotFound {
			return nil, newError(CodeThreadNotFound, "thread not found", err)
		}
		return nil, err
	}
	if len(input.Tasks) == 0 {
		return nil, newError(CodeMissingTasks, "at least one task is required", nil)
	}

	// Validate tasks
	for i, t := range input.Tasks {
		if strings.TrimSpace(t.Assignee) == "" {
			return nil, newError(CodeMissingAssignee, fmt.Sprintf("task[%d]: assignee is required", i), nil)
		}
		if strings.TrimSpace(t.Instruction) == "" {
			return nil, newError(CodeMissingInstruction, fmt.Sprintf("task[%d]: instruction is required", i), nil)
		}
		taskType := strings.TrimSpace(t.Type)
		if taskType == "" {
			taskType = "work"
		}
		if _, err := core.ParseTaskType(taskType); err != nil {
			return nil, newError(CodeInvalidTaskType, fmt.Sprintf("task[%d]: %v", i, err), err)
		}
		// Validate dependency indices
		for _, depIdx := range t.DependsOnIndex {
			if depIdx < 0 || depIdx >= len(input.Tasks) {
				return nil, newError(CodeInvalidDependency, fmt.Sprintf("task[%d]: depends_on_index %d out of range", i, depIdx), nil)
			}
			if depIdx == i {
				return nil, newError(CodeDependencyCycle, fmt.Sprintf("task[%d]: cannot depend on itself", i), nil)
			}
		}
	}

	// Create group
	group := &core.ThreadTaskGroup{
		ThreadID:         input.ThreadID,
		Status:           core.TaskGroupPending,
		SourceMessageID:  input.SourceMessageID,
		NotifyOnComplete: input.NotifyOnComplete,
	}
	groupID, err := s.store.CreateThreadTaskGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	group.ID = groupID

	// Create tasks (two passes: create first, then resolve dependency IDs)
	tasks := make([]*core.ThreadTask, len(input.Tasks))
	for i, t := range input.Tasks {
		taskType := strings.TrimSpace(t.Type)
		if taskType == "" {
			taskType = "work"
		}
		maxRetries := 0
		if t.MaxRetries != nil {
			maxRetries = *t.MaxRetries
		} else if taskType == "review" {
			maxRetries = 3
		}

		outputFilePath := ""
		if fn := strings.TrimSpace(t.OutputFileName); fn != "" {
			outputFilePath = "outputs/" + fn
		}

		task := &core.ThreadTask{
			GroupID:        groupID,
			ThreadID:       input.ThreadID,
			Assignee:       strings.TrimSpace(t.Assignee),
			Type:           core.TaskType(taskType),
			Instruction:    strings.TrimSpace(t.Instruction),
			DependsOn:      []int64{}, // resolved below
			Status:         core.ThreadTaskPending,
			OutputFilePath: outputFilePath,
			MaxRetries:     maxRetries,
		}
		taskID, err := s.store.CreateThreadTask(ctx, task)
		if err != nil {
			return nil, err
		}
		task.ID = taskID
		tasks[i] = task
	}

	// Resolve depends_on_index → actual task IDs
	for i, t := range input.Tasks {
		if len(t.DependsOnIndex) == 0 {
			continue
		}
		deps := make([]int64, 0, len(t.DependsOnIndex))
		for _, depIdx := range t.DependsOnIndex {
			deps = append(deps, tasks[depIdx].ID)
		}
		tasks[i].DependsOn = deps
		if err := s.store.UpdateThreadTask(ctx, tasks[i]); err != nil {
			return nil, err
		}
	}

	// Insert progress card message
	progressMsg := &core.ThreadMessage{
		ThreadID: input.ThreadID,
		SenderID: "system",
		Role:     "system",
		Content:  "",
		Metadata: s.buildProgressCardMetadata(group, tasks),
	}
	msgID, err := s.store.CreateThreadMessage(ctx, progressMsg)
	if err == nil {
		group.StatusMessageID = &msgID
		_ = s.store.UpdateThreadTaskGroup(ctx, group)
	}

	// Update group to running and kick off scheduling
	group.Status = core.TaskGroupRunning
	_ = s.store.UpdateThreadTaskGroup(ctx, group)

	// Publish event
	s.publishEvent(ctx, core.EventThreadTaskGroupCreated, map[string]any{
		"thread_id":     input.ThreadID,
		"task_group_id": groupID,
		"task_count":    len(tasks),
	})

	// Run first scheduling tick
	if err := s.Tick(ctx, groupID); err != nil {
		slog.Warn("initial tick failed", "group_id", groupID, "error", err)
	}

	// Reload tasks to get latest status
	tasks, _ = s.store.ListThreadTasksByGroup(ctx, groupID)

	return &core.ThreadTaskGroupDetail{
		ThreadTaskGroup: *group,
		Tasks:           tasks,
	}, nil
}

// Signal processes a task completion or rejection signal from an agent.
func (s *Service) Signal(ctx context.Context, input SignalInput) error {
	task, err := s.store.GetThreadTask(ctx, input.TaskID)
	if err != nil {
		if err == core.ErrNotFound {
			return newError(CodeTaskNotFound, "task not found", err)
		}
		return err
	}

	switch input.Action {
	case "complete":
		return s.handleTaskComplete(ctx, task, input.OutputFilePath)
	case "reject":
		return s.handleReviewReject(ctx, task, input.OutputFilePath, input.Feedback)
	default:
		return newError(CodeInvalidAction, fmt.Sprintf("invalid action %q, must be 'complete' or 'reject'", input.Action), nil)
	}
}

func (s *Service) handleTaskComplete(ctx context.Context, task *core.ThreadTask, outputFilePath string) error {
	if task.Status != core.ThreadTaskRunning {
		return newError(CodeInvalidState, "task is not running", nil)
	}

	now := time.Now().UTC()
	task.Status = core.ThreadTaskDone
	task.CompletedAt = &now
	if outputFilePath != "" {
		task.OutputFilePath = outputFilePath
	}
	if err := s.store.UpdateThreadTask(ctx, task); err != nil {
		return err
	}

	// Insert output card message
	s.insertOutputMessage(ctx, task)

	// Publish event
	s.publishEvent(ctx, core.EventThreadTaskCompleted, map[string]any{
		"thread_id":     task.ThreadID,
		"task_group_id": task.GroupID,
		"task_id":       task.ID,
		"status":        string(task.Status),
		"assignee":      task.Assignee,
		"output_file":   task.OutputFilePath,
	})

	// Trigger next scheduling tick
	return s.Tick(ctx, task.GroupID)
}

func (s *Service) handleReviewReject(ctx context.Context, reviewTask *core.ThreadTask, outputFilePath, feedback string) error {
	if reviewTask.Status != core.ThreadTaskRunning {
		return newError(CodeInvalidState, "task is not running", nil)
	}
	if reviewTask.Type != core.TaskTypeReview {
		return newError(CodeInvalidAction, "reject is only valid for review tasks", nil)
	}

	now := time.Now().UTC()
	reviewTask.Status = core.ThreadTaskRejected
	reviewTask.CompletedAt = &now
	if outputFilePath != "" {
		reviewTask.OutputFilePath = outputFilePath
	}
	if err := s.store.UpdateThreadTask(ctx, reviewTask); err != nil {
		return err
	}

	// Insert reject message in chat
	s.insertRejectMessage(ctx, reviewTask, feedback)

	// Reset upstream work tasks for retry
	for _, depID := range reviewTask.DependsOn {
		workTask, err := s.store.GetThreadTask(ctx, depID)
		if err != nil || workTask.Type != core.TaskTypeWork {
			continue
		}

		if workTask.RetryCount >= workTask.MaxRetries {
			workTask.Status = core.ThreadTaskFailed
			_ = s.store.UpdateThreadTask(ctx, workTask)
			s.failGroup(ctx, reviewTask.GroupID)
			return nil
		}

		workTask.RetryCount++
		workTask.Status = core.ThreadTaskPending
		workTask.ReviewFeedback = feedback
		workTask.CompletedAt = nil
		_ = s.store.UpdateThreadTask(ctx, workTask)

		// Reset review task to pending too
		reviewTask.Status = core.ThreadTaskPending
		reviewTask.CompletedAt = nil
		_ = s.store.UpdateThreadTask(ctx, reviewTask)
	}

	s.publishEvent(ctx, core.EventThreadTaskCompleted, map[string]any{
		"thread_id":     reviewTask.ThreadID,
		"task_group_id": reviewTask.GroupID,
		"task_id":       reviewTask.ID,
		"status":        "rejected",
		"assignee":      reviewTask.Assignee,
		"feedback":      feedback,
	})

	return s.Tick(ctx, reviewTask.GroupID)
}

// Tick runs one scheduling cycle for a group.
func (s *Service) Tick(ctx context.Context, groupID int64) error {
	tasks, err := s.store.ListThreadTasksByGroup(ctx, groupID)
	if err != nil {
		return err
	}

	// 1. Promote pending tasks whose dependencies are all done → ready
	for _, t := range tasks {
		if t.Status != core.ThreadTaskPending {
			continue
		}
		if s.allDependsDone(t, tasks) {
			t.Status = core.ThreadTaskReady
			if err := s.store.UpdateThreadTask(ctx, t); err != nil {
				return err
			}
		}
	}

	// 2. Dispatch all ready tasks
	for _, t := range tasks {
		if t.Status != core.ThreadTaskReady {
			continue
		}
		s.dispatch(ctx, t, tasks)
	}

	// 3. Check if group is complete
	if s.allTasksTerminal(tasks) {
		if s.anyTaskFailed(tasks) {
			s.failGroup(ctx, groupID)
		} else {
			s.completeGroup(ctx, groupID)
		}
	}

	// 4. Update progress card
	s.updateProgressCard(ctx, groupID, tasks)

	return nil
}

// GetGroupDetail returns a group with all its tasks.
func (s *Service) GetGroupDetail(ctx context.Context, groupID int64) (*core.ThreadTaskGroupDetail, error) {
	group, err := s.store.GetThreadTaskGroup(ctx, groupID)
	if err != nil {
		if err == core.ErrNotFound {
			return nil, newError(CodeGroupNotFound, "task group not found", err)
		}
		return nil, err
	}
	tasks, err := s.store.ListThreadTasksByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return &core.ThreadTaskGroupDetail{
		ThreadTaskGroup: *group,
		Tasks:           tasks,
	}, nil
}

// ---------------------------------------------------------------------------
// Internal scheduling helpers
// ---------------------------------------------------------------------------

func (s *Service) allDependsDone(task *core.ThreadTask, allTasks []*core.ThreadTask) bool {
	if len(task.DependsOn) == 0 {
		return true
	}
	taskMap := make(map[int64]*core.ThreadTask, len(allTasks))
	for _, t := range allTasks {
		taskMap[t.ID] = t
	}
	for _, depID := range task.DependsOn {
		dep, ok := taskMap[depID]
		if !ok || dep.Status != core.ThreadTaskDone {
			return false
		}
	}
	return true
}

func (s *Service) allTasksTerminal(tasks []*core.ThreadTask) bool {
	for _, t := range tasks {
		if !t.Status.Terminal() {
			return false
		}
	}
	return len(tasks) > 0
}

func (s *Service) anyTaskFailed(tasks []*core.ThreadTask) bool {
	for _, t := range tasks {
		if t.Status == core.ThreadTaskFailed {
			return true
		}
	}
	return false
}

func (s *Service) dispatch(ctx context.Context, task *core.ThreadTask, allTasks []*core.ThreadTask) {
	task.Status = core.ThreadTaskRunning
	_ = s.store.UpdateThreadTask(ctx, task)

	s.publishEvent(ctx, core.EventThreadTaskStarted, map[string]any{
		"thread_id":     task.ThreadID,
		"task_group_id": task.GroupID,
		"task_id":       task.ID,
		"assignee":      task.Assignee,
		"type":          string(task.Type),
	})

	if s.agentPool == nil {
		return
	}

	// Collect upstream output files
	upstreamFiles := s.collectUpstreamOutputFiles(task, allTasks)

	// Build task input message
	input := s.buildTaskInput(task, upstreamFiles)

	// Invite agent, wait for boot, then send message. On failure, mark task as failed.
	go func() {
		bgCtx := context.Background()
		if _, err := s.agentPool.InviteAgent(bgCtx, task.ThreadID, task.Assignee); err != nil {
			slog.Warn("task dispatch: invite agent failed",
				"task_id", task.ID, "assignee", task.Assignee, "error", err)
			s.markTaskFailed(bgCtx, task, fmt.Sprintf("agent invite failed: %v", err))
			return
		}
		// Wait for agent to finish booting (up to 60s).
		waitCtx, waitCancel := context.WithTimeout(bgCtx, 60*time.Second)
		defer waitCancel()
		if err := s.agentPool.WaitAgentReady(waitCtx, task.ThreadID, task.Assignee); err != nil {
			slog.Warn("task dispatch: agent boot failed",
				"task_id", task.ID, "assignee", task.Assignee, "error", err)
			s.markTaskFailed(bgCtx, task, fmt.Sprintf("agent boot failed: %v", err))
			return
		}
		if err := s.agentPool.SendMessage(bgCtx, task.ThreadID, task.Assignee, input); err != nil {
			slog.Warn("task dispatch: send message failed",
				"task_id", task.ID, "assignee", task.Assignee, "error", err)
			s.markTaskFailed(bgCtx, task, fmt.Sprintf("agent message failed: %v", err))
			return
		}
	}()
}

// markTaskFailed marks a task as failed (e.g. agent boot failure) and triggers group-level check.
func (s *Service) markTaskFailed(ctx context.Context, task *core.ThreadTask, reason string) {
	now := time.Now().UTC()
	task.Status = core.ThreadTaskFailed
	task.CompletedAt = &now
	task.ReviewFeedback = reason
	_ = s.store.UpdateThreadTask(ctx, task)

	s.publishEvent(ctx, core.EventThreadTaskCompleted, map[string]any{
		"thread_id":     task.ThreadID,
		"task_group_id": task.GroupID,
		"task_id":       task.ID,
		"status":        "failed",
		"assignee":      task.Assignee,
		"error":         reason,
	})

	s.failGroup(ctx, task.GroupID)
}

func (s *Service) collectUpstreamOutputFiles(task *core.ThreadTask, allTasks []*core.ThreadTask) []string {
	if len(task.DependsOn) == 0 {
		return nil
	}
	taskMap := make(map[int64]*core.ThreadTask, len(allTasks))
	for _, t := range allTasks {
		taskMap[t.ID] = t
	}
	var files []string
	for _, depID := range task.DependsOn {
		dep, ok := taskMap[depID]
		if !ok || dep.OutputFilePath == "" {
			continue
		}
		files = append(files, dep.OutputFilePath)
	}
	return files
}

func (s *Service) buildTaskInput(task *core.ThreadTask, upstreamFiles []string) string {
	var sb strings.Builder

	sb.WriteString("## 任务\n\n")
	sb.WriteString(task.Instruction)
	sb.WriteString("\n")

	if len(upstreamFiles) > 0 {
		sb.WriteString("\n## 上游产出\n\n")
		sb.WriteString("以下是上游任务的产出文件，请阅读后开展工作：\n\n")
		for _, f := range upstreamFiles {
			sb.WriteString("- 文件: ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n## 要求\n\n")
	if task.OutputFilePath != "" {
		sb.WriteString("- 将你的工作产出写入文件: ")
		sb.WriteString(task.OutputFilePath)
		sb.WriteString("\n")
	}
	sb.WriteString("- 产出格式: Markdown\n")

	if task.Type == core.TaskTypeReview {
		sb.WriteString("- 你是审核者。请审核上游产出是否达标。\n")
		sb.WriteString("- 审核通过: 在产出文件中写明\"审核通过\"及具体意见\n")
		sb.WriteString("- 审核不通过: 在产出文件中写明\"审核不通过\"及修改建议\n")
	}

	if task.ReviewFeedback != "" {
		sb.WriteString("\n## 上次审核反馈\n\n")
		sb.WriteString("上次提交被审核者打回，反馈如下：\n\n")
		sb.WriteString(task.ReviewFeedback)
		sb.WriteString("\n\n请根据反馈修改后重新提交。\n")
	}

	// Task context env vars — agent should export these before running task-signal scripts.
	sb.WriteString("\n## 任务上下文环境变量\n\n")
	sb.WriteString("在调用 task-signal skill 之前，请先设置以下环境变量：\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString(fmt.Sprintf("export AI_WORKFLOW_TASK_ID=%d\n", task.ID))
	sb.WriteString(fmt.Sprintf("export AI_WORKFLOW_TASK_GROUP_ID=%d\n", task.GroupID))
	sb.WriteString(fmt.Sprintf("export AI_WORKFLOW_TASK_TYPE=%s\n", string(task.Type)))
	if task.OutputFilePath != "" {
		sb.WriteString(fmt.Sprintf("export AI_WORKFLOW_OUTPUT_FILE=%s\n", task.OutputFilePath))
	}
	sb.WriteString("```\n")

	sb.WriteString("\n## 完成后\n\n")
	sb.WriteString("完成工作后，请使用 task-signal skill 报告完成。\n")

	return sb.String()
}

func (s *Service) completeGroup(ctx context.Context, groupID int64) {
	group, err := s.store.GetThreadTaskGroup(ctx, groupID)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	group.Status = core.TaskGroupDone
	group.CompletedAt = &now
	_ = s.store.UpdateThreadTaskGroup(ctx, group)

	tasks, _ := s.store.ListThreadTasksByGroup(ctx, groupID)
	var outputFiles []string
	for _, t := range tasks {
		if t.OutputFilePath != "" {
			outputFiles = append(outputFiles, t.OutputFilePath)
		}
	}

	// Insert group completion message
	completionMsg := &core.ThreadMessage{
		ThreadID: group.ThreadID,
		SenderID: "system",
		Role:     "system",
		Content:  fmt.Sprintf("Task Group #%d 已完成", groupID),
		Metadata: map[string]any{
			"type":           "task_group_completed",
			"task_group_id":  groupID,
			"final_status":   "done",
			"output_files":   outputFiles,
		},
	}
	_, _ = s.store.CreateThreadMessage(ctx, completionMsg)

	s.publishEvent(ctx, core.EventThreadTaskGroupCompleted, map[string]any{
		"thread_id":     group.ThreadID,
		"task_group_id": groupID,
		"final_status":  "done",
		"output_files":  outputFiles,
	})

	// Send notification
	if group.NotifyOnComplete && s.notifier != nil {
		thread, _ := s.store.GetThread(ctx, group.ThreadID)
		title := "任务完成"
		body := fmt.Sprintf("Task Group #%d 已完成", groupID)
		if thread != nil {
			body = fmt.Sprintf("Thread「%s」中的任务组已完成", thread.Title)
		}
		_, _ = s.notifier.Notify(ctx, &core.Notification{
			Level:     core.NotificationLevelSuccess,
			Title:     title,
			Body:      body,
			Category:  "chat",
			Channels:  []core.NotificationChannel{core.ChannelBrowser, core.ChannelInApp},
			ActionURL: fmt.Sprintf("/threads/%d", group.ThreadID),
		})
	}
}

func (s *Service) failGroup(ctx context.Context, groupID int64) {
	group, err := s.store.GetThreadTaskGroup(ctx, groupID)
	if err != nil {
		return
	}
	if group.Status == core.TaskGroupFailed {
		return
	}
	now := time.Now().UTC()
	group.Status = core.TaskGroupFailed
	group.CompletedAt = &now
	_ = s.store.UpdateThreadTaskGroup(ctx, group)

	// Insert group failure message
	failMsg := &core.ThreadMessage{
		ThreadID: group.ThreadID,
		SenderID: "system",
		Role:     "system",
		Content:  fmt.Sprintf("Task Group #%d 执行失败", groupID),
		Metadata: map[string]any{
			"type":           "task_group_completed",
			"task_group_id":  groupID,
			"final_status":   "failed",
		},
	}
	_, _ = s.store.CreateThreadMessage(ctx, failMsg)

	s.publishEvent(ctx, core.EventThreadTaskGroupCompleted, map[string]any{
		"thread_id":     group.ThreadID,
		"task_group_id": groupID,
		"final_status":  "failed",
	})

	// Send notification
	if group.NotifyOnComplete && s.notifier != nil {
		thread, _ := s.store.GetThread(ctx, group.ThreadID)
		title := "任务失败"
		body := fmt.Sprintf("Task Group #%d 执行失败", groupID)
		if thread != nil {
			body = fmt.Sprintf("Thread「%s」中的任务组执行失败", thread.Title)
		}
		_, _ = s.notifier.Notify(ctx, &core.Notification{
			Level:     core.NotificationLevelError,
			Title:     title,
			Body:      body,
			Category:  "chat",
			Channels:  []core.NotificationChannel{core.ChannelBrowser, core.ChannelInApp},
			ActionURL: fmt.Sprintf("/threads/%d", group.ThreadID),
		})
	}
}

func (s *Service) insertOutputMessage(ctx context.Context, task *core.ThreadTask) {
	var metadataType string
	var content string

	switch task.Type {
	case core.TaskTypeReview:
		metadataType = "task_review_approved"
		content = fmt.Sprintf("审核通过。详细意见见 %s", task.OutputFilePath)
	default:
		metadataType = "task_output"
		content = fmt.Sprintf("已完成工作，产出文件见 %s", task.OutputFilePath)
	}

	msg := &core.ThreadMessage{
		ThreadID: task.ThreadID,
		SenderID: task.Assignee,
		Role:     "agent",
		Content:  content,
		Metadata: map[string]any{
			"type":          metadataType,
			"task_id":       task.ID,
			"task_group_id": task.GroupID,
			"output_file":   task.OutputFilePath,
		},
	}
	msgID, err := s.store.CreateThreadMessage(ctx, msg)
	if err == nil {
		task.OutputMessageID = &msgID
		_ = s.store.UpdateThreadTask(ctx, task)
	}

	if s.bus != nil {
		s.bus.Publish(ctx, core.Event{
			Type: core.EventThreadMessage,
			Data: map[string]any{
				"thread_id":  msg.ThreadID,
				"message_id": msgID,
				"content":    msg.Content,
				"sender_id":  msg.SenderID,
				"role":       msg.Role,
				"metadata":   msg.Metadata,
			},
			Timestamp: time.Now().UTC(),
		})
	}
}

func (s *Service) insertRejectMessage(ctx context.Context, task *core.ThreadTask, feedback string) {
	retryRound := fmt.Sprintf("%d/%d", task.RetryCount+1, task.MaxRetries)
	content := fmt.Sprintf("审核未通过，已打回修改（第 %s 轮）。反馈: %s", retryRound, feedback)

	msg := &core.ThreadMessage{
		ThreadID: task.ThreadID,
		SenderID: task.Assignee,
		Role:     "agent",
		Content:  content,
		Metadata: map[string]any{
			"type":          "task_review_rejected",
			"task_id":       task.ID,
			"task_group_id": task.GroupID,
			"output_file":   task.OutputFilePath,
			"feedback":      feedback,
			"retry_round":   retryRound,
		},
	}
	_, _ = s.store.CreateThreadMessage(ctx, msg)
}

func (s *Service) updateProgressCard(ctx context.Context, groupID int64, tasks []*core.ThreadTask) {
	group, err := s.store.GetThreadTaskGroup(ctx, groupID)
	if err != nil || group.StatusMessageID == nil {
		return
	}

	// We update the progress card by publishing the event (front-end uses WebSocket)
	// The metadata is sent as event data; the existing message is identified by StatusMessageID
	s.publishEvent(ctx, core.EventThreadTaskGroupCreated, s.buildProgressCardMetadata(group, tasks))
}

func (s *Service) buildProgressCardMetadata(group *core.ThreadTaskGroup, tasks []*core.ThreadTask) map[string]any {
	taskSummaries := make([]map[string]any, len(tasks))
	var edges [][]int64

	for i, t := range tasks {
		taskSummaries[i] = map[string]any{
			"id":          t.ID,
			"assignee":    t.Assignee,
			"instruction": t.Instruction,
			"status":      string(t.Status),
			"type":        string(t.Type),
		}
		for _, depID := range t.DependsOn {
			edges = append(edges, []int64{depID, t.ID})
		}
	}

	return map[string]any{
		"type":           "task_group_progress",
		"thread_id":      group.ThreadID,
		"task_group_id":  group.ID,
		"tasks":          taskSummaries,
		"edges":          edges,
		"group_status":   string(group.Status),
	}
}

func (s *Service) publishEvent(ctx context.Context, eventType core.EventType, data map[string]any) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, core.Event{
		Type:      eventType,
		Category:  core.EventCategoryDomain,
		Data:      data,
		Timestamp: time.Now().UTC(),
	})
}
