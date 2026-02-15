package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	domainoutbox "zhanggui/internal/domain/outbox"
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

	issueQueue, err := s.listRoleQueueIssues(ctx, role, listenLabels)
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

func (s *Service) listRoleQueueIssues(ctx context.Context, role string, listenLabels []string) ([]ports.OutboxIssue, error) {
	normalizedRole := strings.TrimSpace(role)
	if normalizedRole == "reviewer" && len(listenLabels) > 1 {
		items := make([]ports.OutboxIssue, 0)
		seen := make(map[uint64]struct{})
		for _, label := range listenLabels {
			normalizedLabel := strings.TrimSpace(label)
			if normalizedLabel == "" {
				continue
			}
			rows, err := s.repo.ListIssues(ctx, ports.OutboxIssueFilter{
				IncludeClosed: false,
				IncludeLabels: []string{normalizedLabel},
				ExcludeLabels: []string{"autoflow:off"},
			})
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				if _, ok := seen[row.IssueID]; ok {
					continue
				}
				seen[row.IssueID] = struct{}{}
				items = append(items, row)
			}
		}
		return items, nil
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
}

func (s *Service) processLeadIssue(ctx context.Context, input leadIssueProcessInput) (leadIssueOutcome, error) {
	if input.Issue.IsClosed {
		return leadIssueOutcome{}, nil
	}

	issueRef := formatIssueRef(input.Issue.IssueID)
	if strings.TrimSpace(derefString(input.Issue.Assignee)) != input.Assignee {
		return leadIssueOutcome{}, nil
	}

	labels, err := s.repo.ListIssueLabels(ctx, input.Issue.IssueID)
	if err != nil {
		return leadIssueOutcome{}, err
	}
	if containsString(labels, "autoflow:off") {
		return leadIssueOutcome{}, nil
	}
	if !input.IgnoreStateSkip && shouldSkipLeadSpawnByState(input.Role, labels) {
		return leadIssueOutcome{}, nil
	}

	if containsString(labels, "needs-human") {
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        "none",
			Action:       "blocked",
			Status:       "blocked",
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "manual:needs-human",
			Summary:      "issue has needs-human label",
			BlockedBy:    []string{"needs-human"},
			Next:         fmt.Sprintf("@%s remove needs-human after manual review", input.Role),
			Tests:        WorkResultTests{},
			Changes:      WorkResultChanges{},
			OpenQuestion: "none",
		})
		if err := s.CommentIssue(ctx, CommentIssueInput{
			IssueRef: issueRef,
			Actor:    input.Assignee,
			State:    "blocked",
			Body:     body,
		}); err != nil {
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
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        "none",
			Action:       "blocked",
			Status:       "blocked",
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "manual:depends-on",
			Summary:      "issue has unresolved dependencies",
			BlockedBy:    unresolved,
			Next:         fmt.Sprintf("@%s wait until dependencies are closed", input.Role),
			Tests:        WorkResultTests{},
			Changes:      WorkResultChanges{},
			OpenQuestion: "none",
		})
		if err := s.CommentIssue(ctx, CommentIssueInput{
			IssueRef: issueRef,
			Actor:    input.Assignee,
			State:    "blocked",
			Body:     body,
		}); err != nil {
			return leadIssueOutcome{}, err
		}
		return leadIssueOutcome{processed: true, blocked: true}, nil
	}
	if !input.AllowSpawn {
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

	contextPackDir := leadContextPackDir(issueRef, runID)
	workOrder := domainoutbox.WorkOrder{
		IssueRef: issueRef,
		RunID:    runID,
		Role:     input.Role,
		RepoDir:  repoDir,
	}
	if err := prepareContextPack(contextPackDir, workOrder, input.Issue.Body, input.CursorAfter); err != nil {
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
		// Worker execution errors are normalized into blocked events.
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        runID,
			Action:       "blocked",
			Status:       "blocked",
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "workrun:" + runID,
			Summary:      "worker execution failed: " + err.Error(),
			BlockedBy:    []string{"worker-execution"},
			Next:         fmt.Sprintf("@%s retry with a new run", input.Role),
			OpenQuestion: "none",
		})
		if commentErr := s.CommentIssue(ctx, CommentIssueInput{
			IssueRef: issueRef,
			Actor:    input.Assignee,
			State:    "blocked",
			Body:     body,
		}); commentErr != nil {
			return leadIssueOutcome{}, commentErr
		}
		return leadIssueOutcome{processed: true, blocked: true, spawned: true}, nil
	}

	workResultLoader := s.workResultLoader
	if workResultLoader == nil {
		workResultLoader = loadWorkResultFromContextPack
	}
	result, err := workResultLoader(contextPackDir)
	if err != nil {
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        runID,
			Action:       "blocked",
			Status:       "blocked",
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "workrun:" + runID,
			Summary:      "worker result is missing or invalid: " + err.Error(),
			BlockedBy:    []string{"worker-result"},
			Next:         fmt.Sprintf("@%s provide parseable work result", input.Role),
			OpenQuestion: "none",
		})
		if commentErr := s.CommentIssue(ctx, CommentIssueInput{
			IssueRef: issueRef,
			Actor:    input.Assignee,
			State:    "blocked",
			Body:     body,
		}); commentErr != nil {
			return leadIssueOutcome{}, commentErr
		}
		return leadIssueOutcome{processed: true, blocked: true, spawned: true}, nil
	}

	activeRunID, _, _ := s.cache.Get(ctx, leadActiveRunKey(input.Role, issueRef))
	if domainoutbox.IsStaleRun(activeRunID, result.RunID) {
		return leadIssueOutcome{processed: true, spawned: true}, nil
	}

	if err := domainoutbox.ValidateWorkResultEcho(workOrder, result.toDomain()); err != nil {
		body := buildStructuredComment(StructuredCommentInput{
			Role:         input.Role,
			IssueRef:     issueRef,
			RunID:        runID,
			Action:       "blocked",
			Status:       "blocked",
			ReadUpTo:     formatReadUpTo(input.CursorAfter),
			Trigger:      "workrun:" + runID,
			Summary:      "work result echo validation failed: " + err.Error(),
			BlockedBy:    []string{"work-result-echo"},
			Next:         fmt.Sprintf("@%s fix work result issue_ref/run_id", input.Role),
			OpenQuestion: "none",
		})
		if commentErr := s.CommentIssue(ctx, CommentIssueInput{
			IssueRef: issueRef,
			Actor:    input.Assignee,
			State:    "blocked",
			Body:     body,
		}); commentErr != nil {
			return leadIssueOutcome{}, commentErr
		}
		return leadIssueOutcome{processed: true, blocked: true, spawned: true}, nil
	}

	status := "review"
	action := "update"
	summary := "worker completed with evidence"
	next := "@integrator review and merge"

	if input.Role == "reviewer" {
		summary = "review completed with evidence"
		next = "@integrator finalize review decision"
	}

	if err := domainoutbox.ValidateWorkResultEvidence(result.toDomain()); err != nil {
		status = "blocked"
		action = "blocked"
		summary = "work result is missing required evidence: " + err.Error()
		next = fmt.Sprintf("@%s provide PR/Commit and Tests evidence", input.Role)
	}

	if input.Role == "reviewer" && strings.TrimSpace(result.ResultCode) == "review_changes_requested" {
		targetRole := nextRoleForReviewChanges(labels)
		status = "blocked"
		action = "blocked"
		summary = "review requested changes"
		next = fmt.Sprintf("@%s address review changes and rerun", targetRole)
	}

	if strings.TrimSpace(result.ResultCode) != "" && !(input.Role == "reviewer" && strings.TrimSpace(result.ResultCode) == "review_changes_requested") {
		status = "blocked"
		action = "blocked"
		summary = "worker reported result_code: " + result.ResultCode
		next = fmt.Sprintf("@%s investigate failure and rerun", input.Role)
	}

	body := buildStructuredComment(StructuredCommentInput{
		Role:         input.Role,
		IssueRef:     issueRef,
		RunID:        runID,
		Action:       action,
		Status:       status,
		ReadUpTo:     formatReadUpTo(input.CursorAfter),
		Trigger:      "workrun:" + runID,
		Summary:      summary,
		Changes:      result.Changes,
		Tests:        result.Tests,
		BlockedBy:    []string{"none"},
		OpenQuestion: "none",
		Next:         next,
	})
	if err := s.CommentIssue(ctx, CommentIssueInput{
		IssueRef: issueRef,
		Actor:    input.Assignee,
		State:    status,
		Body:     body,
	}); err != nil {
		return leadIssueOutcome{}, err
	}

	return leadIssueOutcome{
		processed: true,
		blocked:   status == "blocked",
		spawned:   true,
	}, nil
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

	stdoutFile, err := os.Create(filepath.Join(input.ContextPackDir, "stdout.log"))
	if err != nil {
		return err
	}
	defer stdoutFile.Close()

	stderrFile, err := os.Create(filepath.Join(input.ContextPackDir, "stderr.log"))
	if err != nil {
		return err
	}
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

func shouldSkipLeadSpawnByState(role string, labels []string) bool {
	state := currentStateLabel(labels)
	switch state {
	case "state:blocked", "state:done":
		return true
	case "state:review":
		return strings.TrimSpace(role) != "reviewer"
	default:
		return false
	}
}

func nextRoleForReviewChanges(labels []string) string {
	if containsString(labels, "to:frontend") && !containsString(labels, "to:backend") {
		return "frontend"
	}
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
