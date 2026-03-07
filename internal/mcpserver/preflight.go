//go:build dev

package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ProgressFunc is called after each preflight step completes.
type ProgressFunc func(step StepResult, stepIndex, totalSteps int)

// PreflightResult records the outcome of a quality gate run.
type PreflightResult struct {
	Success   bool          `json:"success"`
	CommitSHA string        `json:"commit_sha"`
	Timestamp time.Time     `json:"timestamp"`
	Steps     []StepResult  `json:"steps"`
	Duration  time.Duration `json:"duration"`
}

// StepResult records one preflight step.
type StepResult struct {
	Name     string `json:"name"`
	Success  bool   `json:"success"`
	Output   string `json:"output"`
	Duration string `json:"duration"`
}

// PreflightGate tracks the last successful preflight and gates restart.
type PreflightGate struct {
	mu               sync.Mutex
	last             *PreflightResult
	running          bool
	enforceCommitSHA bool // if true, restart requires preflight SHA == HEAD; default false
}

// NewPreflightGate creates a new gate.
func NewPreflightGate() *PreflightGate {
	return &PreflightGate{}
}

// LastResult returns the most recent preflight result.
func (g *PreflightGate) LastResult() *PreflightResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.last
}

// IsRunning returns whether a preflight is currently in progress.
func (g *PreflightGate) IsRunning() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running
}

// SetEnforceCommitSHA enables/disables commit SHA matching on restart.
// When disabled (default), only requires a successful preflight regardless of commit.
func (g *PreflightGate) SetEnforceCommitSHA(v bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enforceCommitSHA = v
}

// CanRestart checks if a restart is allowed: last preflight must have succeeded.
// If enforceCommitSHA is true, also requires the preflight SHA to match current HEAD.
func (g *PreflightGate) CanRestart(currentSHA string) (bool, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.last == nil {
		return false, "no preflight has been run yet; run self_preflight first"
	}
	if !g.last.Success {
		return false, fmt.Sprintf("last preflight failed at %s", g.last.Timestamp.Format(time.RFC3339))
	}
	if g.enforceCommitSHA && g.last.CommitSHA != currentSHA {
		return false, fmt.Sprintf(
			"preflight was for commit %s but HEAD is now %s; re-run self_preflight",
			g.last.CommitSHA[:min(8, len(g.last.CommitSHA))],
			currentSHA[:min(8, len(currentSHA))],
		)
	}
	return true, ""
}

// preflightStep defines one check to run.
type preflightStep struct {
	Name    string
	Command string
	Args    []string
}

// Run executes the full preflight quality gate.
// If onProgress is non-nil, it is called after each step completes.
func (g *PreflightGate) Run(ctx context.Context, sourceRoot string, frontendDir string, skipFrontend bool, onProgress ...ProgressFunc) (*PreflightResult, error) {
	g.mu.Lock()
	if g.running {
		g.mu.Unlock()
		return nil, fmt.Errorf("preflight already running")
	}
	g.running = true
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
	}()

	start := time.Now()

	// Get current commit SHA.
	sha, err := gitHeadSHA(ctx, sourceRoot)
	if err != nil {
		return nil, fmt.Errorf("get HEAD SHA: %w", err)
	}

	steps := []preflightStep{
		{"go vet", "go", []string{"vet", "./..."}},
		{"go build", "go", []string{"build", "-o", devNull(), "./cmd/ai-flow"}},
		{"go test", "go", []string{"test", "-p", "4", "-timeout", "10m", "-count=1", "./..."}},
	}
	if !skipFrontend {
		prefix := strings.TrimSpace(frontendDir)
		if prefix == "" {
			prefix = "web"
		}
		steps = append(steps,
			preflightStep{"frontend lint", "npm", []string{"--prefix", prefix, "run", "lint"}},
			preflightStep{"frontend typecheck", "npm", []string{"--prefix", prefix, "run", "typecheck"}},
			preflightStep{"frontend build", "npm", []string{"--prefix", prefix, "run", "build"}},
		)
	}

	results := make([]StepResult, 0, len(steps))
	allOK := true

	for i, step := range steps {
		if ctx.Err() != nil {
			results = append(results, StepResult{
				Name:    step.Name,
				Success: false,
				Output:  "cancelled",
			})
			allOK = false
			break
		}

		stepStart := time.Now()
		cmd := exec.CommandContext(ctx, step.Command, step.Args...)
		cmd.Dir = sourceRoot
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf

		runErr := cmd.Run()
		elapsed := time.Since(stepStart)
		output := buf.String()
		// Truncate long output to last 2000 chars.
		if len(output) > 2000 {
			output = "...(truncated)\n" + output[len(output)-2000:]
		}

		ok := runErr == nil
		sr := StepResult{
			Name:     step.Name,
			Success:  ok,
			Output:   output,
			Duration: elapsed.Round(time.Millisecond).String(),
		}
		results = append(results, sr)

		for _, fn := range onProgress {
			fn(sr, i, len(steps))
		}

		if !ok {
			allOK = false
			break // fail fast
		}
	}

	result := &PreflightResult{
		Success:   allOK,
		CommitSHA: sha,
		Timestamp: time.Now(),
		Steps:     results,
		Duration:  time.Since(start).Round(time.Millisecond),
	}

	g.mu.Lock()
	g.last = result
	g.mu.Unlock()

	return result, nil
}

func gitHeadSHA(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func devNull() string {
	if runtime.GOOS == "windows" {
		return "NUL"
	}
	return "/dev/null"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// postSystemEvent sends a system event to the server for WS broadcast.
func postSystemEvent(serverAddr, event string, data map[string]any) {
	if serverAddr == "" {
		return
	}
	body, _ := json.Marshal(map[string]any{"event": event, "data": data})
	req, err := http.NewRequest(http.MethodPost, serverAddr+"/api/v1/admin/ops/system-event", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
