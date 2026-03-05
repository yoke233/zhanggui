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
			if err := transitionIssueStatus(candidate, core.IssueStatusReady); err == nil {
				candidate.RunID = ""
				issue = candidate
			}
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
			if err := transitionIssueStatus(issue, core.IssueStatusReady); err != nil {
				return err
			}
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
