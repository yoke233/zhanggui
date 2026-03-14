package probe

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

var (
	ErrRunProbeConflict = errors.New("run already has an active probe")
	ErrRunNotRunning    = errors.New("run is not running")
)

const defaultRunProbeQuestion = "Execution Probe: report current status in one short reply. State whether you are actively progressing, blocked waiting for input/authorization, or hung. If blocked, say exactly what input or approval is needed."

type RunProbeService struct {
	store           Store
	bus             EventPublisher
	sessionManager  Runtime
	defaultQuestion string
}

type RunProbeServiceConfig struct {
	Store           Store
	Bus             EventPublisher
	SessionManager  Runtime
	DefaultQuestion string
}

func NewRunProbeService(cfg RunProbeServiceConfig) *RunProbeService {
	defaultQuestion := strings.TrimSpace(cfg.DefaultQuestion)
	if defaultQuestion == "" {
		defaultQuestion = defaultRunProbeQuestion
	}
	return &RunProbeService{
		store:           cfg.Store,
		bus:             cfg.Bus,
		sessionManager:  cfg.SessionManager,
		defaultQuestion: defaultQuestion,
	}
}

func (s *RunProbeService) ListRunProbes(ctx context.Context, runID int64) ([]*core.RunProbe, error) {
	signals, err := s.store.ListProbeSignalsByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	probes := make([]*core.RunProbe, 0, len(signals))
	for _, sig := range signals {
		probes = append(probes, core.ProbeFromSignal(sig))
	}
	return probes, nil
}

func (s *RunProbeService) GetLatestRunProbe(ctx context.Context, runID int64) (*core.RunProbe, error) {
	sig, err := s.store.GetLatestProbeSignal(ctx, runID)
	if err != nil {
		return nil, err
	}
	return core.ProbeFromSignal(sig), nil
}

func (s *RunProbeService) RequestRunProbe(ctx context.Context, runID int64, source core.RunProbeTriggerSource, question string, timeout time.Duration) (*core.RunProbe, error) {
	runRec, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if runRec.Status != core.RunRunning {
		return nil, ErrRunNotRunning
	}

	if active, err := s.store.GetActiveProbeSignal(ctx, runID); err == nil && active != nil {
		return nil, ErrRunProbeConflict
	} else if err != nil && !errors.Is(err, core.ErrNotFound) {
		return nil, err
	}

	route, err := s.store.GetRunProbeRoute(ctx, runID)
	if err != nil {
		return nil, err
	}

	trimmedQuestion := strings.TrimSpace(question)
	if trimmedQuestion == "" {
		trimmedQuestion = s.defaultQuestion
	}

	probe := &core.RunProbe{
		RunID:          runRec.ID,
		WorkItemID:     runRec.WorkItemID,
		ActionID:       runRec.ActionID,
		AgentContextID: route.AgentContextID,
		SessionID:      route.SessionID,
		OwnerID:        route.OwnerID,
		TriggerSource:  source,
		Question:       trimmedQuestion,
		Status:         core.RunProbePending,
		Verdict:        core.RunProbeUnknown,
	}

	// Create as probe_request signal.
	reqSignal := core.NewProbeRequestSignal(probe)
	sigID, err := s.store.CreateActionSignal(ctx, reqSignal)
	if err != nil {
		return nil, err
	}
	probe.ID = sigID
	s.publishProbeEvent(ctx, core.EventRunProbeRequested, probe)

	// Mark as sent.
	now := time.Now().UTC()
	probe.Status = core.RunProbeSent
	probe.SentAt = &now
	sentSignal := core.NewProbeRequestSignal(probe)
	sentSignal.ID = sigID
	if err := s.store.UpdateProbeSignal(ctx, sentSignal); err != nil {
		return nil, err
	}
	s.publishProbeEvent(ctx, core.EventRunProbeSent, probe)

	runtimeResult, err := s.sessionManager.ProbeRun(ctx, RunProbeRuntimeRequest{
		RunID:     runID,
		SessionID: route.SessionID,
		OwnerID:   route.OwnerID,
		Question:  trimmedQuestion,
		Timeout:   timeout,
	})
	if err != nil {
		probe.Status = core.RunProbeFailed
		probe.Error = err.Error()
		probe.Verdict = core.RunProbeUnknown
		failSignal := core.NewProbeResponseSignal(probe)
		failSignal.ID = sigID
		if updateErr := s.store.UpdateProbeSignal(ctx, failSignal); updateErr != nil {
			return nil, updateErr
		}
		return probe, nil
	}

	s.applyRuntimeResult(probe, runtimeResult)
	respSignal := core.NewProbeResponseSignal(probe)
	respSignal.ID = sigID
	if err := s.store.UpdateProbeSignal(ctx, respSignal); err != nil {
		return nil, err
	}
	s.publishTerminalProbeEvent(ctx, probe)
	return probe, nil
}

func (s *RunProbeService) applyRuntimeResult(probe *core.RunProbe, runtimeResult *RunProbeRuntimeResult) {
	probe.ReplyText = strings.TrimSpace(runtimeResult.ReplyText)
	probe.Error = strings.TrimSpace(runtimeResult.Error)

	observedAt := runtimeResult.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	switch {
	case !runtimeResult.Reachable:
		probe.Status = core.RunProbeUnreachable
		probe.Verdict = core.RunProbeUnknown
	case runtimeResult.Answered:
		probe.Status = core.RunProbeAnswered
		probe.AnsweredAt = &observedAt
		probe.Verdict = inferRunProbeVerdict(probe.ReplyText)
	case strings.Contains(strings.ToLower(runtimeResult.Error), "timeout"):
		probe.Status = core.RunProbeTimeout
		probe.Verdict = core.RunProbeHung
	default:
		probe.Status = core.RunProbeFailed
		probe.Verdict = core.RunProbeUnknown
	}
}

func inferRunProbeVerdict(reply string) core.RunProbeVerdict {
	lower := strings.ToLower(strings.TrimSpace(reply))
	if lower == "" {
		return core.RunProbeUnknown
	}
	blockedHints := []string{
		"need input", "need your input", "waiting for input", "waiting for approval",
		"authorization", "authorize", "approval", "permission", "login", "credential",
		"需要输入", "等待输入", "等待授权", "等待批准", "权限", "登录", "凭证",
	}
	for _, hint := range blockedHints {
		if strings.Contains(lower, hint) {
			return core.RunProbeBlocked
		}
	}
	return core.RunProbeAlive
}

func (s *RunProbeService) publishTerminalProbeEvent(ctx context.Context, probe *core.RunProbe) {
	switch probe.Status {
	case core.RunProbeAnswered:
		s.publishProbeEvent(ctx, core.EventRunProbeAnswered, probe)
	case core.RunProbeTimeout:
		s.publishProbeEvent(ctx, core.EventRunProbeTimeout, probe)
	case core.RunProbeUnreachable:
		s.publishProbeEvent(ctx, core.EventRunProbeUnreachable, probe)
	}
}

func (s *RunProbeService) publishProbeEvent(ctx context.Context, eventType core.EventType, probe *core.RunProbe) {
	if s.bus == nil || probe == nil {
		return
	}
	s.bus.Publish(ctx, core.Event{
		Type:       eventType,
		WorkItemID: probe.WorkItemID,
		ActionID:   probe.ActionID,
		RunID:      probe.RunID,
		Timestamp:  time.Now().UTC(),
		Data: map[string]any{
			"probe_id":       probe.ID,
			"run_id":         probe.RunID,
			"trigger_source": probe.TriggerSource,
			"status":         probe.Status,
			"verdict":        probe.Verdict,
		},
	})
}
