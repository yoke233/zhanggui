package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/observability"
)

func (e *Executor) ApplyAction(ctx context.Context, action core.RunAction) error {
	if err := action.Validate(); err != nil {
		return err
	}

	p, err := e.store.GetRun(action.RunID)
	if err != nil {
		return err
	}
	if traceID := observability.TraceID(ctx); traceID != "" {
		if p.Config == nil {
			p.Config = map[string]any{}
		}
		p.Config["trace_id"] = traceID
	}

	stage := action.Stage
	if stage == "" {
		stage = p.CurrentStage
	}

	if err := e.store.RecordAction(core.HumanAction{
		RunID:   p.ID,
		Stage:   string(stage),
		Action:  string(action.Type),
		Message: action.Message,
		Source:  "manual",
		UserID:  "local",
	}); err != nil {
		return err
	}

	switch action.Type {
	case core.ActionApprove:
		return e.applyApprove(ctx, p, action, stage)
	case core.ActionReject:
		return e.applyReject(p, action, stage)
	case core.ActionModify:
		return e.applyModify(ctx, p, action, stage)
	case core.ActionSkip:
		return e.applySkip(ctx, p, action, stage)
	case core.ActionRerun:
		return e.applyRerun(ctx, p, action, stage)
	case core.ActionChangeRole:
		return e.applyChangeRole(ctx, p, action, stage)
	case core.ActionAbort:
		return e.applyAbort(p, action, stage)
	case core.ActionPause:
		return e.applyPause(p, action, stage)
	case core.ActionResume:
		return e.applyResume(ctx, p, action, stage)
	default:
		return fmt.Errorf("unsupported action: %s", action.Type)
	}
}

func (e *Executor) applyApprove(ctx context.Context, p *core.Run, action core.RunAction, stage core.StageID) error {
	if p.Status != core.StatusWaitingReview {
		return fmt.Errorf("approve requires waiting_review status, got %s", p.Status)
	}
	p.Status = core.StatusRunning
	p.ErrorMessage = ""
	p.UpdatedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	if err := e.publishActionApplied(p, action, stage); err != nil {
		return err
	}
	return e.RunScheduled(ctx, p.ID)
}

func (e *Executor) applyReject(p *core.Run, action core.RunAction, stage core.StageID) error {
	if stage == "" {
		return fmt.Errorf("reject action requires target stage")
	}
	if err := e.store.InvalidateCheckpointsFromStage(p.ID, stage); err != nil {
		return err
	}
	p.Status = core.StatusWaitingReview
	p.ErrorMessage = action.Message
	p.UpdatedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	if err := e.publishActionApplied(p, action, stage); err != nil {
		return err
	}
	return nil
}

func (e *Executor) applyModify(ctx context.Context, p *core.Run, action core.RunAction, stage core.StageID) error {
	if p.Artifacts == nil {
		p.Artifacts = map[string]string{}
	}
	if p.Config == nil {
		p.Config = map[string]any{}
	}
	p.Artifacts["modify_message"] = action.Message
	p.Config["modify_stage"] = string(stage)
	p.Status = core.StatusRunning
	p.ErrorMessage = action.Message
	p.UpdatedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	if err := e.publishActionApplied(p, action, stage); err != nil {
		return err
	}
	return e.RunScheduled(ctx, p.ID)
}

func (e *Executor) applySkip(ctx context.Context, p *core.Run, action core.RunAction, stage core.StageID) error {
	currentIndex := findStageIndex(p.Stages, p.CurrentStage)
	if currentIndex < 0 {
		return fmt.Errorf("skip action requires current stage")
	}
	next := currentIndex + 1
	if next >= len(p.Stages) {
		p.Status = core.StatusDone
		p.FinishedAt = time.Now()
		p.UpdatedAt = time.Now()
		if err := e.store.SaveRun(p); err != nil {
			return err
		}
		if err := e.publishActionApplied(p, action, stage); err != nil {
			return err
		}
		return nil
	}

	p.CurrentStage = p.Stages[next].Name
	p.Status = core.StatusRunning
	p.UpdatedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	if err := e.publishActionApplied(p, action, stage); err != nil {
		return err
	}
	return e.RunScheduled(ctx, p.ID)
}

func (e *Executor) applyRerun(ctx context.Context, p *core.Run, action core.RunAction, stage core.StageID) error {
	p.Status = core.StatusRunning
	p.UpdatedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	if err := e.publishActionApplied(p, action, stage); err != nil {
		return err
	}
	return e.RunScheduled(ctx, p.ID)
}

func (e *Executor) applyChangeRole(ctx context.Context, p *core.Run, action core.RunAction, stage core.StageID) error {
	if action.Role == "" {
		return fmt.Errorf("change_role requires role field")
	}
	target := stage
	if target == "" {
		target = p.CurrentStage
	}
	targetIndex := findStageIndex(p.Stages, target)
	if targetIndex < 0 {
		return fmt.Errorf("target stage %s not found", target)
	}
	p.Stages[targetIndex].Role = action.Role
	p.Status = core.StatusRunning
	p.UpdatedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	e.publishActionApplied(p, action, target)
	return e.RunScheduled(ctx, p.ID)
}

func (e *Executor) applyAbort(p *core.Run, action core.RunAction, stage core.StageID) error {
	p.Status = core.StatusFailed
	p.FinishedAt = time.Now()
	p.ErrorMessage = action.Message
	p.UpdatedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	if err := e.publishActionApplied(p, action, stage); err != nil {
		return err
	}
	return nil
}

func (e *Executor) applyPause(p *core.Run, action core.RunAction, stage core.StageID) error {
	if err := e.killActiveSession(p.ID); err != nil {
		return err
	}
	p.Status = core.StatusWaitingReview
	p.ErrorMessage = action.Message
	p.UpdatedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	if err := e.publishActionApplied(p, action, stage); err != nil {
		return err
	}
	e.bus.Publish(core.Event{
		Type:      core.EventRunwaiting_review,
		RunID:     p.ID,
		ProjectID: p.ProjectID,
		Stage:     stage,
		Timestamp: time.Now(),
	})
	return nil
}

func (e *Executor) applyResume(ctx context.Context, p *core.Run, action core.RunAction, stage core.StageID) error {
	if p.Status != core.StatusWaitingReview {
		return fmt.Errorf("resume requires waiting_review status, got %s", p.Status)
	}
	p.Status = core.StatusRunning
	p.ErrorMessage = ""
	p.UpdatedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	if err := e.publishActionApplied(p, action, stage); err != nil {
		return err
	}
	e.bus.Publish(core.Event{
		Type:      core.EventRunResumed,
		RunID:     p.ID,
		ProjectID: p.ProjectID,
		Stage:     stage,
		Timestamp: time.Now(),
	})
	return e.RunScheduled(ctx, p.ID)
}

func (e *Executor) publishActionApplied(p *core.Run, action core.RunAction, stage core.StageID) error {
	now := time.Now()
	logPayload := map[string]string{
		"action": string(action.Type),
	}
	if action.Message != "" {
		logPayload["message"] = action.Message
	}
	if action.Role != "" {
		logPayload["role"] = action.Role
	}
	logContent, err := json.Marshal(logPayload)
	if err != nil {
		return fmt.Errorf("marshal action_applied log payload: %w", err)
	}
	if err := e.appendEventLog(
		p.ID,
		stage,
		core.EventActionApplied,
		"manual",
		string(logContent),
		now,
	); err != nil {
		return err
	}

	data := map[string]string{
		"action": string(action.Type),
	}
	if action.Message != "" {
		data["message"] = action.Message
	}
	if action.Role != "" {
		data["role"] = action.Role
	}
	if traceID := RunTraceID(p); traceID != "" {
		data["trace_id"] = traceID
	}

	e.bus.Publish(core.Event{
		Type:      core.EventActionApplied,
		RunID:     p.ID,
		ProjectID: p.ProjectID,
		Stage:     stage,
		Data:      data,
		Timestamp: now,
	})
	return nil
}
