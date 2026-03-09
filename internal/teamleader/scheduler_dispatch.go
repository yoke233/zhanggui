package teamleader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *DepScheduler) dispatchIssue(ctx context.Context, sessionID, issueID string) (bool, error) {
	if s == nil || s.store == nil {
		return false, errors.New("scheduler store is not configured")
	}
	if ctx != nil && ctx.Err() != nil {
		return false, ctx.Err()
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

	if err := transitionIssueStatus(issue, core.IssueStatusExecuting); err != nil {
		s.releaseSlot()
		s.mu.Unlock()
		return false, err
	}
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
	s.recordTaskStep(issue, core.StepExecutionStarted, "system", "scheduler dispatched run")
	s.publishIssueEvent(core.EventIssueExecuting, issue, map[string]string{
		"workflow_profile": string(profile),
	}, "")

	runCtx := context.Background()
	if ctx != nil {
		runCtx = context.WithoutCancel(ctx)
	}
	runCtx, cancel := context.WithCancel(runCtx)
	s.registerRunCancel(Run.ID, cancel)
	go func(runCtx context.Context, RunID string) {
		defer s.forgetRunCancel(RunID)
		defer func() {
			if r := recover(); r != nil {
				slog.Error("run goroutine panicked, releasing slot", "run_id", RunID, "panic", r)
				_ = s.OnEvent(context.Background(), core.Event{
					Type:      core.EventRunFailed,
					RunID:     RunID,
					Error:     fmt.Sprintf("panic: %v", r),
					Timestamp: time.Now(),
				})
			}
		}()
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
			if err := transitionIssueStatus(candidate, core.IssueStatusReady); err == nil {
				candidate.RunID = ""
				issue = candidate
			}
		}
		delete(rs.Running, issueID)
	}
	delete(s.RunIndex, RunID)
	if cancel := s.runCancels[RunID]; cancel != nil {
		delete(s.runCancels, RunID)
		cancel()
	}
	s.releaseSlot()
	s.mu.Unlock()

	if issue != nil {
		_ = s.saveIssue(issue)
		s.recordTaskStep(issue, core.StepReady, "system", "dispatch rollback")
	}
}

func (s *DepScheduler) dispatchReadyAcrossSessions(ctx context.Context) error {
	if s == nil {
		return nil
	}

	for {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
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
		ready, err := s.dependenciesSatisfiedLocked(rs, issue)
		if err != nil {
			return err
		}
		if !ready {
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
			ready, err := s.dependenciesSatisfiedLocked(rs, issue)
			if err != nil {
				return err
			}
			if !ready {
				continue
			}
			if err := transitionIssueStatus(issue, core.IssueStatusReady); err != nil {
				return err
			}
			if err := s.saveIssue(issue); err != nil {
				return err
			}
			s.recordTaskStep(issue, core.StepReady, "system", "dependencies satisfied")
			s.publishIssueEvent(core.EventIssueReady, issue, map[string]string{
				"workflow_profile": string(profile),
			}, "")
		}
	}
	return nil
}

func areDependenciesMet(dependsOn []string, failPolicy core.FailurePolicy, lookup func(string) *core.Issue) bool {
	if len(dependsOn) == 0 {
		return true
	}
	for _, depID := range dependsOn {
		trimmed := strings.TrimSpace(depID)
		if trimmed == "" {
			continue
		}
		dep := lookup(trimmed)
		if dep == nil {
			return false
		}
		if dep.Status == core.IssueStatusDone {
			continue
		}
		switch dep.Status {
		case core.IssueStatusFailed, core.IssueStatusAbandoned:
			if failPolicy == core.FailSkip {
				continue
			}
			return false
		default:
			return false
		}
	}
	return true
}

func (s *DepScheduler) effectiveDependsOn(rs *runningSession, issue *core.Issue) []string {
	if issue == nil {
		return nil
	}
	if rs == nil || strings.TrimSpace(issue.ParentID) == "" {
		return issue.DependsOn
	}

	parent := rs.IssueByID[issue.ParentID]
	if parent == nil {
		storedParent, err := s.store.GetIssue(issue.ParentID)
		if err == nil {
			parent = storedParent
		}
	}
	if parent == nil || parent.ChildrenMode != core.ChildrenModeSequential {
		return issue.DependsOn
	}

	siblings := make([]*core.Issue, 0)
	for _, candidate := range rs.IssueByID {
		if candidate == nil || candidate.ParentID != issue.ParentID {
			continue
		}
		siblings = append(siblings, candidate)
	}
	if len(siblings) == 0 {
		storedSiblings, err := s.store.GetChildIssues(issue.ParentID)
		if err != nil {
			return issue.DependsOn
		}
		for i := range storedSiblings {
			sibling := storedSiblings[i]
			siblings = append(siblings, &sibling)
		}
	}
	if len(siblings) <= 1 {
		return issue.DependsOn
	}

	sort.SliceStable(siblings, func(i, j int) bool {
		if siblings[i].Priority != siblings[j].Priority {
			return siblings[i].Priority > siblings[j].Priority
		}
		if !siblings[i].CreatedAt.Equal(siblings[j].CreatedAt) {
			return siblings[i].CreatedAt.Before(siblings[j].CreatedAt)
		}
		return siblings[i].ID < siblings[j].ID
	})

	previousID := ""
	for i, sibling := range siblings {
		if sibling == nil || sibling.ID != issue.ID {
			continue
		}
		if i > 0 && siblings[i-1] != nil {
			previousID = strings.TrimSpace(siblings[i-1].ID)
		}
		break
	}
	if previousID == "" {
		return issue.DependsOn
	}
	for _, depID := range issue.DependsOn {
		if strings.TrimSpace(depID) == previousID {
			return issue.DependsOn
		}
	}

	effective := append([]string{}, issue.DependsOn...)
	effective = append(effective, previousID)
	return effective
}

func (s *DepScheduler) dependenciesSatisfiedLocked(rs *runningSession, issue *core.Issue) (bool, error) {
	if issue == nil {
		return false, nil
	}
	effectiveDependsOn := s.effectiveDependsOn(rs, issue)
	lookup := func(id string) *core.Issue {
		if dep := rs.IssueByID[id]; dep != nil {
			return dep
		}
		dep, err := s.store.GetIssue(id)
		if err != nil {
			return nil
		}
		return dep
	}
	for _, depID := range effectiveDependsOn {
		trimmed := strings.TrimSpace(depID)
		if trimmed == "" || rs.IssueByID[trimmed] != nil {
			continue
		}
		if _, err := s.store.GetIssue(trimmed); err != nil {
			return false, err
		}
	}
	return areDependenciesMet(effectiveDependsOn, issue.FailPolicy, lookup), nil
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

func (s *DepScheduler) registerRunCancel(runID string, cancel context.CancelFunc) {
	if s == nil || strings.TrimSpace(runID) == "" || cancel == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runCancels[runID] = cancel
}

func (s *DepScheduler) forgetRunCancel(runID string) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runCancels, runID)
}

func (s *DepScheduler) cancelRun(runID string) bool {
	if s == nil || strings.TrimSpace(runID) == "" {
		return false
	}
	s.mu.Lock()
	cancel := s.runCancels[runID]
	s.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}
