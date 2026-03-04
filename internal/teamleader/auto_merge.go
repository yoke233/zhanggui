package teamleader

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// PRMerger abstracts PR creation and merge so AutoMergeHandler does not
// depend on the github package directly.
type PRMerger interface {
	OnImplementComplete(ctx context.Context, runID string) (string, error)
	OnMergeApproved(ctx context.Context, runID string) error
}

// AutoMergeHandler listens for RunDone events and auto-merges when
// Issue.AutoMerge is true and the test gate passes.
type AutoMergeHandler struct {
	store      core.Store
	bus        eventPublisher
	merger     PRMerger
	log        *slog.Logger
	testGateFn func(ctx context.Context, repoPath string) error // nil uses default runTestGate
}

// NewAutoMergeHandler creates a handler that triggers auto-merge on RunDone.
// merger may be nil; if nil, the handler publishes EventAutoMerged without
// performing actual PR operations.
func NewAutoMergeHandler(store core.Store, bus eventPublisher, merger PRMerger) *AutoMergeHandler {
	return &AutoMergeHandler{
		store:  store,
		bus:    bus,
		merger: merger,
		log:    slog.Default(),
	}
}

// OnEvent handles a single event. Only reacts to EventRunDone.
func (h *AutoMergeHandler) OnEvent(ctx context.Context, evt core.Event) {
	if evt.Type != core.EventRunDone {
		return
	}
	runID := strings.TrimSpace(evt.RunID)
	if runID == "" {
		return
	}

	issue, err := h.store.GetIssueByRun(runID)
	if err != nil || issue == nil {
		return
	}
	if !issue.AutoMerge {
		return
	}

	run, err := h.store.GetRun(runID)
	if err != nil || run == nil {
		return
	}

	project, err := h.store.GetProject(run.ProjectID)
	if err != nil || project == nil {
		h.log.Warn("auto-merge: project not found", "project_id", run.ProjectID)
		return
	}

	repoPath := strings.TrimSpace(project.RepoPath)
	if repoPath == "" {
		h.log.Warn("auto-merge: repo path empty", "project_id", project.ID)
		return
	}

	// Pre-merge gate: run go test ./...
	testGate := h.runTestGate
	if h.testGateFn != nil {
		testGate = h.testGateFn
	}
	h.log.Info("auto-merge: running test gate", "run_id", runID, "repo", repoPath)
	if err := testGate(ctx, repoPath); err != nil {
		h.log.Error("auto-merge: test gate failed", "run_id", runID, "error", err)
		h.bus.Publish(core.Event{
			Type:      core.EventRunFailed,
			RunID:     runID,
			ProjectID: project.ID,
			IssueID:   issue.ID,
			Error:     fmt.Sprintf("auto-merge test gate failed: %v", err),
			Data:      map[string]string{"phase": "auto_merge_test_gate"},
			Timestamp: time.Now(),
		})
		return
	}

	h.log.Info("auto-merge: test gate passed", "run_id", runID)

	var prURL string
	if h.merger != nil {
		var createErr error
		prURL, createErr = h.merger.OnImplementComplete(ctx, runID)
		if createErr != nil {
			h.log.Error("auto-merge: create PR failed", "run_id", runID, "error", createErr)
			h.bus.Publish(core.Event{
				Type:      core.EventRunFailed,
				RunID:     runID,
				ProjectID: project.ID,
				IssueID:   issue.ID,
				Error:     fmt.Sprintf("auto-merge create PR failed: %v", createErr),
				Data:      map[string]string{"phase": "auto_merge_create_pr"},
				Timestamp: time.Now(),
			})
			return
		}
		if mergeErr := h.merger.OnMergeApproved(ctx, runID); mergeErr != nil {
			h.log.Error("auto-merge: merge PR failed", "run_id", runID, "error", mergeErr)
			h.bus.Publish(core.Event{
				Type:      core.EventRunFailed,
				RunID:     runID,
				ProjectID: project.ID,
				IssueID:   issue.ID,
				Error:     fmt.Sprintf("auto-merge merge PR failed: %v", mergeErr),
				Data:      map[string]string{"phase": "auto_merge_merge_pr"},
				Timestamp: time.Now(),
			})
			return
		}
	}

	data := map[string]string{"branch": run.BranchName}
	if prURL != "" {
		data["pr_url"] = prURL
	}
	h.log.Info("auto-merge: merge complete", "run_id", runID, "pr_url", prURL)
	h.bus.Publish(core.Event{
		Type:      core.EventAutoMerged,
		RunID:     runID,
		ProjectID: project.ID,
		IssueID:   issue.ID,
		Data:      data,
		Timestamp: time.Now(),
	})
}

// runTestGate runs `go test` only on packages with changed .go files (vs main branch).
// Falls back to `go build ./...` if no Go files were changed.
func (h *AutoMergeHandler) runTestGate(ctx context.Context, repoPath string) error {
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	goBin, err := resolveGoBinary()
	if err != nil {
		return fmt.Errorf("go binary not found: %w", err)
	}

	pkgs, err := changedGoPackages(testCtx, repoPath)
	if err != nil {
		h.log.Warn("auto-merge: failed to detect changed packages, falling back to go build", "error", err)
		return runGoBuild(testCtx, goBin, repoPath)
	}
	if len(pkgs) == 0 {
		h.log.Info("auto-merge: no Go packages changed, running go build only")
		return runGoBuild(testCtx, goBin, repoPath)
	}

	h.log.Info("auto-merge: testing changed packages", "packages", pkgs)
	args := append([]string{"test"}, pkgs...)
	cmd := exec.CommandContext(testCtx, goBin, args...)
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

// changedGoPackages returns the list of Go package paths (./pkg/...) that have
// changed .go files compared to the main branch.
func changedGoPackages(ctx context.Context, repoPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "main...HEAD", "--", "*.go")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasSuffix(line, ".go") {
			continue
		}
		dir := "./" + line[:strings.LastIndex(line, "/")]
		if !seen[dir] {
			seen[dir] = true
		}
	}
	pkgs := make([]string, 0, len(seen))
	for p := range seen {
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

func runGoBuild(ctx context.Context, goBin, repoPath string) error {
	cmd := exec.CommandContext(ctx, goBin, "build", "./...")
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build ./... failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

// resolveGoBinary finds the go binary, checking PATH first then common install locations.
func resolveGoBinary() (string, error) {
	if p, err := exec.LookPath("go"); err == nil {
		return p, nil
	}
	for _, dir := range []string{"/usr/local/go/bin", "/usr/lib/go/bin"} {
		candidate := dir + "/go"
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("go not in PATH and not found in common locations")
}
