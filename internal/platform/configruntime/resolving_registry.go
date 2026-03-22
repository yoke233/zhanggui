package configruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/platform/config"
	"github.com/yoke233/zhanggui/internal/platform/profilellm"
)

type driverConfigResolver func(driverID string) (*core.DriverConfig, error)
type llmConfigResolver func(llmConfigID string) (*config.RuntimeLLMEntryConfig, error)

type resolvingRegistry struct {
	base          core.AgentRegistry
	resolveDriver driverConfigResolver
	resolveLLM    llmConfigResolver
}

func NewResolvingRegistry(base core.AgentRegistry, resolveDriver func(driverID string) (*core.DriverConfig, error), resolveLLM func(llmConfigID string) (*config.RuntimeLLMEntryConfig, error)) core.AgentRegistry {
	if base == nil {
		return nil
	}
	if resolveDriver == nil && resolveLLM == nil {
		return base
	}
	return &resolvingRegistry{
		base:          base,
		resolveDriver: resolveDriver,
		resolveLLM:    resolveLLM,
	}
}

func (r *resolvingRegistry) GetProfile(ctx context.Context, id string) (*core.AgentProfile, error) {
	return r.base.GetProfile(ctx, id)
}

func (r *resolvingRegistry) ListProfiles(ctx context.Context) ([]*core.AgentProfile, error) {
	return r.base.ListProfiles(ctx)
}

func (r *resolvingRegistry) CreateProfile(ctx context.Context, p *core.AgentProfile) error {
	return r.base.CreateProfile(ctx, p)
}

func (r *resolvingRegistry) UpdateProfile(ctx context.Context, p *core.AgentProfile) error {
	return r.base.UpdateProfile(ctx, p)
}

func (r *resolvingRegistry) DeleteProfile(ctx context.Context, id string) error {
	return r.base.DeleteProfile(ctx, id)
}

func (r *resolvingRegistry) ResolveForAction(ctx context.Context, action *core.Action) (*core.AgentProfile, error) {
	profile, err := r.base.ResolveForAction(ctx, action)
	if err != nil {
		return nil, err
	}
	return r.materializeProfile(profile)
}

func (r *resolvingRegistry) ResolveByID(ctx context.Context, profileID string) (*core.AgentProfile, error) {
	profile, err := r.base.ResolveByID(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return r.materializeProfile(profile)
}

func (r *resolvingRegistry) materializeProfile(profile *core.AgentProfile) (*core.AgentProfile, error) {
	if profile == nil {
		return nil, nil
	}

	cloned := cloneAgentProfile(profile)
	driverID := strings.TrimSpace(cloned.DriverID)
	if driverID != "" && r.resolveDriver != nil {
		driverCfg, err := r.resolveDriver(driverID)
		if err != nil {
			return nil, fmt.Errorf("resolve driver %q for profile %q: %w", driverID, cloned.ID, err)
		}
		cloned.Driver = cloneDriverConfig(driverCfg)
	}

	llmConfigID := strings.TrimSpace(cloned.LLMConfigID)
	if !profilellm.IsSystemLLMConfig(llmConfigID) && r.resolveLLM != nil {
		llmCfg, err := r.resolveLLM(llmConfigID)
		if err != nil {
			return nil, fmt.Errorf("resolve llm config %q for profile %q: %w", llmConfigID, cloned.ID, err)
		}
		if err := profilellm.ValidateDriverProviderCompatibility(cloned.DriverID, cloned.Driver.LaunchCommand, cloned.Driver.LaunchArgs, llmCfg.Type); err != nil {
			return nil, fmt.Errorf("profile %q llm config %q incompatible with driver: %w", cloned.ID, llmConfigID, err)
		}
		cloned.Driver.Env = profilellm.MergeEnv(profilellm.BuildEnv(NewProviderConfigFromEntry(llmCfg)), cloned.Driver.Env)
	}

	return cloned, nil
}

func cloneAgentProfile(profile *core.AgentProfile) *core.AgentProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	if profile.Capabilities != nil {
		cloned.Capabilities = append([]string(nil), profile.Capabilities...)
	}
	if profile.ActionsAllowed != nil {
		cloned.ActionsAllowed = append([]core.AgentAction(nil), profile.ActionsAllowed...)
	}
	if profile.Skills != nil {
		cloned.Skills = append([]string(nil), profile.Skills...)
	}
	if profile.MCP.Tools != nil {
		cloned.MCP.Tools = append([]string(nil), profile.MCP.Tools...)
	}
	cloned.Driver = cloneDriverConfig(&profile.Driver)
	return &cloned
}

func cloneDriverConfig(driver *core.DriverConfig) core.DriverConfig {
	if driver == nil {
		return core.DriverConfig{}
	}
	cloned := *driver
	if driver.LaunchArgs != nil {
		cloned.LaunchArgs = append([]string(nil), driver.LaunchArgs...)
	}
	if driver.SandboxArgs != nil {
		cloned.SandboxArgs = append([]string(nil), driver.SandboxArgs...)
	}
	if driver.Env != nil {
		cloned.Env = config.CloneStringMap(driver.Env)
	}
	return cloned
}
