package probe

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

var (
	ErrExecutionProbeConflict = errors.New("execution already has an active probe")
	ErrExecutionNotRunning    = errors.New("execution is not running")
)

const defaultExecutionProbeQuestion = "Execution Probe: report current status in one short reply. State whether you are actively progressing, blocked waiting for input/authorization, or hung. If blocked, say exactly what input or approval is needed."

type ExecutionProbeService struct {
	store           Store
	bus             EventPublisher
	sessionManager  Runtime
	defaultQuestion string
}

type ExecutionProbeServiceConfig struct {
	Store           Store
	Bus             EventPublisher
	SessionManager  Runtime
	DefaultQuestion string
}

func NewExecutionProbeService(cfg ExecutionProbeServiceConfig) *ExecutionProbeService {
	defaultQuestion := strings.TrimSpace(cfg.DefaultQuestion)
	if defaultQuestion == "" {
		defaultQuestion = defaultExecutionProbeQuestion
	}
	return &ExecutionProbeService{
		store:           cfg.Store,
		bus:             cfg.Bus,
		sessionManager:  cfg.SessionManager,
		defaultQuestion: defaultQuestion,
	}
}

func (s *ExecutionProbeService) ListExecutionProbes(ctx context.Context, executionID int64) ([]*core.ExecutionProbe, error) {
	return s.store.ListExecutionProbesByExecution(ctx, executionID)
}

func (s *ExecutionProbeService) GetLatestExecutionProbe(ctx context.Context, executionID int64) (*core.ExecutionProbe, error) {
	return s.store.GetLatestExecutionProbe(ctx, executionID)
}

func (s *ExecutionProbeService) RequestExecutionProbe(ctx context.Context, executionID int64, source core.ExecutionProbeTriggerSource, question string, timeout time.Duration) (*core.ExecutionProbe, error) {
	execRec, err := s.store.GetExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	if execRec.Status != core.ExecRunning {
		return nil, ErrExecutionNotRunning
	}

	if active, err := s.store.GetActiveExecutionProbe(ctx, executionID); err == nil && active != nil {
		return nil, ErrExecutionProbeConflict
	} else if err != nil && !errors.Is(err, core.ErrNotFound) {
		return nil, err
	}

	route, err := s.store.GetExecutionProbeRoute(ctx, executionID)
	if err != nil {
		return nil, err
	}

	trimmedQuestion := strings.TrimSpace(question)
	if trimmedQuestion == "" {
		trimmedQuestion = s.defaultQuestion
	}

	probe := &core.ExecutionProbe{
		ExecutionID:    execRec.ID,
		IssueID:        execRec.IssueID,
		StepID:         execRec.StepID,
		AgentContextID: route.AgentContextID,
		SessionID:      route.SessionID,
		OwnerID:        route.OwnerID,
		TriggerSource:  source,
		Question:       trimmedQuestion,
		Status:         core.ExecutionProbePending,
		Verdict:        core.ExecutionProbeUnknown,
	}
	if _, err := s.store.CreateExecutionProbe(ctx, probe); err != nil {
		return nil, err
	}
	s.publishProbeEvent(ctx, core.EventExecProbeRequested, probe)

	now := time.Now().UTC()
	probe.Status = core.ExecutionProbeSent
	probe.SentAt = &now
	if err := s.store.UpdateExecutionProbe(ctx, probe); err != nil {
		return nil, err
	}
	s.publishProbeEvent(ctx, core.EventExecProbeSent, probe)

	runtimeResult, err := s.sessionManager.ProbeExecution(ctx, ExecutionProbeRuntimeRequest{
		ExecutionID: executionID,
		SessionID:   route.SessionID,
		OwnerID:     route.OwnerID,
		Question:    trimmedQuestion,
		Timeout:     timeout,
	})
	if err != nil {
		probe.Status = core.ExecutionProbeFailed
		probe.Error = err.Error()
		probe.Verdict = core.ExecutionProbeUnknown
		if updateErr := s.store.UpdateExecutionProbe(ctx, probe); updateErr != nil {
			return nil, updateErr
		}
		return probe, nil
	}

	s.applyRuntimeResult(probe, runtimeResult)
	if err := s.store.UpdateExecutionProbe(ctx, probe); err != nil {
		return nil, err
	}
	s.publishTerminalProbeEvent(ctx, probe)
	return probe, nil
}

func (s *ExecutionProbeService) applyRuntimeResult(probe *core.ExecutionProbe, runtimeResult *ExecutionProbeRuntimeResult) {
	probe.ReplyText = strings.TrimSpace(runtimeResult.ReplyText)
	probe.Error = strings.TrimSpace(runtimeResult.Error)

	observedAt := runtimeResult.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	switch {
	case !runtimeResult.Reachable:
		probe.Status = core.ExecutionProbeUnreachable
		probe.Verdict = core.ExecutionProbeUnknown
	case runtimeResult.Answered:
		probe.Status = core.ExecutionProbeAnswered
		probe.AnsweredAt = &observedAt
		probe.Verdict = inferExecutionProbeVerdict(probe.ReplyText)
	case strings.Contains(strings.ToLower(runtimeResult.Error), "timeout"):
		probe.Status = core.ExecutionProbeTimeout
		probe.Verdict = core.ExecutionProbeHung
	default:
		probe.Status = core.ExecutionProbeFailed
		probe.Verdict = core.ExecutionProbeUnknown
	}
}

func inferExecutionProbeVerdict(reply string) core.ExecutionProbeVerdict {
	lower := strings.ToLower(strings.TrimSpace(reply))
	if lower == "" {
		return core.ExecutionProbeUnknown
	}
	blockedHints := []string{
		"need input", "need your input", "waiting for input", "waiting for approval",
		"authorization", "authorize", "approval", "permission", "login", "credential",
		"需要输入", "等待输入", "等待授权", "等待批准", "权限", "登录", "凭证",
	}
	for _, hint := range blockedHints {
		if strings.Contains(lower, hint) {
			return core.ExecutionProbeBlocked
		}
	}
	return core.ExecutionProbeAlive
}

func (s *ExecutionProbeService) publishTerminalProbeEvent(ctx context.Context, probe *core.ExecutionProbe) {
	switch probe.Status {
	case core.ExecutionProbeAnswered:
		s.publishProbeEvent(ctx, core.EventExecProbeAnswered, probe)
	case core.ExecutionProbeTimeout:
		s.publishProbeEvent(ctx, core.EventExecProbeTimeout, probe)
	case core.ExecutionProbeUnreachable:
		s.publishProbeEvent(ctx, core.EventExecProbeUnreachable, probe)
	}
}

func (s *ExecutionProbeService) publishProbeEvent(ctx context.Context, eventType core.EventType, probe *core.ExecutionProbe) {
	if s.bus == nil || probe == nil {
		return
	}
	s.bus.Publish(ctx, core.Event{
		Type:      eventType,
		IssueID:   probe.IssueID,
		StepID:    probe.StepID,
		ExecID:    probe.ExecutionID,
		Timestamp: time.Now().UTC(),
		Data: map[string]any{
			"probe_id":       probe.ID,
			"execution_id":   probe.ExecutionID,
			"trigger_source": probe.TriggerSource,
			"status":         probe.Status,
			"verdict":        probe.Verdict,
		},
	})
}
