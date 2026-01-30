package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/gateway"
)

func (h *Handler) runNew(ctx context.Context, gw *gateway.Gateway, emitter *Emitter, st *RunState, req RunRequest, session *Session) error {
	switch req.Workflow {
	case "demo":
		return h.runDemoNew(ctx, gw, emitter, st, req, session)
	case "ping":
		return h.runPing(ctx, gw, emitter, st, req)
	case "task":
		return h.runTaskNew(ctx, gw, emitter, st, req, session)
	default:
		return fmt.Errorf("unknown workflow: %s", req.Workflow)
	}
}

func (h *Handler) runResume(ctx context.Context, gw *gateway.Gateway, emitter *Emitter, st *RunState, req RunRequest, session *Session) error {
	switch req.Workflow {
	case "demo":
		return h.runDemoResume(ctx, gw, emitter, st, req, session)
	case "ping":
		return h.runPing(ctx, gw, emitter, st, req)
	case "task":
		return h.runTaskResume(ctx, gw, emitter, st, req, session)
	default:
		return fmt.Errorf("unknown workflow: %s", req.Workflow)
	}
}

func (h *Handler) runDemoNew(ctx context.Context, gw *gateway.Gateway, emitter *Emitter, st *RunState, req RunRequest, session *Session) error {
	// STEP: COLLECT（通过 activity_message 承载 A2UI）
	st.CurrentStep = "COLLECT"
	st.Status = "RUNNING"
	if err := writeRunState(gw, *st); err != nil {
		return err
	}
	if err := emitter.Emit(Event{"type": "STEP_STARTED", "stepName": "COLLECT"}); err != nil {
		return err
	}

	messageID := newMessageID(time.Now())
	if err := emitter.Emit(Event{"type": "TEXT_MESSAGE_START", "messageId": messageID, "role": "assistant"}); err != nil {
		return err
	}
	if err := emitter.Emit(Event{"type": "TEXT_MESSAGE_CONTENT", "messageId": messageID, "delta": "请在 UI 里选择一个选项（A 或 B）。\n"}); err != nil {
		return err
	}
	if err := emitter.Emit(Event{"type": "TEXT_MESSAGE_END", "messageId": messageID}); err != nil {
		return err
	}

	toolCallID := newToolCallID(time.Now())
	toolName := strings.TrimSpace(firstString(req.Raw, "toolCallName", "tool_call_name", "toolName", "tool_name"))
	if toolName == "" {
		toolName = "a2ui.message"
	}

	createSurface := map[string]any{
		"createSurface": map[string]any{
			"surfaceId": "main",
			"theme": map[string]any{
				"agentDisplayName": "zhanggui demo",
			},
		},
	}
	updateComponents := map[string]any{
		"updateComponents": map[string]any{
			"surfaceId": "main",
			"components": []any{
				map[string]any{"id": "root", "component": "Column", "children": map[string]any{"array": []any{"title", "choices", "choice_a", "choice_b"}}},
				map[string]any{"id": "title", "component": "Text", "text": "A2UI demo choices"},
				map[string]any{"id": "choices", "component": "Text", "text": "请选择一个选项："},
				map[string]any{
					"id":        "choice_a",
					"component": "Button",
					"text":      "选项 A",
					"variant":   "primary",
					"action": map[string]any{
						"name": "choose_a",
						"context": map[string]any{
							"source":     "a2ui_demo",
							"runId":      req.RunID,
							"threadId":   req.ThreadID,
							"toolCallId": toolCallID,
						},
					},
				},
				map[string]any{
					"id":        "choice_b",
					"component": "Button",
					"text":      "选项 B",
					"variant":   "primary",
					"action": map[string]any{
						"name": "choose_b",
						"context": map[string]any{
							"source":     "a2ui_demo",
							"runId":      req.RunID,
							"threadId":   req.ThreadID,
							"toolCallId": toolCallID,
						},
					},
				},
			},
		},
	}
	updateDataModel := map[string]any{
		"updateDataModel": map[string]any{
			"surfaceId": "main",
			"path":      "/state",
			"value":     map[string]any{},
		},
	}

	spec := "a2ui"
	version := "0.9"
	messages := []any{createSurface, updateComponents, updateDataModel}
	var rawArgs map[string]any
	if v := asMap(req.Raw["toolArgs"]); v != nil {
		rawArgs = v
	} else if v := asMap(req.Raw["tool_args"]); v != nil {
		rawArgs = v
	} else if v, ok := req.Raw["toolArgs"].(string); ok {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil && len(parsed) > 0 {
			rawArgs = parsed
		}
	}

	content := map[string]any{
		"spec":     spec,
		"version":  version,
		"messages": messages,
	}
	if rawArgs != nil {
		if s := strings.TrimSpace(firstString(rawArgs, "spec")); s != "" {
			spec = s
		}
		if v := strings.TrimSpace(firstString(rawArgs, "version")); v != "" {
			version = v
		}
		if ms, ok := rawArgs["messages"].([]any); ok {
			content["messages"] = ms
		} else if msg := asMap(rawArgs["message"]); msg != nil {
			content["messages"] = []any{msg}
		} else if _, ok := rawArgs["messages"]; ok {
			content["messages"] = rawArgs["messages"]
		} else if _, ok := rawArgs["message"]; ok {
			content["messages"] = rawArgs["message"]
		} else {
			content = rawArgs
		}
		content["spec"] = spec
		content["version"] = version
	}

	st.PendingTool = &PendingToolCall{
		ID:        toolCallID,
		Name:      toolName,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	if err := emitter.Emit(Event{"type": "activity_message", "content": content}); err != nil {
		return err
	}

	var tr ToolResult
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case got, ok := <-session.ToolResults:
			if !ok {
				return fmt.Errorf("tool result channel closed")
			}
			if got.ToolCallID != "" && got.ToolCallID != toolCallID {
				continue
			}
			tr = got
		}
		break
	}

	if st.Data == nil {
		st.Data = map[string]any{}
	}
	st.Data["choice"] = tr.Content
	st.PendingTool = nil
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	msgA2 := newMessageID(time.Now())
	_ = emitter.Emit(Event{
		"type":      "TEXT_MESSAGE_START",
		"messageId": msgA2,
		"role":      "assistant",
	})
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_CONTENT", "messageId": msgA2, "delta": "已收到 A2UI action 回传。\n"})
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_END", "messageId": msgA2})

	if err := emitter.Emit(Event{"type": "STEP_FINISHED", "stepName": "COLLECT"}); err != nil {
		return err
	}

	// STEP: PROCESS
	st.CurrentStep = "PROCESS"
	if err := writeRunState(gw, *st); err != nil {
		return err
	}
	if err := emitter.Emit(Event{"type": "STEP_STARTED", "stepName": "PROCESS"}); err != nil {
		return err
	}
	msg2 := newMessageID(time.Now())
	if err := emitter.Emit(Event{"type": "TEXT_MESSAGE_START", "messageId": msg2, "role": "assistant"}); err != nil {
		return err
	}
	if err := emitter.Emit(Event{"type": "TEXT_MESSAGE_CONTENT", "messageId": msg2, "delta": "已收到用户选择，进入 interrupt 演示：将请求审批后继续。\n"}); err != nil {
		return err
	}
	if err := emitter.Emit(Event{"type": "TEXT_MESSAGE_END", "messageId": msg2}); err != nil {
		return err
	}
	if err := emitter.Emit(Event{"type": "STEP_FINISHED", "stepName": "PROCESS"}); err != nil {
		return err
	}

	// INTERRUPT：要求人审（resume 继续）
	interruptID := newInterruptID(time.Now())
	st.Status = "INTERRUPTED"
	st.CurrentStep = "WAIT_APPROVAL"
	st.PendingInt = &Interrupt{
		ID:        interruptID,
		Reason:    "human_approval",
		Payload:   map[string]any{"title": "demo：请审批是否继续", "data": st.Data},
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	return emitter.Emit(Event{
		"type":      "RUN_FINISHED",
		"outcome":   "interrupt",
		"interrupt": st.PendingInt,
	})
}

func (h *Handler) runPing(ctx context.Context, gw *gateway.Gateway, emitter *Emitter, st *RunState, req RunRequest) error {
	st.CurrentStep = "PING"
	st.Status = "RUNNING"
	if err := writeRunState(gw, *st); err != nil {
		return err
	}
	if err := emitter.Emit(Event{"type": "STEP_STARTED", "stepName": "PING"}); err != nil {
		return err
	}

	msg := newMessageID(time.Now())
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_START", "messageId": msg, "role": "assistant"})
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_CONTENT", "messageId": msg, "delta": "ping：最小回合（用于演示 thread watch 的 delta）。\n"})
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_END", "messageId": msg})

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := emitter.Emit(Event{"type": "STEP_FINISHED", "stepName": "PING"}); err != nil {
		return err
	}

	st.Status = "DONE"
	st.CurrentStep = ""
	st.PendingInt = nil
	st.PendingTool = nil
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	return emitter.Emit(Event{
		"type":    "RUN_FINISHED",
		"outcome": "success",
		"result":  map[string]any{"ok": true},
	})
}

func (h *Handler) runDemoResume(ctx context.Context, gw *gateway.Gateway, emitter *Emitter, st *RunState, req RunRequest, session *Session) error {
	if req.Resume == nil || req.Resume.InterruptID == "" {
		return fmt.Errorf("missing resume.interruptId")
	}

	parentRunID, parentState, err := findInterruptedRun(h.runsDir, req.Resume.InterruptID)
	if err != nil {
		return err
	}

	// 继承上一次 run 的 data，并记录本次 resume payload（demo）
	if st.Data == nil {
		st.Data = map[string]any{}
	}
	for k, v := range parentState.Data {
		st.Data[k] = v
	}
	st.Data["resume"] = req.Resume.Payload
	st.Data["parentRunId"] = parentRunID

	st.Status = "RUNNING"
	st.CurrentStep = "FINALIZE"
	st.PendingInt = nil
	st.PendingTool = nil
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	if err := emitter.Emit(Event{"type": "STEP_STARTED", "stepName": "FINALIZE"}); err != nil {
		return err
	}

	msg := newMessageID(time.Now())
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_START", "messageId": msg, "role": "assistant"})
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_CONTENT", "messageId": msg, "delta": "已收到 resume payload，demo 结束。\n"})
	_ = emitter.Emit(Event{"type": "TEXT_MESSAGE_END", "messageId": msg})

	// 产物：写一个小文件，证明可通过 Tool Gateway 落盘（events 之外）
	outRel := filepath.ToSlash(filepath.Join("events", "result.json"))
	b, _ := json.MarshalIndent(map[string]any{
		"schema_version": 1,
		"parent_run_id":  parentRunID,
		"run_id":         req.RunID,
		"thread_id":      req.ThreadID,
		"data":           st.Data,
	}, "", "  ")
	b = append(b, '\n')
	if err := gw.ReplaceFile(outRel, b, 0o644, "demo: write result.json"); err != nil {
		return err
	}

	if err := emitter.Emit(Event{"type": "STEP_FINISHED", "stepName": "FINALIZE"}); err != nil {
		return err
	}

	st.Status = "DONE"
	st.CurrentStep = ""
	if err := writeRunState(gw, *st); err != nil {
		return err
	}

	return emitter.Emit(Event{
		"type":    "RUN_FINISHED",
		"outcome": "success",
		"result":  map[string]any{"ok": true, "parentRunId": parentRunID},
	})
}
