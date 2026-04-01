package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	"github.com/yoke233/zhanggui/internal/platform/config"
)

func TestRunEnsureExecutionProfilesMaterializesWorkerAndReviewer(t *testing.T) {
	t.Setenv("AI_WORKFLOW_DATA_DIR", t.TempDir())

	var stdout bytes.Buffer
	if err := runEnsureExecutionProfiles(&stdout, []string{"--driver-id", "codex-acp"}); err != nil {
		t.Fatalf("runEnsureExecutionProfiles() error = %v", err)
	}

	var result ensureExecutionProfilesResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.OK {
		t.Fatalf("result.OK = false: %+v", result)
	}
	if result.DriverID != "codex-acp" {
		t.Fatalf("DriverID = %q, want codex-acp", result.DriverID)
	}

	cfg, dataDir, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	storePath := ExpandStorePath(cfg.Store.Path, dataDir)
	runtimeDBPath := strings.TrimSuffix(storePath, filepath.Ext(storePath)) + "_runtime.db"
	store, err := sqlite.New(runtimeDBPath)
	if err != nil {
		t.Fatalf("sqlite.New(runtime db) error = %v", err)
	}
	defer store.Close()

	worker, err := store.GetProfile(context.Background(), "worker")
	if err != nil {
		t.Fatalf("GetProfile(worker) error = %v", err)
	}
	if worker.DriverID != "codex-acp" {
		t.Fatalf("worker.DriverID = %q, want codex-acp", worker.DriverID)
	}
	if worker.ManagerProfileID != "ceo" {
		t.Fatalf("worker.ManagerProfileID = %q, want ceo", worker.ManagerProfileID)
	}

	reviewer, err := store.GetProfile(context.Background(), "reviewer")
	if err != nil {
		t.Fatalf("GetProfile(reviewer) error = %v", err)
	}
	if reviewer.DriverID != "codex-acp" {
		t.Fatalf("reviewer.DriverID = %q, want codex-acp", reviewer.DriverID)
	}
	if reviewer.ManagerProfileID != "ceo" {
		t.Fatalf("reviewer.ManagerProfileID = %q, want ceo", reviewer.ManagerProfileID)
	}

	reloaded, _, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig(reloaded) error = %v", err)
	}
	byID := make(map[string]config.RuntimeProfileConfig)
	for _, profile := range reloaded.Runtime.Agents.Profiles {
		byID[strings.TrimSpace(profile.ID)] = profile
	}
	if byID["worker"].Driver != "codex-acp" {
		t.Fatalf("config worker.driver = %q, want codex-acp", byID["worker"].Driver)
	}
	if byID["reviewer"].Driver != "codex-acp" {
		t.Fatalf("config reviewer.driver = %q, want codex-acp", byID["reviewer"].Driver)
	}
}
