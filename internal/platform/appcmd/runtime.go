package appcmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/platform/config"
	"github.com/yoke233/zhanggui/internal/platform/configruntime"
	"github.com/yoke233/zhanggui/internal/skills"
)

type ensureExecutionProfilesResult struct {
	OK               bool     `json:"ok"`
	DriverID         string   `json:"driver_id"`
	ManagerProfileID string   `json:"manager_profile_id"`
	Materialized     []string `json:"materialized"`
	ConfigUpdated    []string `json:"config_updated"`
}

func RunRuntime(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ai-flow runtime ensure-execution-profiles [flags]")
	}
	switch strings.TrimSpace(args[0]) {
	case "ensure-execution-profiles":
		return runEnsureExecutionProfiles(os.Stdout, args[1:])
	default:
		return fmt.Errorf("unknown runtime command: %s", args[0])
	}
}

func runEnsureExecutionProfiles(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("ensure-execution-profiles", flag.ContinueOnError)
	fs.SetOutput(out)

	var driverID string
	var managerProfileID string
	var workerID string
	var reviewerID string
	fs.StringVar(&driverID, "driver-id", "", "driver ID to use for ensured execution profiles")
	fs.StringVar(&managerProfileID, "manager-profile", "ceo", "manager profile ID for ensured profiles")
	fs.StringVar(&workerID, "worker-id", "worker", "execution worker profile ID")
	fs.StringVar(&reviewerID, "reviewer-id", "reviewer", "review gate profile ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, dataDir, _, err := LoadConfig()
	if err != nil {
		return err
	}
	configPath := resolveGlobalConfigFilePath(dataDir)
	secretsPath := resolveSecretsFilePath(dataDir)
	skillsRoot := filepath.Join(dataDir, "skills")
	if err := skills.EnsureBuiltinSkills(skillsRoot); err != nil {
		return fmt.Errorf("ensure builtin skills: %w", err)
	}

	manager, err := configruntime.NewManager(configPath, secretsPath, configruntime.DisabledMCPEnv(), nil, nil)
	if err != nil {
		return fmt.Errorf("open runtime manager: %w", err)
	}

	selectedDriver, err := resolveExecutionDriver(cfg, strings.TrimSpace(driverID))
	if err != nil {
		return err
	}

	workerCfg := buildExecutionWorkerProfileConfig(strings.TrimSpace(workerID), strings.TrimSpace(managerProfileID), selectedDriver.ID)
	reviewerCfg := buildExecutionReviewerProfileConfig(strings.TrimSpace(reviewerID), strings.TrimSpace(managerProfileID), selectedDriver.ID)

	updatedConfig := make([]string, 0, 2)
	if updated, err := ensureRuntimeProfileConfig(context.Background(), manager, workerCfg); err != nil {
		return err
	} else if updated {
		updatedConfig = append(updatedConfig, workerCfg.ID)
	}
	if updated, err := ensureRuntimeProfileConfig(context.Background(), manager, reviewerCfg); err != nil {
		return err
	} else if updated {
		updatedConfig = append(updatedConfig, reviewerCfg.ID)
	}

	snap := manager.Current()
	if snap == nil || snap.Config == nil {
		return fmt.Errorf("runtime config unavailable after ensure")
	}

	storePath := ExpandStorePath(cfg.Store.Path, dataDir)
	runtimeDBPath := strings.TrimSuffix(storePath, filepath.Ext(storePath)) + "_runtime.db"
	store, err := sqlite.New(runtimeDBPath)
	if err != nil {
		return fmt.Errorf("open runtime store: %w", err)
	}
	defer store.Close()

	profilesByID := make(map[string]*core.AgentProfile)
	for _, profile := range configruntime.BuildAgents(snap.Config) {
		if profile == nil {
			continue
		}
		profilesByID[profile.ID] = profile
	}

	toMaterialize := []string{strings.TrimSpace(managerProfileID), workerCfg.ID, reviewerCfg.ID}
	materialized := make([]string, 0, len(toMaterialize))
	for _, profileID := range toMaterialize {
		if profileID == "" {
			continue
		}
		profile := profilesByID[profileID]
		if profile == nil {
			return fmt.Errorf("profile %q not found in runtime config after ensure", profileID)
		}
		if err := store.UpsertProfile(context.Background(), profile); err != nil {
			return fmt.Errorf("materialize profile %q: %w", profileID, err)
		}
		materialized = append(materialized, profileID)
	}

	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return enc.Encode(ensureExecutionProfilesResult{
		OK:               true,
		DriverID:         selectedDriver.ID,
		ManagerProfileID: strings.TrimSpace(managerProfileID),
		Materialized:     materialized,
		ConfigUpdated:    updatedConfig,
	})
}

func resolveExecutionDriver(cfg *config.Config, requested string) (config.RuntimeDriverConfig, error) {
	if cfg == nil {
		return config.RuntimeDriverConfig{}, fmt.Errorf("config is required")
	}
	drivers := cfg.Runtime.Agents.Drivers
	if len(drivers) == 0 {
		return config.RuntimeDriverConfig{}, fmt.Errorf("no runtime drivers configured")
	}
	if requested != "" {
		for _, driver := range drivers {
			if strings.TrimSpace(driver.ID) == requested {
				return driver, nil
			}
		}
		return config.RuntimeDriverConfig{}, fmt.Errorf("driver %q not found", requested)
	}
	for _, candidate := range []string{"codex-acp", "claude-acp"} {
		for _, driver := range drivers {
			if strings.TrimSpace(driver.ID) == candidate {
				return driver, nil
			}
		}
	}
	return drivers[0], nil
}

func ensureRuntimeProfileConfig(ctx context.Context, manager *configruntime.Manager, profile config.RuntimeProfileConfig) (bool, error) {
	if manager == nil {
		return false, fmt.Errorf("runtime manager is required")
	}
	current := manager.GetRuntime()
	for _, existing := range current.Agents.Profiles {
		if strings.TrimSpace(existing.ID) != profile.ID {
			continue
		}
		if _, err := manager.UpdateProfileConfig(ctx, profile.ID, profile); err != nil {
			return false, fmt.Errorf("update profile config %q: %w", profile.ID, err)
		}
		return true, nil
	}
	if _, err := manager.CreateProfileConfig(ctx, profile); err != nil {
		return false, fmt.Errorf("create profile config %q: %w", profile.ID, err)
	}
	return true, nil
}

func buildExecutionWorkerProfileConfig(profileID string, managerProfileID string, driverID string) config.RuntimeProfileConfig {
	return config.RuntimeProfileConfig{
		ID:               firstRuntimeValue(profileID, "worker"),
		Name:             "Worker Agent",
		ManagerProfileID: strings.TrimSpace(managerProfileID),
		Driver:           strings.TrimSpace(driverID),
		LLMConfigID:      "system",
		Role:             string(core.RoleWorker),
		Capabilities:     []string{"backend", "frontend", "test"},
		PromptTemplate:   "implement",
		Session: config.RuntimeSessionConfig{
			Reuse:    true,
			MaxTurns: 24,
			IdleTTL:  config.Duration{Duration: 15 * time.Minute},
		},
	}
}

func buildExecutionReviewerProfileConfig(profileID string, managerProfileID string, driverID string) config.RuntimeProfileConfig {
	return config.RuntimeProfileConfig{
		ID:               firstRuntimeValue(profileID, "reviewer"),
		Name:             "Reviewer Agent",
		ManagerProfileID: strings.TrimSpace(managerProfileID),
		Driver:           strings.TrimSpace(driverID),
		LLMConfigID:      "system",
		Role:             string(core.RoleGate),
		Capabilities:     []string{"review"},
		PromptTemplate:   "review",
		Session: config.RuntimeSessionConfig{
			Reuse:    true,
			MaxTurns: 16,
			IdleTTL:  config.Duration{Duration: 15 * time.Minute},
		},
	}
}

func firstRuntimeValue(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
