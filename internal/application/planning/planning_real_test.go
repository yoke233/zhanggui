//go:build real
// +build real

package planning_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	llmadapter "github.com/yoke233/ai-workflow/internal/adapters/llm"
	llmplanning "github.com/yoke233/ai-workflow/internal/adapters/planning/llm"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	agentapp "github.com/yoke233/ai-workflow/internal/application/agent"
	planning "github.com/yoke233/ai-workflow/internal/application/planning"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func TestReal_PlanningGenerateAndMaterializeLLM(t *testing.T) {
	if os.Getenv("AI_WORKFLOW_REAL_PLANNING") == "" {
		t.Skip("set AI_WORKFLOW_REAL_PLANNING=1 to run")
	}

	cfgPath := strings.TrimSpace(os.Getenv("AI_WORKFLOW_REAL_PLANNING_CONFIG"))
	if cfgPath == "" {
		repoRoot, ok := findPlanningRepoRoot(t)
		if !ok {
			t.Skip("repo root not found")
		}
		cfgPath = filepath.Join(repoRoot, ".ai-workflow", "config.toml")
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skipf("missing planning config at %s", cfgPath)
	}

	cfg, err := config.LoadGlobal(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	llmEntry, ok := pickPlanningLLMConfig(cfg.Runtime.LLM)
	if !ok {
		t.Skip("runtime.llm has no usable LLM config")
	}

	apiKey := strings.TrimSpace(os.Getenv("AI_WORKFLOW_REAL_PLANNING_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(llmEntry.APIKey)
	}
	if apiKey == "" {
		t.Skip("planning real integration requires api key via runtime.llm config or AI_WORKFLOW_REAL_PLANNING_API_KEY")
	}

	baseURL := strings.TrimSpace(os.Getenv("AI_WORKFLOW_REAL_PLANNING_BASE_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(llmEntry.BaseURL)
	}
	model := strings.TrimSpace(os.Getenv("AI_WORKFLOW_REAL_PLANNING_MODEL"))
	if model == "" {
		model = strings.TrimSpace(llmEntry.Model)
	}
	if model == "" {
		t.Skip("planning real integration requires model via runtime.llm config or AI_WORKFLOW_REAL_PLANNING_MODEL")
	}

	client, err := llmadapter.New(llmadapter.Config{
		Provider:   llmEntry.Type,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Model:      model,
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("init llm client: %v", err)
	}

	registry := agentapp.NewConfigRegistryFromConfig(cfg.Runtime.Agents)
	profiles, err := registry.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("list planning profiles: %v", err)
	}
	if len(profiles) == 0 {
		t.Skip("runtime.agents.profiles is empty")
	}

	service := planning.NewService(llmplanning.NewCompleter(client), registry)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dag, err := service.Generate(ctx, "Plan a minimal backend feature delivery workflow for adding a health-check API endpoint with implementation and review. Keep it concise and executable.")
	if err != nil {
		t.Fatalf("Generate(real llm): %v", err)
	}
	if dag == nil || len(dag.Steps) == 0 {
		t.Fatalf("generated dag = %#v, want non-empty", dag)
	}
	if err := planning.ValidateGeneratedDAG(dag); err != nil {
		t.Fatalf("generated dag invalid: %v", err)
	}

	store := newPlanningRealIntegrationStore(t)
	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		Title:  "planning-real-integration",
		Status: core.WorkItemOpen,
	})
	if err != nil {
		t.Fatalf("CreateWorkItem() error = %v", err)
	}

	actions, err := service.Materialize(ctx, store, workItemID, dag)
	if err != nil {
		t.Fatalf("Materialize(real llm): %v", err)
	}
	if len(actions) != len(dag.Steps) {
		t.Fatalf("materialized actions = %d, generated steps = %d", len(actions), len(dag.Steps))
	}
	for i, action := range actions {
		if action.Position != i {
			t.Fatalf("action[%d] position = %d, want %d", i, action.Position, i)
		}
		if action.Name != dag.Steps[i].Name {
			t.Fatalf("action[%d].Name = %q, want %q", i, action.Name, dag.Steps[i].Name)
		}
	}
}

func newPlanningRealIntegrationStore(t *testing.T) core.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "planning-real-integration.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func findPlanningRepoRoot(t *testing.T) (string, bool) {
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

func pickPlanningLLMConfig(cfg config.RuntimeLLMConfig) (config.RuntimeLLMEntryConfig, bool) {
	wantID := strings.TrimSpace(os.Getenv("AI_WORKFLOW_REAL_PLANNING_LLM_CONFIG_ID"))
	if wantID == "" {
		wantID = strings.TrimSpace(cfg.DefaultConfigID)
	}
	if wantID != "" {
		for _, item := range cfg.Configs {
			if strings.TrimSpace(item.ID) != wantID {
				continue
			}
			if isPlanningLLMTypeSupported(item.Type) {
				return item, true
			}
			return config.RuntimeLLMEntryConfig{}, false
		}
	}
	for _, item := range cfg.Configs {
		if isPlanningLLMTypeSupported(item.Type) {
			return item, true
		}
	}
	return config.RuntimeLLMEntryConfig{}, false
}

func isPlanningLLMTypeSupported(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", llmadapter.ProviderOpenAIResponse, llmadapter.ProviderOpenAIChatCompletion, llmadapter.ProviderAnthropic:
		return true
	default:
		return false
	}
}
