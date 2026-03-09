package teamleader

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *DepScheduler) handleRunEventLocked(evt core.Event) error {
	ref, trackedRunID, err := s.resolveRunRefLocked(evt)
	if err != nil {
		return err
	}
	if ref.sessionID == "" || ref.issueID == "" {
		return nil
	}

	rs := s.sessions[ref.sessionID]
	if rs == nil {
		if trackedRunID != "" {
			delete(s.RunIndex, trackedRunID)
		}
		return nil
	}

	issue := rs.IssueByID[ref.issueID]
	if issue == nil {
		if trackedRunID != "" {
			delete(s.RunIndex, trackedRunID)
		}
		delete(rs.Running, ref.issueID)
		s.releaseSlot()
		return nil
	}

	cleanupRunID := trackedRunID
	if cleanupRunID == "" {
		cleanupRunID = strings.TrimSpace(issue.RunID)
	}

	switch evt.Type {
	case core.EventRunDone:
		if issue.AutoMerge {
			if err := transitionIssueStatus(issue, core.IssueStatusMerging); err != nil {
				return err
			}
			if err := s.saveIssue(issue); err != nil {
				return err
			}
			s.recordTaskStep(issue, core.StepMergeStarted, "system", "run completed, entering merge")
			s.publishIssueEvent(core.EventIssueMerging, issue, nil, "")
			return nil
		}
		if err := transitionIssueStatus(issue, core.IssueStatusDone); err != nil {
			return err
		}
		if err := s.saveIssue(issue); err != nil {
			return err
		}
		s.recordTaskStep(issue, core.StepCompleted, "system", "run completed with auto_merge disabled")
		s.publishIssueEvent(core.EventIssueDone, issue, nil, "")
	case core.EventRunFailed:
		if err := transitionIssueStatus(issue, core.IssueStatusFailed); err != nil {
			return err
		}
		if err := s.saveIssue(issue); err != nil {
			return err
		}
		s.recordTaskStep(issue, core.StepFailed, "system", evt.Error)
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
	case core.EventIssueMerged:
		if err := transitionIssueStatus(issue, core.IssueStatusDone); err != nil {
			return err
		}
		if err := s.saveIssue(issue); err != nil {
			return err
		}
		s.recordTaskStep(issue, core.StepMergeCompleted, "system", "merge completed")
		s.publishIssueEvent(core.EventIssueDone, issue, nil, "")
	case core.EventMergeFailed:
		if err := transitionIssueStatus(issue, core.IssueStatusFailed); err != nil {
			return err
		}
		if err := s.saveIssue(issue); err != nil {
			return err
		}
		s.recordTaskStep(issue, core.StepFailed, "system", evt.Error)
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
	case core.EventIssueMergeRetry:
		if err := s.syncIssueStateFromStoreLocked(issue); err != nil {
			return err
		}
		if issue.Status != core.IssueStatusQueued {
			if err := transitionIssueStatus(issue, core.IssueStatusQueued); err != nil {
				return err
			}
			issue.RunID = ""
			if err := s.saveIssue(issue); err != nil {
				return err
			}
			s.recordTaskStep(issue, core.StepQueued, "system", "merge retry queued")
		}
	case core.EventIssueFailed:
		if err := s.syncIssueStateFromStoreLocked(issue); err != nil {
			return err
		}
		_, running := rs.Running[ref.issueID]
		if issue.Status != core.IssueStatusFailed {
			if err := transitionIssueStatus(issue, core.IssueStatusFailed); err != nil {
				return err
			}
			s.recordTaskStep(issue, core.StepFailed, "system", evt.Error)
			if err := s.saveIssue(issue); err != nil {
				return err
			}
		}
		if !running {
			if cleanupRunID != "" {
				delete(s.RunIndex, cleanupRunID)
			}
			return nil
		}
		switch issue.FailPolicy {
		case core.FailSkip:
		case core.FailHuman:
			rs.HaltNew = true
		default:
			if err := s.applyBlockPolicyLocked(rs, issue.ID); err != nil {
				return err
			}
		}
	case core.EventIssueMergeConflict:
		return nil
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
	if cleanupRunID != "" {
		delete(s.RunIndex, cleanupRunID)
	}
	return nil
}

func isSchedulerHandledEvent(eventType core.EventType) bool {
	switch eventType {
	case core.EventRunDone,
		core.EventRunFailed,
		core.EventIssueMerged,
		core.EventMergeFailed,
		core.EventIssueMergeRetry,
		core.EventIssueFailed,
		core.EventIssueMergeConflict:
		return true
	default:
		return false
	}
}

func (s *DepScheduler) resolveRunRefLocked(evt core.Event) (RunRef, string, error) {
	runID := strings.TrimSpace(evt.RunID)
	if runID != "" {
		if ref, ok := s.RunIndex[runID]; ok {
			return ref, runID, nil
		}
		issue, err := s.store.GetIssueByRun(runID)
		if err != nil || issue == nil {
			return RunRef{}, "", err
		}
		sessionID := makeSessionID(issue.ProjectID, issue.SessionID)
		rs := s.sessions[sessionID]
		if rs == nil {
			return RunRef{}, runID, nil
		}
		if _, exists := rs.IssueByID[issue.ID]; !exists {
			return RunRef{}, runID, nil
		}
		ref := RunRef{sessionID: sessionID, issueID: issue.ID}
		s.RunIndex[runID] = ref
		return ref, runID, nil
	}
	return s.resolveRunRefByIssueLocked(strings.TrimSpace(evt.IssueID))
}

func (s *DepScheduler) resolveRunRefByIssueLocked(issueID string) (RunRef, string, error) {
	if issueID == "" {
		return RunRef{}, "", nil
	}
	for sessionID, rs := range s.sessions {
		if rs == nil {
			continue
		}
		issue := rs.IssueByID[issueID]
		if issue == nil {
			continue
		}
		runID := strings.TrimSpace(issue.RunID)
		if runID != "" {
			s.RunIndex[runID] = RunRef{sessionID: sessionID, issueID: issueID}
		}
		return RunRef{sessionID: sessionID, issueID: issueID}, runID, nil
	}
	storedIssue, err := s.store.GetIssue(issueID)
	if err != nil || storedIssue == nil {
		return RunRef{}, "", err
	}
	sessionID := makeSessionID(storedIssue.ProjectID, storedIssue.SessionID)
	rs := s.sessions[sessionID]
	if rs == nil || rs.IssueByID[issueID] == nil {
		return RunRef{}, "", nil
	}
	runID := strings.TrimSpace(rs.IssueByID[issueID].RunID)
	if runID != "" {
		s.RunIndex[runID] = RunRef{sessionID: sessionID, issueID: issueID}
	}
	return RunRef{sessionID: sessionID, issueID: issueID}, runID, nil
}

func (s *DepScheduler) applyBlockPolicyLocked(rs *runningSession, failedIssueID string) error {
	rs.HaltNew = true
	for _, issueID := range sortedIssueIDs(rs.IssueByID) {
		if issueID == failedIssueID {
			continue
		}
		issue := rs.IssueByID[issueID]
		if issue == nil || isIssueTerminal(issue.Status) ||
			issue.Status == core.IssueStatusExecuting || issue.Status == core.IssueStatusMerging {
			continue
		}
		if err := transitionIssueStatus(issue, core.IssueStatusFailed); err != nil {
			return err
		}
		s.recordTaskStep(issue, core.StepFailed, "system", "blocked by session failure")
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

func (s *DepScheduler) syncIssueStateFromStoreLocked(issue *core.Issue) error {
	if s == nil || s.store == nil || issue == nil {
		return nil
	}
	storedIssue, err := s.store.GetIssue(issue.ID)
	if err != nil || storedIssue == nil {
		return err
	}
	issue.Status = storedIssue.Status
	issue.RunID = storedIssue.RunID
	issue.FailPolicy = storedIssue.FailPolicy
	issue.MergeRetries = storedIssue.MergeRetries
	return nil
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

func (s *DepScheduler) recordTaskStep(issue *core.Issue, action core.TaskStepAction, agentID, note string) {
	if s == nil || s.store == nil || issue == nil || strings.TrimSpace(issue.ID) == "" {
		return
	}
	if _, err := s.store.SaveTaskStep(&core.TaskStep{
		ID:        core.NewTaskStepID(),
		IssueID:   strings.TrimSpace(issue.ID),
		RunID:     strings.TrimSpace(issue.RunID),
		Action:    action,
		AgentID:   strings.TrimSpace(agentID),
		Note:      strings.TrimSpace(note),
		CreatedAt: time.Now(),
	}); err != nil {
		slog.Warn("failed to save task step", "error", err, "issue", issue.ID, "action", action)
	}
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
	s.pub.Publish(context.Background(), evt)
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
