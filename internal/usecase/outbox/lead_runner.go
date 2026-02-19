package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"zhanggui/internal/bootstrap/logging"
	domainoutbox "zhanggui/internal/domain/outbox"
	"zhanggui/internal/errs"
	"zhanggui/internal/ports"
)

const (
	leadDefaultEventBatch = 200
)

type LeadSyncInput struct {
	Role           string
	Assignee       string
	WorkflowFile   string
	ConfigFile     string
	ExecutablePath string
	EventBatch     int
}

type LeadSyncResult struct {
	CursorBefore uint64
	CursorAfter  uint64
	Candidates   int
	Processed    int
	Blocked      int
	Spawned      int
	Skipped      int
}

type leadIssueOutcome struct {
	processed bool
	blocked   bool
	spawned   bool
}

func (s *Service) LeadSyncOnce(ctx context.Context, input LeadSyncInput) (LeadSyncResult, error) {
	if ctx == nil {
		return LeadSyncResult{}, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return LeadSyncResult{}, err
	}
	if s.repo == nil {
		return LeadSyncResult{}, errors.New("outbox repository is required")
	}
	if s.cache == nil {
		return LeadSyncResult{}, errors.New("cache is required")
	}

	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = "backend"
	}

	profile, err := loadWorkflowProfile(input.WorkflowFile)
	if err != nil {
		return LeadSyncResult{}, err
	}
	if !isRoleEnabled(profile, role) {
		return LeadSyncResult{}, fmt.Errorf("role %s is not enabled in workflow", role)
	}
	if strings.TrimSpace(profile.Outbox.Backend) != "sqlite" {
		return LeadSyncResult{}, fmt.Errorf("lead only supports sqlite backend, got %q", profile.Outbox.Backend)
	}

	group, ok := findGroupByRole(profile, role)
	if !ok {
		return LeadSyncResult{}, fmt.Errorf("group config is required for role %s", role)
	}
	listenLabels := normalizeLabels(group.ListenLabels)
	if len(listenLabels) == 0 {
		listenLabels = []string{"to:" + role}
	}
	groupMode := normalizeGroupMode(group.Mode)
	groupWriteback := normalizeGroupWriteback(group.Writeback)

	assignee := strings.TrimSpace(input.Assignee)
	if assignee == "" {
		assignee = "lead-" + role
	}

	eventBatch := input.EventBatch
	if eventBatch <= 0 {
		eventBatch = leadDefaultEventBatch
	}

	cursorKey := leadCursorKey(role)
	cursorBefore, err := s.getUintCache(ctx, cursorKey)
	if err != nil {
		return LeadSyncResult{}, err
	}

	result := LeadSyncResult{
		CursorBefore: cursorBefore,
		CursorAfter:  cursorBefore,
	}

	candidateIDs := make(map[uint64]struct{})
	events, err := s.repo.ListEventsAfter(ctx, cursorBefore, eventBatch)
	if err != nil {
		return LeadSyncResult{}, err
	}
	for _, event := range events {
		candidateIDs[event.IssueID] = struct{}{}
		if event.EventID > result.CursorAfter {
			result.CursorAfter = event.EventID
		}
	}

	issueQueue, err := s.listRoleQueueIssues(ctx, listenLabels)
	if err != nil {
		return LeadSyncResult{}, err
	}
	for _, issue := range issueQueue {
		candidateIDs[issue.IssueID] = struct{}{}
	}

	claimedQueue, err := s.repo.ListIssues(ctx, ports.OutboxIssueFilter{
		IncludeClosed: false,
		Assignee:      assignee,
		ExcludeLabels: []string{"autoflow:off"},
	})
	if err != nil {
		return LeadSyncResult{}, err
	}
	for _, issue := range claimedQueue {
		candidateIDs[issue.IssueID] = struct{}{}
	}

	issueIDs := make([]uint64, 0, len(candidateIDs))
	for issueID := range candidateIDs {
		issueIDs = append(issueIDs, issueID)
	}
	sort.Slice(issueIDs, func(i int, j int) bool { return issueIDs[i] < issueIDs[j] })
	result.Candidates = len(issueIDs)
	maxSpawnPerTick := maxConcurrentForTick(group.MaxConcurrent)

	for _, issueID := range issueIDs {
		issue, getErr := s.repo.GetIssue(ctx, issueID)
		if getErr != nil {
			return LeadSyncResult{}, getErr
		}
		allowSpawn := result.Spawned < maxSpawnPerTick

		outcome, processErr := s.processLeadIssue(ctx, leadIssueProcessInput{
			Role:           role,
			Assignee:       assignee,
			Issue:          issue,
			WorkflowFile:   input.WorkflowFile,
			ConfigFile:     input.ConfigFile,
			ExecutablePath: input.ExecutablePath,
			Profile:        profile,
			CursorAfter:    result.CursorAfter,
			AllowSpawn:     allowSpawn,
			GroupMode:      groupMode,
			WritebackMode:  groupWriteback,
			ListenLabels:   listenLabels,
		})
		if processErr != nil {
			return LeadSyncResult{}, processErr
		}
		if outcome.processed {
			result.Processed++
		} else {
			result.Skipped++
		}
		if outcome.blocked {
			result.Blocked++
		}
		if outcome.spawned {
			result.Spawned++
		}
	}

	if result.CursorAfter > cursorBefore {
		if err := s.cache.Set(ctx, cursorKey, strconv.FormatUint(result.CursorAfter, 10), 0); err != nil {
			return LeadSyncResult{}, err
		}
	}
	return result, nil
}

func (s *Service) listRoleQueueIssues(ctx context.Context, listenLabels []string) ([]ports.OutboxIssue, error) {
	listenLabels = normalizeLabels(listenLabels)
	if len(listenLabels) == 0 {
		return nil, nil
	}

	return s.repo.ListIssues(ctx, ports.OutboxIssueFilter{
		IncludeClosed: false,
		IncludeLabels: listenLabels,
		ExcludeLabels: []string{"autoflow:off"},
	})
}

type leadIssueProcessInput struct {
	Role            string
	Assignee        string
	Issue           ports.OutboxIssue
	WorkflowFile    string
	ConfigFile      string
	ExecutablePath  string
	Profile         workflowProfile
	CursorAfter     uint64
	AllowSpawn      bool
	IgnoreStateSkip bool
	GroupMode       string
	WritebackMode   string
	ListenLabels    []string
}

func (s *Service) processLeadIssue(ctx context.Context, input leadIssueProcessInput) (leadIssueOutcome, error) {
	if input.Issue.IsClosed {
		return leadIssueOutcome{}, nil
	}

	issueRef := formatIssueRef(input.Issue.IssueID)
	issueAssignee := strings.TrimSpace(derefString(input.Issue.Assignee))
	if isSubscriberMode(input.GroupMode) {
		if issueAssignee == "" {
			return leadIssueOutcome{}, nil
		}
	} else if issueAssignee != input.Assignee {
		return leadIssueOutcome{}, nil
	}

	currentUpdatedAt := strings.TrimSpace(input.Issue.UpdatedAt)
	seenUpdatedAt, seen, err := s.cache.Get(ctx, leadIssueSeenUpdatedAtKey(input.Role, issueRef))
	if err != nil {
		return leadIssueOutcome{}, err
	}
	if seen && strings.TrimSpace(seenUpdatedAt) != "" && strings.TrimSpace(seenUpdatedAt) == currentUpdatedAt {
		return leadIssueOutcome{}, nil
	}

	labels, err := s.repo.ListIssueLabels(ctx, input.Issue.IssueID)
	if err != nil {
		return leadIssueOutcome{}, err
	}
	if containsString(labels, "autoflow:off") {
		return leadIssueOutcome{}, nil
	}
	if !input.IgnoreStateSkip && shouldSkipLeadSpawnByState(labels, input.ListenLabels) {
		return leadIssueOutcome{}, nil
	}
	if isSubscriberMode(input.GroupMode) && !containsAllLabels(labels, input.ListenLabels) {
		return leadIssueOutcome{}, nil
	}

	commentOnly := isCommentOnlyWriteback(input.WritebackMode)
	writeComment := func(state string, body string) error {
		effectiveState := state
		if commentOnly {
			effectiveState = ""
		}
		return s.CommentIssue(ctx, CommentIssueInput{
			IssueRef: issueRef,
			Actor:    input.Assignee,
			State:    effectiveState,
			Body:     body,
		})
	}
	addNeedsHumanBestEffort := func() {
		if commentOnly {
			return
		}
		if containsString(labels, "needs-human") {
			return
		}
		if addErr := s.AddIssueLabels(ctx, AddIssueLabelsInput{
			IssueRef: issueRef,
			Actor:    input.Assignee,
			Labels:   []string{"needs-human"},
		}); addErr != nil {
			logging.Error(logging.WithAttrs(ctx, slog.String("component", "outbox.lead")), "add needs-human label failed", slog.Any("err", errs.Loggable(addErr)), slog.String("issue_ref", issueRef))
		}
	}
	markSeen := func() error {
		refreshed, err := s.repo.GetIssue(ctx, input.Issue.IssueID)
		if err != nil {
			return err
		}
		updatedAt := strings.TrimSpace(refreshed.UpdatedAt)
		if updatedAt == "" {
			updatedAt = currentUpdatedAt
		}
		return s.cache.Set(ctx, leadIssueSeenUpdatedAtKey(input.Role, issueRef), updatedAt, 0)
	}

	indexedVerdictsChanged := false
	if shouldIndexVerdictLabels(input.Role, input.GroupMode, input.WritebackMode) {
		changed, err := s.indexVerdictLabels(ctx, input.Issue.IssueID, input.Assignee, labels)
		if err != nil {
			return leadIssueOutcome{}, err
		}
		indexedVerdictsChanged = changed
	}

	if containsString(labels, "needs-human") {
		summary := applyVerdictMarker(input.Role, "blocked", "manual_intervention", "issue has needs-human label")
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        "none",
			Action:       "blocked",
			Status:       "blocked",
			ResultCode:   "manual_intervention",
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "manual:needs-human",
			Summary:      summary,
			BlockedBy:    []string{"needs-human"},
			Next:         fmt.Sprintf("@%s remove needs-human after manual review", input.Role),
			Tests:        WorkResultTests{},
			Changes:      WorkResultChanges{},
			OpenQuestion: "none",
		})
		if err := writeComment("blocked", body); err != nil {
			return leadIssueOutcome{}, err
		}
		if err := markSeen(); err != nil {
			return leadIssueOutcome{}, err
		}
		return leadIssueOutcome{processed: true, blocked: true}, nil
	}

	dependencies := parseDependsOnRefs(input.Issue.Body)
	unresolved, err := unresolvedDependenciesTx(ctx, s.repo, dependencies)
	if err != nil {
		return leadIssueOutcome{}, err
	}
	if len(unresolved) > 0 {
		summary := applyVerdictMarker(input.Role, "blocked", "dep_unresolved", "issue has unresolved dependencies")
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        "none",
			Action:       "blocked",
			Status:       "blocked",
			ResultCode:   "dep_unresolved",
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "manual:depends-on",
			Summary:      summary,
			BlockedBy:    unresolved,
			Next:         fmt.Sprintf("@%s wait until dependencies are closed", input.Role),
			Tests:        WorkResultTests{},
			Changes:      WorkResultChanges{},
			OpenQuestion: "none",
		})
		if err := writeComment("blocked", body); err != nil {
			return leadIssueOutcome{}, err
		}
		if err := markSeen(); err != nil {
			return leadIssueOutcome{}, err
		}
		return leadIssueOutcome{processed: true, blocked: true}, nil
	}
	if !input.AllowSpawn {
		if indexedVerdictsChanged {
			if err := markSeen(); err != nil {
				return leadIssueOutcome{}, err
			}
			return leadIssueOutcome{processed: true}, nil
		}
		return leadIssueOutcome{}, nil
	}

	repoDir, err := resolveRoleRepoDir(input.Profile, input.WorkflowFile, input.Role)
	if err != nil {
		return leadIssueOutcome{}, err
	}

	runID, err := s.nextRunID(ctx, issueRef, input.Role)
	if err != nil {
		return leadIssueOutcome{}, err
	}
	if err := s.cache.Set(ctx, leadActiveRunKey(input.Role, issueRef), runID, 0); err != nil {
		return leadIssueOutcome{}, err
	}

	effectiveRepoDir := repoDir
	var workdirMgr workdirManager
	workdirCfg := input.Profile.Workdir
	workdirPath := ""
	workdirDisplay := ""
	if shouldUseWorkdir(workdirCfg, input.Role) {
		factory := s.workdirFactory
		if factory == nil {
			factory = func(cfg workflowWorkdirConfig, workflowFile string, repoDir string) (workdirManager, error) {
				return newGitWorktreeManager(cfg, workflowFile, repoDir)
			}
		}

		workdirMgr, err = factory(workdirCfg, input.WorkflowFile, repoDir)
		if err != nil {
			return leadIssueOutcome{}, err
		}

		workdirPath, err = workdirMgr.Prepare(ctx, input.Role, issueRef, runID)
		if err != nil {
			summary := applyVerdictMarker(input.Role, "blocked", "env_unavailable", "workdir prepare failed: "+err.Error())
			body := buildStructuredComment(StructuredCommentInput{
				Role:         input.Role,
				IssueRef:     issueRef,
				RunID:        runID,
				Action:       "blocked",
				Status:       "blocked",
				ResultCode:   "env_unavailable",
				ReadUpTo:     formatReadUpTo(input.CursorAfter),
				Trigger:      "workdir:prepare:" + runID,
				Summary:      summary,
				BlockedBy:    []string{"workdir-prepare"},
				Next:         fmt.Sprintf("@%s fix git worktree setup and retry with a new run", input.Role),
				OpenQuestion: "none",
			})
			if commentErr := writeComment("blocked", body); commentErr != nil {
				return leadIssueOutcome{}, commentErr
			}
			if err := markSeen(); err != nil {
				return leadIssueOutcome{}, err
			}
			return leadIssueOutcome{processed: true, blocked: true}, nil
		}

		effectiveRepoDir = workdirPath
		workdirDisplay = workdirPath
		if rel, relErr := filepath.Rel(filepath.Dir(input.WorkflowFile), workdirPath); relErr == nil {
			if cleaned := filepath.Clean(rel); cleaned != "." && cleaned != ".." && !strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
				workdirDisplay = cleaned
			}
		}
	}

	cleanupWorkdir := func() error {
		if workdirMgr == nil || strings.TrimSpace(workdirPath) == "" {
			return nil
		}
		return workdirMgr.Cleanup(ctx, input.Role, issueRef, runID, workdirPath)
	}

	contextPackDir := leadContextPackDir(issueRef, runID)
	workOrder := domainoutbox.WorkOrder{
		IssueRef: issueRef,
		RunID:    runID,
		Role:     input.Role,
		RepoDir:  effectiveRepoDir,
	}
	if err := prepareContextPack(contextPackDir, workOrder, input.Issue.Body, input.CursorAfter); err != nil {
		if cleanupErr := cleanupWorkdir(); cleanupErr != nil {
			return leadIssueOutcome{}, errs.Wrap(cleanupErr, "cleanup workdir after context pack failure")
		}
		return leadIssueOutcome{}, err
	}

	workerInvoker := s.workerInvoker
	if workerInvoker == nil {
		workerInvoker = s.invokeWorker
	}
	if err := workerInvoker(ctx, invokeWorkerInput{
		ExecutablePath: input.ExecutablePath,
		ConfigFile:     input.ConfigFile,
		WorkflowFile:   input.WorkflowFile,
		ContextPackDir: contextPackDir,
		IssueRef:       issueRef,
		RunID:          runID,
		Role:           input.Role,
	}); err != nil {
		cleanupErr := cleanupWorkdir()
		blockedBy := []string{"worker-execution"}
		summary := "worker execution failed: " + err.Error()
		next := fmt.Sprintf("@%s retry with a new run", input.Role)
		resultCode := "env_unavailable"
		if cleanupErr != nil {
			blockedBy = append(blockedBy, "workdir-cleanup")
			summary = summary + "; workdir cleanup failed: " + cleanupErr.Error()
			next = fmt.Sprintf("@%s cleanup %s and retry with a new run", input.Role, workdirDisplay)
			resultCode = "manual_intervention"
		}

		summary = applyVerdictMarker(input.Role, "blocked", resultCode, summary)
		// Worker execution errors are normalized into blocked events.
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        runID,
			Action:       "blocked",
			Status:       "blocked",
			ResultCode:   resultCode,
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "workrun:" + runID,
			Summary:      summary,
			BlockedBy:    blockedBy,
			Next:         next,
			OpenQuestion: "none",
		})
		if commentErr := writeComment("blocked", body); commentErr != nil {
			return leadIssueOutcome{}, commentErr
		}
		if shouldAddNeedsHumanForResultCode(resultCode) {
			addNeedsHumanBestEffort()
		}
		if err := markSeen(); err != nil {
			return leadIssueOutcome{}, err
		}
		return leadIssueOutcome{processed: true, blocked: true, spawned: true}, nil
	}

	workResultLoader := s.workResultLoader
	if workResultLoader == nil {
		workResultLoader = loadWorkResultFromContextPack
	}
	result, err := workResultLoader(contextPackDir)
	if err != nil {
		cleanupErr := cleanupWorkdir()
		blockedBy := []string{"worker-result"}
		summary := "worker result is missing or invalid: " + err.Error()
		next := fmt.Sprintf("@%s provide parseable work result", input.Role)
		resultCode := "output_unparseable"
		if cleanupErr != nil {
			blockedBy = append(blockedBy, "workdir-cleanup")
			summary = summary + "; workdir cleanup failed: " + cleanupErr.Error()
			next = fmt.Sprintf("@%s cleanup %s and retry with a new run", input.Role, workdirDisplay)
			resultCode = "manual_intervention"
		}

		summary = applyVerdictMarker(input.Role, "blocked", resultCode, summary)
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        runID,
			Action:       "blocked",
			Status:       "blocked",
			ResultCode:   resultCode,
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "workrun:" + runID,
			Summary:      summary,
			BlockedBy:    blockedBy,
			Next:         next,
			OpenQuestion: "none",
		})
		if commentErr := writeComment("blocked", body); commentErr != nil {
			return leadIssueOutcome{}, commentErr
		}
		addNeedsHumanBestEffort()
		if err := markSeen(); err != nil {
			return leadIssueOutcome{}, err
		}
		return leadIssueOutcome{processed: true, blocked: true, spawned: true}, nil
	}

	activeRunID, _, _ := s.cache.Get(ctx, leadActiveRunKey(input.Role, issueRef))
	if domainoutbox.IsStaleRun(activeRunID, result.RunID) {
		if cleanupErr := cleanupWorkdir(); cleanupErr != nil {
			logging.Error(logging.WithAttrs(ctx, slog.String("component", "outbox.lead")), "cleanup stale workdir failed", slog.Any("err", errs.Loggable(cleanupErr)), slog.String("issue_ref", issueRef), slog.String("run_id", runID), slog.String("workdir", workdirDisplay))
		}
		return leadIssueOutcome{processed: true, spawned: true}, nil
	}

	if err := domainoutbox.ValidateWorkResultEcho(workOrder, result.toDomain()); err != nil {
		cleanupErr := cleanupWorkdir()
		blockedBy := []string{"work-result-echo"}
		summary := "work result echo validation failed: " + err.Error()
		next := fmt.Sprintf("@%s fix work result issue_ref/run_id", input.Role)
		resultCode := "output_unparseable"
		if cleanupErr != nil {
			blockedBy = append(blockedBy, "workdir-cleanup")
			summary = summary + "; workdir cleanup failed: " + cleanupErr.Error()
			next = fmt.Sprintf("@%s cleanup %s and retry with a new run", input.Role, workdirDisplay)
			resultCode = "manual_intervention"
		}

		summary = applyVerdictMarker(input.Role, "blocked", resultCode, summary)
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        runID,
			Action:       "blocked",
			Status:       "blocked",
			ResultCode:   resultCode,
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "workrun:" + runID,
			Summary:      summary,
			BlockedBy:    blockedBy,
			Next:         next,
			OpenQuestion: "none",
		})
		if commentErr := writeComment("blocked", body); commentErr != nil {
			return leadIssueOutcome{}, commentErr
		}
		addNeedsHumanBestEffort()
		if err := markSeen(); err != nil {
			return leadIssueOutcome{}, err
		}
		return leadIssueOutcome{processed: true, blocked: true, spawned: true}, nil
	}

	status := "review"
	action := "update"
	summary := "worker completed with evidence"
	next := "@integrator review and merge"
	blockedBy := []string{"none"}
	resultCode := "none"
	needsHuman := false

	if input.Role == "reviewer" {
		summary = "review completed with evidence"
		next = "@integrator finalize review decision"
	}
	if input.Role == "qa" {
		summary = "qa completed with evidence"
		next = "@integrator finalize qa decision"
	}

	if err := domainoutbox.ValidateWorkResultEvidence(result.toDomain()); err != nil {
		status = "blocked"
		action = "blocked"
		summary = "work result is missing required evidence: " + err.Error()
		next = fmt.Sprintf("@%s provide PR/Commit and Tests evidence", input.Role)
		blockedBy = []string{"missing-evidence"}
		resultCode = "manual_intervention"
		needsHuman = true
	}

	if input.Role == "reviewer" && strings.TrimSpace(result.ResultCode) == "review_changes_requested" {
		targetRole := nextRoleForFixCycle(labels)
		status = "blocked"
		action = "blocked"
		summary = "review requested changes"
		next = fmt.Sprintf("@%s address review changes and rerun", targetRole)
		blockedBy = []string{"review-changes-requested"}
		resultCode = "review_changes_requested"
	}

	if !needsHuman && strings.TrimSpace(result.ResultCode) != "" && !(input.Role == "reviewer" && strings.TrimSpace(result.ResultCode) == "review_changes_requested") {
		status = "blocked"
		action = "blocked"
		summary = "worker reported result_code: " + result.ResultCode
		next = fmt.Sprintf("@%s investigate failure and rerun", input.Role)
		blockedBy = []string{"worker-result-code"}
		resultCode = strings.TrimSpace(result.ResultCode)
		if shouldAddNeedsHumanForResultCode(resultCode) {
			needsHuman = true
		}
	}

	if strings.TrimSpace(result.Source) != "" {
		summary = summary + "\n- WorkResultSource: " + strings.TrimSpace(result.Source)
	}
	if _, statErr := os.Stat(filepath.Join(contextPackDir, "work_audit.json")); statErr == nil {
		summary = summary + "\n- Audit: " + filepath.ToSlash(filepath.Join(contextPackDir, "work_audit.json"))
	}

	if cleanupErr := cleanupWorkdir(); cleanupErr != nil {
		logging.Error(logging.WithAttrs(ctx, slog.String("component", "outbox.lead")), "cleanup workdir failed", slog.Any("err", errs.Loggable(cleanupErr)), slog.String("issue_ref", issueRef), slog.String("run_id", runID), slog.String("workdir", workdirDisplay))

		cleanupSummary := "workdir cleanup failed"
		if cleanupErr.Error() != "" {
			cleanupSummary = cleanupSummary + ": " + cleanupErr.Error()
		}

		status = "blocked"
		action = "blocked"
		resultCode = "manual_intervention"
		needsHuman = true
		if strings.TrimSpace(summary) != "" && summary != cleanupSummary {
			summary = summary + "; " + cleanupSummary
		} else {
			summary = cleanupSummary
		}
		next = fmt.Sprintf("@%s cleanup %s and retry with a new run", input.Role, workdirDisplay)
		cleanupBlockedBy := make([]string, 0, len(blockedBy)+1)
		for _, item := range blockedBy {
			normalized := strings.TrimSpace(item)
			if normalized == "" || normalized == "none" {
				continue
			}
			cleanupBlockedBy = append(cleanupBlockedBy, normalized)
		}
		cleanupBlockedBy = append(cleanupBlockedBy, "workdir-cleanup")
		blockedBy = normalizeLabels(cleanupBlockedBy)
	}

	summary = applyVerdictMarker(input.Role, status, resultCode, summary)
	body := buildStructuredComment(StructuredCommentInput{
		Role:         input.Role,
		IssueRef:     issueRef,
		RunID:        runID,
		Action:       action,
		Status:       status,
		ResultCode:   resultCode,
		ReadUpTo:     formatReadUpTo(input.CursorAfter),
		Trigger:      "workrun:" + runID,
		Summary:      summary,
		Changes:      result.Changes,
		Tests:        result.Tests,
		BlockedBy:    blockedBy,
		OpenQuestion: "none",
		Next:         next,
	})
	if err := writeComment(status, body); err != nil {
		return leadIssueOutcome{}, err
	}
	if needsHuman {
		addNeedsHumanBestEffort()
	}
	if err := markSeen(); err != nil {
		return leadIssueOutcome{}, err
	}

	return leadIssueOutcome{
		processed: true,
		blocked:   status == "blocked",
		spawned:   true,
	}, nil
}

func (s *Service) indexVerdictLabels(ctx context.Context, issueID uint64, actor string, currentLabels []string) (bool, error) {
	events, err := s.repo.ListIssueEvents(ctx, issueID)
	if err != nil {
		return false, err
	}

	latestReview := ""
	latestQA := ""
	for _, event := range events {
		marker := extractVerdictMarker(event.Body)
		switch marker {
		case "review:approved", "review:changes_requested":
			latestReview = marker
		case "qa:pass", "qa:fail":
			latestQA = marker
		}
	}

	changed := false
	issueRef := formatIssueRef(issueID)

	reviewLabels := []string{"review:approved", "review:changes_requested"}
	qaLabels := []string{"qa:pass", "qa:fail"}

	if latestReview != "" {
		toRemove := make([]string, 0)
		for _, label := range reviewLabels {
			if label == latestReview {
				continue
			}
			if containsString(currentLabels, label) {
				toRemove = append(toRemove, label)
			}
		}
		toAdd := make([]string, 0, 1)
		if !containsString(currentLabels, latestReview) {
			toAdd = append(toAdd, latestReview)
		}

		if len(toRemove) > 0 {
			if err := s.RemoveIssueLabels(ctx, RemoveIssueLabelsInput{
				IssueRef: issueRef,
				Actor:    actor,
				Labels:   toRemove,
			}); err != nil {
				return false, err
			}
			changed = true
		}
		if len(toAdd) > 0 {
			if err := s.AddIssueLabels(ctx, AddIssueLabelsInput{
				IssueRef: issueRef,
				Actor:    actor,
				Labels:   toAdd,
			}); err != nil {
				return false, err
			}
			changed = true
		}
	}

	if latestQA != "" {
		toRemove := make([]string, 0)
		for _, label := range qaLabels {
			if label == latestQA {
				continue
			}
			if containsString(currentLabels, label) {
				toRemove = append(toRemove, label)
			}
		}
		toAdd := make([]string, 0, 1)
		if !containsString(currentLabels, latestQA) {
			toAdd = append(toAdd, latestQA)
		}

		if len(toRemove) > 0 {
			if err := s.RemoveIssueLabels(ctx, RemoveIssueLabelsInput{
				IssueRef: issueRef,
				Actor:    actor,
				Labels:   toRemove,
			}); err != nil {
				return false, err
			}
			changed = true
		}
		if len(toAdd) > 0 {
			if err := s.AddIssueLabels(ctx, AddIssueLabelsInput{
				IssueRef: issueRef,
				Actor:    actor,
				Labels:   toAdd,
			}); err != nil {
				return false, err
			}
			changed = true
		}
	}

	return changed, nil
}

func leadCursorKey(role string) string {
	return "lead:" + role + ":cursor:event_id"
}

func leadActiveRunKey(role string, issueRef string) string {
	return "lead:" + role + ":active_run:" + issueRef
}

func leadRunSeqKey(role string, issueRef string) string {
	return "lead:" + role + ":run_seq:" + issueRef
}

func leadIssueSeenUpdatedAtKey(role string, issueRef string) string {
	return "lead:" + role + ":seen_issue_updated_at:" + issueRef
}

func (s *Service) getUintCache(ctx context.Context, key string) (uint64, error) {
	value, found, err := s.cache.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if !found || strings.TrimSpace(value) == "" {
		return 0, nil
	}
	parsed, parseErr := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if parseErr != nil {
		return 0, parseErr
	}
	return parsed, nil
}

func (s *Service) nextRunID(ctx context.Context, issueRef string, role string) (string, error) {
	seqKey := leadRunSeqKey(role, issueRef)
	seqValue, found, err := s.cache.Get(ctx, seqKey)
	if err != nil {
		return "", err
	}

	seq := 0
	if found && strings.TrimSpace(seqValue) != "" {
		seq, err = strconv.Atoi(strings.TrimSpace(seqValue))
		if err != nil {
			return "", err
		}
	}
	seq++

	runID := fmt.Sprintf("%s-%s-%04d", nowUTCDate(), role, seq)
	if err := s.cache.Set(ctx, seqKey, strconv.Itoa(seq), 0); err != nil {
		return "", err
	}
	return runID, nil
}

func nowUTCDate() string {
	return strings.Split(nowUTCString(), "T")[0]
}

func leadContextPackDir(issueRef string, runID string) string {
	sanitized := strings.NewReplacer("/", "_", "#", "_", ":", "_", "\\", "_").Replace(issueRef)
	return filepath.Join("state", "context_packs", sanitized, runID)
}

func prepareContextPack(contextPackDir string, order domainoutbox.WorkOrder, specSnapshot string, readUpTo uint64) error {
	if err := os.MkdirAll(contextPackDir, 0o755); err != nil {
		return err
	}

	rawOrder, err := json.MarshalIndent(order, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(contextPackDir, "work_order.json"), rawOrder, 0o644); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(contextPackDir, "spec_snapshot.md"), []byte(specSnapshot), 0o644); err != nil {
		return err
	}

	constraints := "Hard:\n- Keep IssueRef and RunId unchanged\n- Provide Changes and Tests evidence\n"
	if err := os.WriteFile(filepath.Join(contextPackDir, "constraints.md"), []byte(constraints), 0o644); err != nil {
		return err
	}

	links := fmt.Sprintf("IssueRef: %s\nReadUpTo: %s\n", order.IssueRef, formatReadUpTo(readUpTo))
	if err := os.WriteFile(filepath.Join(contextPackDir, "links.md"), []byte(links), 0o644); err != nil {
		return err
	}

	return nil
}

type invokeWorkerInput struct {
	ExecutablePath string
	ConfigFile     string
	WorkflowFile   string
	ContextPackDir string
	IssueRef       string
	RunID          string
	Role           string
}

func ensureLeadInvokeLogs(ctx context.Context, contextPackDir string) (*os.File, *os.File, error) {
	stdoutFile, err := os.Create(filepath.Join(contextPackDir, "worker_stdout.log"))
	if err != nil {
		return nil, nil, err
	}

	stderrFile, err := os.Create(filepath.Join(contextPackDir, "worker_stderr.log"))
	if err != nil {
		stdoutFile.Close()
		return nil, nil, err
	}

	return stdoutFile, stderrFile, nil
}

func (s *Service) invokeWorker(ctx context.Context, input invokeWorkerInput) error {
	executablePath := strings.TrimSpace(input.ExecutablePath)
	if executablePath == "" {
		var err error
		executablePath, err = os.Executable()
		if err != nil {
			return err
		}
	}

	args := make([]string, 0, 8)
	if strings.TrimSpace(input.ConfigFile) != "" {
		args = append(args, "--config", input.ConfigFile)
	}
	args = append(args, "worker", "run", "--context-pack", input.ContextPackDir, "--workflow", input.WorkflowFile)
	cmd := exec.CommandContext(ctx, executablePath, args...)
	cmd.Env = append(os.Environ(),
		"ZG_CONTEXT_PACK="+input.ContextPackDir,
		"ZG_ISSUE_REF="+input.IssueRef,
		"ZG_RUN_ID="+input.RunID,
		"ZG_ROLE="+input.Role,
	)

	stdoutFile, stderrFile, err := ensureLeadInvokeLogs(ctx, input.ContextPackDir)
	if err != nil {
		return err
	}
	defer stdoutFile.Close()
	defer stderrFile.Close()

	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	return cmd.Run()
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func containsAllLabels(labels []string, required []string) bool {
	for _, label := range required {
		trimmed := strings.TrimSpace(label)
		if trimmed == "" {
			continue
		}
		if !containsString(labels, trimmed) {
			return false
		}
	}
	return true
}

func extractVerdictMarker(body string) string {
	first := extractFirstSummaryBullet(body)
	lower := strings.ToLower(strings.TrimSpace(first))
	for _, marker := range []string{"review:approved", "review:changes_requested", "qa:pass", "qa:fail"} {
		if strings.HasPrefix(lower, marker) {
			return marker
		}
	}
	return ""
}

func extractFirstSummaryBullet(body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}

	lines := strings.Split(body, "\n")
	inSummary := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if !inSummary {
			if line == "Summary:" {
				inSummary = true
			}
			continue
		}

		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "- "))
		}
		if strings.HasSuffix(line, ":") {
			return ""
		}
	}
	return ""
}

func shouldSkipLeadSpawnByState(labels []string, listenLabels []string) bool {
	state := currentStateLabel(labels)
	switch state {
	case "state:blocked", "state:done":
		return true
	case "state:review":
		return !containsString(listenLabels, "state:review")
	default:
		return false
	}
}

func nextRoleForFixCycle(labels []string) string {
	if containsString(labels, "to:backend") {
		return "backend"
	}
	if containsString(labels, "to:frontend") {
		return "frontend"
	}
	return "backend"
}

func nextRoleForReviewChanges(labels []string) string {
	if containsString(labels, "to:backend") {
		return "backend"
	}
	if containsString(labels, "to:frontend") {
		return "frontend"
	}
	if containsString(labels, "to:qa") {
		return "qa"
	}
	return "backend"
}

func currentStateLabel(labels []string) string {
	for _, label := range labels {
		normalized := strings.TrimSpace(label)
		if strings.HasPrefix(normalized, "state:") {
			return normalized
		}
	}
	return ""
}

func maxConcurrentForTick(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func formatReadUpTo(eventID uint64) string {
	if eventID == 0 {
		return "none"
	}
	return "e" + strconv.FormatUint(eventID, 10)
}
