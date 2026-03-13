package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func storeBuiltinArtifact(ctx context.Context, store core.Store, bus core.EventBus, step *core.Action, execRec *core.Run, markdown string, metadata map[string]any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil {
		return fmt.Errorf("storeBuiltinArtifact: store is nil")
	}
	if step == nil || execRec == nil {
		return fmt.Errorf("storeBuiltinArtifact: step/exec is nil")
	}

	art := &core.Deliverable{
		RunID:          execRec.ID,
		ActionID:       step.ID,
		WorkItemID:     step.WorkItemID,
		ResultMarkdown: strings.TrimSpace(markdown),
		Metadata:       metadata,
	}
	artID, err := store.CreateDeliverable(ctx, art)
	if err != nil {
		return fmt.Errorf("storeBuiltinArtifact: create artifact: %w", err)
	}
	execRec.DeliverableID = &artID
	execRec.Output = map[string]any{"text": art.ResultMarkdown, "stop_reason": "builtin"}

	now := time.Now().UTC()
	if bus != nil {
		bus.Publish(ctx, core.Event{
			Type:       core.EventRunAgentOutput,
			WorkItemID: step.WorkItemID,
			ActionID:   step.ID,
			RunID:      execRec.ID,
			Timestamp: now,
			Data: map[string]any{
				"type":    "done",
				"content": art.ResultMarkdown,
			},
		})
	}
	return nil
}

// writeAskPassCmd writes a temporary Windows .cmd askpass helper that returns
// x-access-token as username and the provided token as password.
func writeAskPassCmd(token string) (path string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "ai-workflow-askpass-*")
	if err != nil {
		return "", nil, fmt.Errorf("create askpass dir: %w", err)
	}
	cmdPath := filepath.Join(dir, "git-askpass.cmd")

	// Avoid echoing the token to stdout; only print it when Git asks.
	content := strings.Join([]string{
		"@echo off",
		"set prompt=%~1",
		"echo %prompt% | findstr /i \"username\" >nul",
		"if %errorlevel%==0 (",
		"  echo x-access-token",
		"  exit /b 0",
		")",
		"echo %prompt% | findstr /i \"password\" >nul",
		"if %errorlevel%==0 (",
		"  echo " + token,
		"  exit /b 0",
		")",
		"echo " + token,
		"",
	}, "\r\n")

	if err := os.WriteFile(cmdPath, []byte(content), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("write askpass cmd: %w", err)
	}

	return cmdPath, func() { _ = os.RemoveAll(dir) }, nil
}

func asJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	// GitHub API (go-github) commonly surfaces "401 Bad credentials".
	if strings.Contains(s, "401") || strings.Contains(s, "bad credentials") {
		return true
	}
	// Git over HTTPS often reports these fragments.
	if strings.Contains(s, "authentication failed") || strings.Contains(s, "auth failed") {
		return true
	}
	// Some setups return 403 for insufficient/invalid token.
	if strings.Contains(s, "403") || strings.Contains(s, "forbidden") {
		return true
	}
	return false
}
