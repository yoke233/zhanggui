package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/gateway"
)

func (h *Handler) runTaskNew(ctx context.Context, gw *gateway.Gateway, emitter *Emitter, st *RunState, req RunRequest, session *Session) error {
	// v1：最小“可控暂停”演示工作流（不是最终任务流水线）
	// - 多个 step
	// - step 边界检查 Thread.phase=PAUSE_REQUESTED
	// - 若收到暂停请求：在安全点 interrupt

	st.Status = "RUNNING"
	st.PendingInt = nil
	st.PendingTool = nil
	if st.Data == nil {
		st.Data = map[string]any{}
	}
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	// STEP: WORK_A
	if err := h.runTaskWorkStep(ctx, gw, emitter, st, "WORK_A", "work A: doing something...\n"); err != nil {
		return err
	}
	if interrupted, err := h.checkpointMaybeInterrupt(ctx, gw, emitter, st, req); err != nil {
		return err
	} else if interrupted {
		return nil
	}

	// STEP: WORK_B
	if err := h.runTaskWorkStep(ctx, gw, emitter, st, "WORK_B", "work B: doing something...\n"); err != nil {
		return err
	}
	if interrupted, err := h.checkpointMaybeInterrupt(ctx, gw, emitter, st, req); err != nil {
		return err
	} else if interrupted {
		return nil
	}

	st.Status = "DONE"
	st.CurrentStep = ""
	st.PendingInt = nil
	st.PendingTool = nil
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	return emitter.Emit(Event{
		"type":    "RUN_FINISHED",
		"outcome": "success",
		"result":  map[string]any{"ok": true},
	})
}

func (h *Handler) runTaskResume(ctx context.Context, gw *gateway.Gateway, emitter *Emitter, st *RunState, req RunRequest, session *Session) error {
	if req.Resume == nil || strings.TrimSpace(req.Resume.InterruptID) == "" {
		return fmt.Errorf("missing resume.interruptId")
	}

	changeSetID := ""
	if m, ok := req.Resume.Payload.(map[string]any); ok {
		changeSetID = firstString(m, "changeSetId", "change_set_id", "changesetId")
	}

	// 兜底：若未传 changeSetId，则尝试从 ThreadStatus 读 lastChangeSetId。
	if strings.TrimSpace(changeSetID) == "" {
		if src, ok := h.threadSink.(ThreadStatusSource); ok {
			_, last, err := src.ThreadStatus(req.ThreadID)
			if err != nil {
				return err
			}
			changeSetID = last
		}
	}

	st.Status = "RUNNING"
	st.CurrentStep = "INTAKE"
	st.PendingInt = nil
	st.PendingTool = nil
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	if st.Data == nil {
		st.Data = map[string]any{}
	}
	st.Data["resume"] = req.Resume.Payload
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	if err := emitter.Emit(Event{"type": "STEP_STARTED", "stepName": "INTAKE"}); err != nil {
		return err
	}

	// 记录决策（可选：由 thread sink 落盘并推进 ChangeSet 生命周期/线程状态）
	if applier, ok := h.threadSink.(ChangeSetApplier); ok && strings.TrimSpace(changeSetID) != "" {
		if err := applier.ApplyChangeSet(req.ThreadID, req.RunID, changeSetID, req.Resume.Payload); err != nil {
			return err
		}
	}

	// 产物：写一个 intake 回执到 run 目录（events 以外）
	outRel := filepathToSlash("events/intake_receipt.json")
	b, _ := json.MarshalIndent(map[string]any{
		"schema_version": 1,
		"thread_id":      req.ThreadID,
		"run_id":         req.RunID,
		"interrupt_id":   req.Resume.InterruptID,
		"change_set_id":  changeSetID,
		"payload":        req.Resume.Payload,
		"applied_at":     time.Now().Format(time.RFC3339),
	}, "", "  ")
	b = append(b, '\n')
	if err := gw.ReplaceFile(outRel, b, 0o644, "task: write intake receipt"); err != nil {
		return err
	}

	if err := emitter.Emit(Event{"type": "STEP_FINISHED", "stepName": "INTAKE"}); err != nil {
		return err
	}

	st.Status = "DONE"
	st.CurrentStep = ""
	st.PendingInt = nil
	st.PendingTool = nil
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	return emitter.Emit(Event{
		"type":    "RUN_FINISHED",
		"outcome": "success",
		"result":  map[string]any{"ok": true, "changeSetId": changeSetID},
	})
}

func (h *Handler) runTaskWorkStep(ctx context.Context, gw *gateway.Gateway, emitter *Emitter, st *RunState, stepName string, msg string) error {
	st.CurrentStep = stepName
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := writeRunState(gw, *st); err != nil {
		return err
	}
	if err := emitter.Emit(Event{"type": "STEP_STARTED", "stepName": stepName}); err != nil {
		return err
	}

	messageID := newMessageID(time.Now())
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_START", "messageId": messageID, "role": "assistant"})
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_CONTENT", "messageId": messageID, "delta": msg})
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_END", "messageId": messageID})

	if err := sleepContext(ctx, 120*time.Millisecond); err != nil {
		return err
	}

	return emitter.Emit(Event{"type": "STEP_FINISHED", "stepName": stepName})
}

func (h *Handler) checkpointMaybeInterrupt(ctx context.Context, gw *gateway.Gateway, emitter *Emitter, st *RunState, req RunRequest) (bool, error) {
	src, ok := h.threadSink.(ThreadStatusSource)
	if !ok {
		return false, nil
	}

	phase, lastChangeSetID, err := src.ThreadStatus(req.ThreadID)
	if err != nil {
		return false, err
	}
	if !strings.EqualFold(strings.TrimSpace(phase), "PAUSE_REQUESTED") {
		return false, nil
	}

	interruptID := newInterruptID(time.Now())
	st.Status = "INTERRUPTED"
	st.CurrentStep = "WAIT_RESUME"
	st.PendingTool = nil
	st.PendingInt = &Interrupt{
		ID:        interruptID,
		Reason:    "pause_requested",
		Payload:   map[string]any{"changeSetId": lastChangeSetID},
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := writeRunState(gw, *st); err != nil {
		return false, err
	}

	if err := emitter.Emit(Event{
		"type":      "RUN_FINISHED",
		"outcome":   "interrupt",
		"interrupt": st.PendingInt,
	}); err != nil {
		return false, err
	}

	return true, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func filepathToSlash(p string) string {
	// 避免在 agui 包里引入 filepath，仅用于固定相对路径。
	return strings.ReplaceAll(p, "\\", "/")
}
