package teamleader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
)

type eventSubscriber interface {
	Subscribe() chan core.Event
	Unsubscribe(ch chan core.Event)
}

type eventPublisher interface {
	Publish(evt core.Event)
}

type RunRef struct {
	sessionID string
	issueID   string
}

type readyDispatch struct {
	sessionID string
	issueID   string
}

type runningSession struct {
	SessionID string
	ProjectID string
	Running   map[string]string
	IssueByID map[string]*core.Issue
	HaltNew   bool
	Recovered bool
}

func newRunningSession(sessionID, projectID string, issues []*core.Issue) *runningSession {
	issueByID := make(map[string]*core.Issue, len(issues))
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		issueByID[issue.ID] = issue
	}

	return &runningSession{
		SessionID: sessionID,
		ProjectID: projectID,
		Running:   make(map[string]string),
		IssueByID: issueByID,
	}
}

// DepScheduler schedules issues by profile queue and maps each issue to one Run.
type DepScheduler struct {
	store   core.Store
	bus     eventSubscriber
	pub     eventPublisher
	tracker core.Tracker

	runRun     func(context.Context, string) error
	sem        chan struct{}
	stageRoles map[core.StageID]string

	mu            sync.Mutex
	sessions      map[string]*runningSession
	RunIndex      map[string]RunRef
	lastSessionID string

	loopCancel  context.CancelFunc
	loopWG      sync.WaitGroup
	reconcileWG sync.WaitGroup

	reconcileInterval time.Duration
	reconcileRun      func(context.Context) error
}

// SetStageRoles configures the role mapping for run stages.
func (s *DepScheduler) SetStageRoles(roles map[string]string) {
	if s == nil || len(roles) == 0 {
		return
	}
	m := make(map[core.StageID]string, len(roles))
	for k, v := range roles {
		stage := core.StageID(strings.TrimSpace(k))
		role := strings.TrimSpace(v)
		if stage != "" && role != "" {
			m[stage] = role
		}
	}
	s.stageRoles = m
}

func NewDepScheduler(
	store core.Store,
	bus eventSubscriber,
	runRun func(context.Context, string) error,
	tracker core.Tracker,
	maxConcurrent int,
) *DepScheduler {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if runRun == nil {
		runRun = func(context.Context, string) error { return nil }
	}

	var pub eventPublisher
	if typed, ok := bus.(eventPublisher); ok {
		pub = typed
	}

	return &DepScheduler{
		store:    store,
		bus:      bus,
		pub:      pub,
		tracker:  tracker,
		runRun:   runRun,
		sem:      make(chan struct{}, maxConcurrent),
		sessions: make(map[string]*runningSession),
		RunIndex: make(map[string]RunRef),
	}
}

// SetReconcileRunner configures periodic reconcile hook for status drift repair.
func (s *DepScheduler) SetReconcileRunner(interval time.Duration, run func(context.Context) error) {
	if s == nil {
		return
	}
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileInterval = interval
	s.reconcileRun = run
}

func (s *DepScheduler) Start(ctx context.Context) error {
	if s == nil || s.bus == nil {
		return nil
	}

	s.mu.Lock()
	if s.loopCancel != nil {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.loopCancel = cancel
	ch := s.bus.Subscribe()
	reconcileRun := s.reconcileRun
	reconcileInterval := s.reconcileInterval
	if reconcileInterval <= 0 {
		reconcileInterval = 10 * time.Minute
	}
	s.loopWG.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.loopWG.Done()
		defer s.bus.Unsubscribe(ch)
		for {
			select {
			case <-runCtx.Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				_ = s.OnEvent(context.Background(), evt)
			}
		}
	}()

	if reconcileRun != nil {
		s.reconcileWG.Add(1)
		go func(runCtx context.Context, interval time.Duration, runFn func(context.Context) error) {
			defer s.reconcileWG.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					_ = runFn(context.Background())
				}
			}
		}(runCtx, reconcileInterval, reconcileRun)
	}

	return nil
}

func (s *DepScheduler) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	cancel := s.loopCancel
	s.loopCancel = nil
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.loopWG.Wait()
		s.reconcileWG.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ScheduleIssues puts issues into profile-aware ready queues and dispatches runnable items.
func (s *DepScheduler) ScheduleIssues(ctx context.Context, issues []*core.Issue) error {
	if s == nil || s.store == nil {
		return errors.New("scheduler store is not configured")
	}
	if len(issues) == 0 {
		return nil
	}

	grouped, err := groupIssuesBySession(issues)
	if err != nil {
		return err
	}

	for _, sessionID := range sortedSessionIDs(grouped) {
		if err := s.scheduleSession(ctx, sessionID, grouped[sessionID]); err != nil {
			return err
		}
	}
	return nil
}

func (s *DepScheduler) scheduleSession(ctx context.Context, sessionID string, issues []*core.Issue) error {
	if len(issues) == 0 {
		return nil
	}

	projectID := strings.TrimSpace(issues[0].ProjectID)
	rs := newRunningSession(sessionID, projectID, issues)

	for _, issueID := range sortedIssueIDs(rs.IssueByID) {
		issue := rs.IssueByID[issueID]
		if issue == nil {
			continue
		}
		if issue.FailPolicy == "" {
			issue.FailPolicy = core.FailBlock
		}
		if isIssueTerminal(issue.Status) {
			continue
		}

		switch issue.Status {
		case core.IssueStatusExecuting:
			if strings.TrimSpace(issue.RunID) == "" {
				issue.Status = core.IssueStatusQueued
			} else {
				rs.Running[issueID] = issue.RunID
			}
		case core.IssueStatusReady, core.IssueStatusQueued:
		default:
			issue.Status = core.IssueStatusQueued
			issue.RunID = ""
		}

		if err := s.saveIssue(issue); err != nil {
			return err
		}
		if issue.Status == core.IssueStatusQueued {
			s.publishIssueEvent(core.EventIssueQueued, issue, nil, "")
		}
	}
	if err := s.markReadyByProfileQueueLocked(rs); err != nil {
		return err
	}

	if err := s.registerSessionRuntime(sessionID, rs); err != nil {
		return err
	}
	return s.dispatchReadyAcrossSessions(ctx)
}

// RecoverExecutingIssues is the crash-recovery entrypoint in issue semantics.
func (s *DepScheduler) RecoverExecutingIssues(ctx context.Context, projectID string) error {
	if s == nil || s.store == nil {
		return errors.New("scheduler store is not configured")
	}

	active, err := s.store.GetActiveIssues(strings.TrimSpace(projectID))
	if err != nil {
		return err
	}
	if len(active) == 0 {
		return nil
	}

	ptrs := make([]*core.Issue, 0, len(active))
	for i := range active {
		ptrs = append(ptrs, &active[i])
	}

	grouped, err := groupIssuesBySession(ptrs)
	if err != nil {
		return err
	}

	for _, sessionID := range sortedSessionIDs(grouped) {
		if err := s.recoverSession(ctx, sessionID, grouped[sessionID]); err != nil {
			return err
		}
	}
	return nil
}

func (s *DepScheduler) recoverSession(ctx context.Context, sessionID string, issues []*core.Issue) error {
	if len(issues) == 0 {
		return nil
	}

	projectID := strings.TrimSpace(issues[0].ProjectID)
	rs := newRunningSession(sessionID, projectID, issues)
	rs.Recovered = true
	replayEvents := make([]core.Event, 0)

	for _, issueID := range sortedIssueIDs(rs.IssueByID) {
		issue := rs.IssueByID[issueID]
		if issue == nil {
			continue
		}
		if issue.FailPolicy == "" {
			issue.FailPolicy = core.FailBlock
		}

		switch issue.Status {
		case core.IssueStatusDone:
		case core.IssueStatusExecuting:
			if strings.TrimSpace(issue.RunID) == "" {
				issue.Status = core.IssueStatusQueued
				if err := s.saveIssue(issue); err != nil {
					return err
				}
				continue
			}
			Run, getErr := s.store.GetRun(issue.RunID)
			if getErr != nil {
				return fmt.Errorf("recover issue %s Run %s: %w", issueID, issue.RunID, getErr)
			}
			rs.Running[issueID] = issue.RunID
			if evtType, terminal := RunRecoveryEvent(Run.Status, Run.Conclusion); terminal {
				replayEvents = append(replayEvents, core.Event{
					Type:      evtType,
					RunID:     issue.RunID,
					Error:     Run.ErrorMessage,
					Timestamp: time.Now(),
				})
			}
		case core.IssueStatusReady, core.IssueStatusQueued:
		default:
			if isIssueTerminal(issue.Status) {
				continue
			}
			issue.Status = core.IssueStatusQueued
			issue.RunID = ""
			if err := s.saveIssue(issue); err != nil {
				return err
			}
			s.publishIssueEvent(core.EventIssueQueued, issue, nil, "")
		}
	}

	if err := s.markReadyByProfileQueueLocked(rs); err != nil {
		return err
	}
	if err := s.registerSessionRuntime(sessionID, rs); err != nil {
		return err
	}

	for i := range replayEvents {
		if err := s.OnEvent(ctx, replayEvents[i]); err != nil {
			return err
		}
	}
	if rs.HaltNew {
		return nil
	}
	return s.dispatchReadyAcrossSessions(ctx)
}

func (s *DepScheduler) registerSessionRuntime(sessionID string, rs *runningSession) error {
	if rs == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, exists := s.sessions[sessionID]; exists && existing != nil {
		for issueID, issue := range rs.IssueByID {
			if issue == nil {
				continue
			}
			existing.IssueByID[issueID] = issue
		}
		for issueID, RunID := range rs.Running {
			if strings.TrimSpace(RunID) == "" {
				continue
			}
			existing.Running[issueID] = RunID
		}
		rs = existing
	} else {
		s.sessions[sessionID] = rs
	}

	for issueID, RunID := range rs.Running {
		if strings.TrimSpace(RunID) == "" {
			continue
		}
		if _, exists := s.RunIndex[RunID]; exists {
			continue
		}
		s.RunIndex[RunID] = RunRef{sessionID: sessionID, issueID: issueID}
		if !s.tryAcquireSlot() {
			delete(s.RunIndex, RunID)
			return fmt.Errorf("recover session %s exceeds max concurrency %d", sessionID, cap(s.sem))
		}
	}
	return nil
}

// OnEvent handles run_done/run_failed events and advances Issue state.
func (s *DepScheduler) OnEvent(ctx context.Context, evt core.Event) error {
	if s == nil {
		return nil
	}
	if evt.Type != core.EventRunDone && evt.Type != core.EventRunFailed {
		return nil
	}
	if strings.TrimSpace(evt.RunID) == "" {
		return nil
	}

	s.mu.Lock()
	err := s.handleRunEventLocked(evt)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.dispatchReadyAcrossSessions(ctx)
}

func (s *DepScheduler) handleRunEventLocked(evt core.Event) error {
	ref, ok := s.RunIndex[evt.RunID]
	if !ok {
		issue, err := s.store.GetIssueByRun(evt.RunID)
		if err != nil || issue == nil {
			return err
		}
		sessionID := makeSessionID(issue.ProjectID, issue.SessionID)
		rs := s.sessions[sessionID]
		if rs == nil {
			return nil
		}
		if _, exists := rs.IssueByID[issue.ID]; !exists {
			return nil
		}
		ref = RunRef{sessionID: sessionID, issueID: issue.ID}
		s.RunIndex[evt.RunID] = ref
	}

	rs := s.sessions[ref.sessionID]
	if rs == nil {
		delete(s.RunIndex, evt.RunID)
		return nil
	}

	issue := rs.IssueByID[ref.issueID]
	if issue == nil {
		delete(s.RunIndex, evt.RunID)
		delete(rs.Running, ref.issueID)
		s.releaseSlot()
		return nil
	}

	switch evt.Type {
	case core.EventRunDone:
		issue.Status = core.IssueStatusDone
		if err := s.saveIssue(issue); err != nil {
			return err
		}
		s.publishIssueEvent(core.EventIssueDone, issue, nil, "")
	case core.EventRunFailed:
		issue.Status = core.IssueStatusFailed
		if err := s.saveIssue(issue); err != nil {
			return err
		}
		s.publishIssueEvent(core.EventIssueFailed, issue, nil, evt.Error)
		switch issue.FailPolicy {
		case core.FailSkip:
		case core.FailHuman:
			rs.HaltNew = true
		default:
			if err := s.applyBlockPolicyLocked(rs, issue.ID); err != nil {
				return err
			}
		}
	default:
		return nil
	}

	if err := s.markReadyByProfileQueueLocked(rs); err != nil {
		return err
	}

	if _, running := rs.Running[ref.issueID]; running {
		delete(rs.Running, ref.issueID)
		s.releaseSlot()
	}
	delete(s.RunIndex, evt.RunID)
	return nil
}

func (s *DepScheduler) applyBlockPolicyLocked(rs *runningSession, failedIssueID string) error {
	rs.HaltNew = true
	for _, issueID := range sortedIssueIDs(rs.IssueByID) {
		if issueID == failedIssueID {
			continue
		}
		issue := rs.IssueByID[issueID]
		if issue == nil || isIssueTerminal(issue.Status) || issue.Status == core.IssueStatusExecuting {
			continue
		}
		issue.Status = core.IssueStatusFailed
		issue.RunID = ""
		if err := s.saveIssue(issue); err != nil {
			return err
		}
		s.publishIssueEvent(core.EventIssueFailed, issue, map[string]string{
			"reason":         "blocked_by_session_failure",
			"cause_issue_id": failedIssueID,
		}, "")
	}
	return nil
}

func (s *DepScheduler) markReadyByProfileQueueLocked(rs *runningSession) error {
	if rs == nil {
		return nil
	}

	queuedByProfile := map[core.WorkflowProfileType][]string{
		core.WorkflowProfileStrict:      {},
		core.WorkflowProfileNormal:      {},
		core.WorkflowProfileFastRelease: {},
	}

	for _, issueID := range sortedIssueIDs(rs.IssueByID) {
		issue := rs.IssueByID[issueID]
		if issue == nil || issue.Status != core.IssueStatusQueued {
			continue
		}
		profile := workflowProfileFromIssue(issue)
		queuedByProfile[profile] = append(queuedByProfile[profile], issueID)
	}

	for _, profile := range workflowDispatchProfileOrder() {
		queue := queuedByProfile[profile]
		if len(queue) == 0 {
			continue
		}
		sort.Strings(queue)
		for _, issueID := range queue {
			issue := rs.IssueByID[issueID]
			if issue == nil || issue.Status != core.IssueStatusQueued {
				continue
			}
			issue.Status = core.IssueStatusReady
			if err := s.saveIssue(issue); err != nil {
				return err
			}
			s.publishIssueEvent(core.EventIssueReady, issue, map[string]string{
				"workflow_profile": string(profile),
			}, "")
		}
	}
	return nil
}

func (s *DepScheduler) dispatchIssue(ctx context.Context, sessionID, issueID string) (bool, error) {
	if s == nil || s.store == nil {
		return false, errors.New("scheduler store is not configured")
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(issueID) == "" {
		return false, errors.New("session id and issue id are required")
	}

	s.mu.Lock()
	rs := s.sessions[sessionID]
	if rs == nil {
		s.mu.Unlock()
		return false, fmt.Errorf("session %s is not running", sessionID)
	}
	if rs.HaltNew {
		s.mu.Unlock()
		return false, nil
	}
	issue := rs.IssueByID[issueID]
	if issue == nil {
		s.mu.Unlock()
		return false, fmt.Errorf("issue %s not found in session %s", issueID, sessionID)
	}
	if issue.Status != core.IssueStatusReady {
		s.mu.Unlock()
		return false, nil
	}
	if _, running := rs.Running[issueID]; running {
		s.mu.Unlock()
		return false, nil
	}
	if !s.tryAcquireSlot() {
		s.mu.Unlock()
		return false, nil
	}

	profile := workflowProfileFromIssue(issue)
	Run, err := buildRunFromIssue(issue, profile, s.stageRoles)
	if err != nil {
		s.releaseSlot()
		s.mu.Unlock()
		return false, err
	}

	issue.Status = core.IssueStatusExecuting
	issue.RunID = Run.ID
	rs.Running[issueID] = Run.ID
	s.RunIndex[Run.ID] = RunRef{sessionID: sessionID, issueID: issueID}
	s.lastSessionID = sessionID
	s.mu.Unlock()

	if err := s.store.SaveRun(Run); err != nil {
		s.rollbackDispatch(sessionID, issueID, Run.ID)
		return false, err
	}
	if err := s.saveIssue(issue); err != nil {
		s.rollbackDispatch(sessionID, issueID, Run.ID)
		return false, err
	}
	s.publishIssueEvent(core.EventIssueExecuting, issue, map[string]string{
		"workflow_profile": string(profile),
	}, "")

	runCtx := context.Background()
	if ctx != nil {
		runCtx = context.WithoutCancel(ctx)
	}
	go func(runCtx context.Context, RunID string) {
		if runErr := s.runRun(runCtx, RunID); runErr != nil {
			_ = s.OnEvent(context.Background(), core.Event{
				Type:      core.EventRunFailed,
				RunID:     RunID,
				Error:     runErr.Error(),
				Timestamp: time.Now(),
			})
		}
	}(runCtx, Run.ID)

	return true, nil
}

func (s *DepScheduler) rollbackDispatch(sessionID, issueID, RunID string) {
	var issue *core.Issue

	s.mu.Lock()
	rs := s.sessions[sessionID]
	if rs != nil {
		if candidate := rs.IssueByID[issueID]; candidate != nil &&
			candidate.Status == core.IssueStatusExecuting &&
			candidate.RunID == RunID {
			candidate.Status = core.IssueStatusReady
			candidate.RunID = ""
			issue = candidate
		}
		delete(rs.Running, issueID)
	}
	delete(s.RunIndex, RunID)
	s.releaseSlot()
	s.mu.Unlock()

	if issue != nil {
		_ = s.saveIssue(issue)
	}
}

func (s *DepScheduler) dispatchReadyAcrossSessions(ctx context.Context) error {
	if s == nil {
		return nil
	}

	for {
		s.mu.Lock()
		semLen, semCap := len(s.sem), cap(s.sem)
		if semCap > 0 && semLen >= semCap {
			slog.Info("dispatchReady: sem full", "len", semLen, "cap", semCap)
			s.mu.Unlock()
			return nil
		}
		candidates := s.globalReadyCandidatesLocked()
		sessCount := len(s.sessions)
		s.mu.Unlock()
		slog.Info("dispatchReady: candidates", "count", len(candidates), "sessions", sessCount, "sem", fmt.Sprintf("%d/%d", semLen, semCap))
		if len(candidates) == 0 {
			return nil
		}

		dispatchedAny := false
		for _, candidate := range candidates {
			dispatched, err := s.dispatchIssue(ctx, candidate.sessionID, candidate.issueID)
			if err != nil {
				slog.Error("dispatchReady: dispatchIssue failed", "session", candidate.sessionID, "issue", candidate.issueID, "error", err)
				return err
			}
			slog.Info("dispatchReady: dispatchIssue", "session", candidate.sessionID, "issue", candidate.issueID, "dispatched", dispatched)
			if dispatched {
				dispatchedAny = true
			}
		}
		if !dispatchedAny {
			return nil
		}
	}
}

func (s *DepScheduler) globalReadyCandidatesLocked() []readyDispatch {
	sessionIDs := make([]string, 0, len(s.sessions))
	readyBySession := make(map[string][]string, len(s.sessions))
	maxReady := 0

	for sessionID, rs := range s.sessions {
		if rs == nil || rs.HaltNew {
			continue
		}
		ready := rs.readyToDispatchIDs()
		if len(ready) == 0 {
			continue
		}
		sessionIDs = append(sessionIDs, sessionID)
		readyBySession[sessionID] = ready
		if len(ready) > maxReady {
			maxReady = len(ready)
		}
	}
	if len(sessionIDs) == 0 {
		return nil
	}

	sort.Strings(sessionIDs)
	start := 0
	if s.lastSessionID != "" {
		idx := sort.SearchStrings(sessionIDs, s.lastSessionID)
		if idx < len(sessionIDs) && sessionIDs[idx] == s.lastSessionID {
			start = (idx + 1) % len(sessionIDs)
		} else if idx < len(sessionIDs) {
			start = idx
		}
	}

	orderedSessionIDs := append([]string{}, sessionIDs[start:]...)
	orderedSessionIDs = append(orderedSessionIDs, sessionIDs[:start]...)

	candidates := make([]readyDispatch, 0, len(sessionIDs))
	for i := 0; i < maxReady; i++ {
		for _, sessionID := range orderedSessionIDs {
			ready := readyBySession[sessionID]
			if i >= len(ready) {
				continue
			}
			candidates = append(candidates, readyDispatch{sessionID: sessionID, issueID: ready[i]})
		}
	}
	return candidates
}

func (s *DepScheduler) saveIssue(issue *core.Issue) error {
	if issue == nil {
		return nil
	}
	issue.UpdatedAt = time.Now()
	if err := s.store.SaveIssue(issue); err != nil {
		return err
	}

	if s.tracker == nil {
		return nil
	}
	if strings.TrimSpace(issue.ExternalID) == "" {
		externalID, err := s.tracker.CreateIssue(context.Background(), issue)
		if err == nil && strings.TrimSpace(externalID) != "" {
			issue.ExternalID = externalID
			issue.UpdatedAt = time.Now()
			if saveErr := s.store.SaveIssue(issue); saveErr != nil {
				return saveErr
			}
		}
	}
	if strings.TrimSpace(issue.ExternalID) != "" {
		_ = s.tracker.UpdateStatus(context.Background(), issue.ExternalID, issue.Status)
	}
	return nil
}

func (s *DepScheduler) syncIssueDependencies(issues []*core.Issue) {
	// V2 removes runtime dependency orchestration; keep no-op hook to avoid
	// touching caller graph in this wave.
	_ = issues
}

func (s *DepScheduler) publishEvent(evt core.Event) {
	if s == nil || s.pub == nil {
		return
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	s.pub.Publish(evt)
}

func (s *DepScheduler) publishIssueEvent(eventType core.EventType, issue *core.Issue, data map[string]string, eventErr string) {
	if issue == nil {
		return
	}

	evtData := map[string]string{
		"issue_status": string(issue.Status),
	}
	for k, v := range data {
		evtData[k] = v
	}
	if eventErr != "" {
		evtData["error"] = eventErr
	}

	s.publishEvent(core.Event{
		Type:      eventType,
		RunID:     issue.RunID,
		ProjectID: issue.ProjectID,
		IssueID:   issue.ID,
		Data:      evtData,
		Error:     eventErr,
		Timestamp: time.Now(),
	})
}

func (s *DepScheduler) tryAcquireSlot() bool {
	select {
	case s.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *DepScheduler) releaseSlot() {
	select {
	case <-s.sem:
	default:
	}
}

// RunRecoveryEvent maps a terminal run's status+conclusion to an event for scheduler replay.
// All non-success conclusions (failure, timed_out, cancelled) map to EventRunFailed
// because the scheduler treats any non-success outcome identically: mark the issue failed
// and apply the session's fail policy.
func RunRecoveryEvent(status core.RunStatus, conclusion core.RunConclusion) (core.EventType, bool) {
	if status != core.StatusCompleted {
		return "", false
	}
	if conclusion == core.ConclusionSuccess {
		return core.EventRunDone, true
	}
	return core.EventRunFailed, true
}

func workflowDispatchProfileOrder() []core.WorkflowProfileType {
	return []core.WorkflowProfileType{
		core.WorkflowProfileStrict,
		core.WorkflowProfileNormal,
		core.WorkflowProfileFastRelease,
	}
}

func workflowProfileFromIssue(issue *core.Issue) core.WorkflowProfileType {
	if issue == nil {
		return core.WorkflowProfileNormal
	}
	for _, label := range issue.Labels {
		trimmed := strings.TrimSpace(strings.ToLower(label))
		if !strings.HasPrefix(trimmed, "profile:") {
			continue
		}
		candidate := core.WorkflowProfileType(strings.TrimSpace(strings.TrimPrefix(trimmed, "profile:")))
		if candidate.Validate() == nil {
			return candidate
		}
	}
	if candidate := core.WorkflowProfileType(strings.TrimSpace(strings.ToLower(issue.Template))); candidate.Validate() == nil {
		return candidate
	}
	return core.WorkflowProfileNormal
}

func (rs *runningSession) readyToDispatchIDs() []string {
	readyByProfile := map[core.WorkflowProfileType][]string{
		core.WorkflowProfileStrict:      {},
		core.WorkflowProfileNormal:      {},
		core.WorkflowProfileFastRelease: {},
	}
	for _, issueID := range sortedIssueIDs(rs.IssueByID) {
		issue := rs.IssueByID[issueID]
		if issue == nil {
			continue
		}
		if issue.Status != core.IssueStatusReady {
			continue
		}
		if _, running := rs.Running[issueID]; running {
			continue
		}
		profile := workflowProfileFromIssue(issue)
		readyByProfile[profile] = append(readyByProfile[profile], issueID)
	}
	ordered := make([]string, 0, len(rs.IssueByID))
	for _, profile := range workflowDispatchProfileOrder() {
		ids := readyByProfile[profile]
		sort.Strings(ids)
		ordered = append(ordered, ids...)
	}
	return ordered
}

func buildRunFromIssue(issue *core.Issue, profile core.WorkflowProfileType, stageRoles map[core.StageID]string) (*core.Run, error) {
	if issue == nil {
		return nil, errors.New("issue cannot be nil")
	}

	template := strings.TrimSpace(issue.Template)
	if template == "" {
		template = "standard"
	}
	stages, err := buildSchedulerStages(template, stageRoles)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(issue.Title)
	if name == "" {
		name = issue.ID
	}

	now := time.Now()
	return &core.Run{
		ID:          engine.NewRunID(),
		ProjectID:   issue.ProjectID,
		Name:        name,
		Description: issue.Body,
		Template:    template,
		Status:      core.StatusQueued,
		Stages:      stages,
		Artifacts:   map[string]string{},
		Config: map[string]any{
			"workflow_profile": string(profile),
		},
		IssueID:         issue.ID,
		MaxTotalRetries: 5,
		QueuedAt:        now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func buildSchedulerStages(template string, stageRoles map[core.StageID]string) ([]core.StageConfig, error) {
	stageIDs, ok := engine.Templates[template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", template)
	}

	stages := make([]core.StageConfig, len(stageIDs))
	for i, stageID := range stageIDs {
		stages[i] = schedulerDefaultStageConfig(stageID)
		if role, ok := stageRoles[stageID]; ok {
			stages[i].Role = role
		}
	}
	return stages, nil
}

func schedulerDefaultStageConfig(id core.StageID) core.StageConfig {
	cfg := core.StageConfig{
		Name:           id,
		PromptTemplate: string(id),
		Timeout:        30 * time.Minute,
		MaxRetries:     1,
		OnFailure:      core.OnFailureHuman,
	}

	switch id {
	case core.StageRequirements, core.StageReview:
		cfg.Agent = "codex"
	case core.StageImplement:
		cfg.Agent = "codex"
	case core.StageFixup:
		cfg.Agent = "codex"
		cfg.ReuseSessionFrom = core.StageImplement
	case core.StageTest:
		cfg.Agent = "codex"
		cfg.Timeout = 15 * time.Minute
	case core.StageSetup, core.StageMerge, core.StageCleanup:
		cfg.Timeout = 2 * time.Minute
	}
	return cfg
}

func isIssueTerminal(status core.IssueStatus) bool {
	switch status {
	case core.IssueStatusDone, core.IssueStatusFailed, core.IssueStatusSuperseded, core.IssueStatusAbandoned:
		return true
	default:
		return false
	}
}

func makeSessionID(projectID, sessionID string) string {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID != "" {
		return trimmedSessionID
	}
	return "project:" + strings.TrimSpace(projectID)
}

func groupIssuesBySession(issues []*core.Issue) (map[string][]*core.Issue, error) {
	grouped := make(map[string][]*core.Issue)
	sessionProject := make(map[string]string)

	for _, issue := range issues {
		if issue == nil {
			continue
		}
		issueID := strings.TrimSpace(issue.ID)
		projectID := strings.TrimSpace(issue.ProjectID)
		if issueID == "" {
			return nil, errors.New("issue id is required")
		}
		if projectID == "" {
			return nil, fmt.Errorf("issue %s project id is required", issueID)
		}

		issue.ID = issueID
		issue.ProjectID = projectID
		issue.SessionID = strings.TrimSpace(issue.SessionID)

		sessionID := makeSessionID(projectID, issue.SessionID)
		if existingProjectID, ok := sessionProject[sessionID]; ok && existingProjectID != projectID {
			return nil, fmt.Errorf("session %s has mixed project ids: %s vs %s", sessionID, existingProjectID, projectID)
		}
		sessionProject[sessionID] = projectID
		grouped[sessionID] = append(grouped[sessionID], issue)
	}

	if len(grouped) == 0 {
		return nil, errors.New("no issues provided")
	}
	return grouped, nil
}

func sortedSessionIDs(grouped map[string][]*core.Issue) []string {
	sessionIDs := make([]string, 0, len(grouped))
	for sessionID := range grouped {
		sessionIDs = append(sessionIDs, sessionID)
	}
	sort.Strings(sessionIDs)
	return sessionIDs
}

func sortedIssueIDs(issueByID map[string]*core.Issue) []string {
	ids := make([]string, 0, len(issueByID))
	for issueID := range issueByID {
		ids = append(ids, issueID)
	}
	sort.Strings(ids)
	return ids
}
