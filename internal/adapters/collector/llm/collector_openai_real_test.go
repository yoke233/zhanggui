//go:build real
// +build real

package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func TestReal_OpenAICollector(t *testing.T) {
	if os.Getenv("AI_WORKFLOW_REAL_OPENAI") == "" {
		t.Skip("set AI_WORKFLOW_REAL_OPENAI=1 to run")
	}

	repoRoot, ok := findRepoRoot(t)
	if !ok {
		t.Skip("repo root not found")
	}
	cfgPath := filepath.Join(repoRoot, ".ai-workflow", "config.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skipf("missing config.toml at %s", cfgPath)
	}

	cfg, err := config.LoadGlobal(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	llmEntry, ok := pickCollectorLLMConfig(cfg.Runtime.LLM)
	if !ok {
		t.Skip("runtime.llm has no usable collector LLM config")
	}

	completer, err := NewCompleter(CompleterConfig{
		Provider:   llmEntry.Type,
		BaseURL:    llmEntry.BaseURL,
		APIKey:     llmEntry.APIKey,
		Model:      llmEntry.Model,
		MaxRetries: cfg.Runtime.Collector.MaxRetries,
	})
	if err != nil {
		t.Fatalf("init openai completer: %v", err)
	}

	collector := NewLLMCollector(completer.Complete)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	md := "## Changes\n- Added login endpoint in api/auth.go\n- Updated tests in api/auth_test.go\n\nAll tests pass."
	out, err := collector.Extract(ctx, core.ActionExec, md)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	// JSON schema contract checks (exec)
	if _, ok := out["summary"].(string); !ok {
		t.Fatalf("summary missing or not string: %#v", out["summary"])
	}
	if files, ok := out["files_changed"].([]any); !ok {
		t.Fatalf("files_changed missing or not array: %#v", out["files_changed"])
	} else if len(files) == 0 {
		t.Fatalf("files_changed empty (unexpected for this input): %#v", files)
	}
	if v, ok := out["tests_passed"]; ok && v != nil {
		if _, ok := v.(bool); !ok {
			t.Fatalf("tests_passed not bool or null: %#v", v)
		}
	}
	for k := range out {
		if k != "summary" && k != "files_changed" && k != "tests_passed" {
			t.Fatalf("unexpected key in output: %q", k)
		}
	}
}

func findRepoRoot(t *testing.T) (string, bool) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := wd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func pickCollectorLLMConfig(cfg config.RuntimeLLMConfig) (config.RuntimeLLMEntryConfig, bool) {
	wantID := strings.TrimSpace(os.Getenv("AI_WORKFLOW_REAL_COLLECTOR_LLM_CONFIG_ID"))
	if wantID == "" {
		wantID = strings.TrimSpace(cfg.DefaultConfigID)
	}
	if wantID != "" {
		for _, item := range cfg.Configs {
			if strings.TrimSpace(item.ID) != wantID {
				continue
			}
			if isCollectorLLMTypeSupported(item.Type) && strings.TrimSpace(item.APIKey) != "" && strings.TrimSpace(item.Model) != "" {
				return item, true
			}
			return config.RuntimeLLMEntryConfig{}, false
		}
	}
	for _, item := range cfg.Configs {
		if isCollectorLLMTypeSupported(item.Type) && strings.TrimSpace(item.APIKey) != "" && strings.TrimSpace(item.Model) != "" {
			return item, true
		}
	}
	return config.RuntimeLLMEntryConfig{}, false
}

func isCollectorLLMTypeSupported(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", ProviderOpenAIResponse, ProviderOpenAIChatCompletion, ProviderAnthropic:
		return true
	default:
		return false
	}
}
