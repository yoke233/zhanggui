package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/platform/config"
	"github.com/yoke233/zhanggui/internal/platform/configruntime"
	"github.com/yoke233/zhanggui/internal/platform/profilellm"
	skillset "github.com/yoke233/zhanggui/internal/skills"
)

type DriverConfigService interface {
	ListDriverConfigs() []config.RuntimeDriverConfig
	ResolveDriverConfig(driverID string) (*core.DriverConfig, error)
	ResolveLLMConfig(llmConfigID string) (*config.RuntimeLLMEntryConfig, error)
	CreateDriverConfig(ctx context.Context, driver config.RuntimeDriverConfig) (*configruntime.Snapshot, error)
	UpdateDriverConfig(ctx context.Context, driverID string, driver config.RuntimeDriverConfig) (*configruntime.Snapshot, error)
	DeleteDriverConfig(ctx context.Context, driverID string) (*configruntime.Snapshot, error)
	CreateProfileConfig(ctx context.Context, profile config.RuntimeProfileConfig) (*configruntime.Snapshot, error)
	UpdateProfileConfig(ctx context.Context, profileID string, profile config.RuntimeProfileConfig) (*configruntime.Snapshot, error)
	DeleteProfileConfig(ctx context.Context, profileID string) (*configruntime.Snapshot, error)
}

func registerAgentRoutes(r chi.Router, registry core.AgentRegistry, drivers DriverConfigService) {
	if registry == nil && drivers == nil {
		return
	}
	a := &agentsHandler{registry: registry, drivers: drivers}

	// Profiles
	if registry != nil {
		r.Post("/agents/profiles", a.createProfile)
		r.Get("/agents/profiles", a.listProfiles)
		r.Get("/agents/profiles/{profileID}", a.getProfile)
		r.Put("/agents/profiles/{profileID}", a.updateProfile)
		r.Delete("/agents/profiles/{profileID}", a.deleteProfile)
	}
	if drivers != nil {
		r.Get("/agents/drivers", a.listDrivers)
		r.Post("/agents/drivers", a.createDriver)
		r.Put("/agents/drivers/{driverID}", a.updateDriver)
		r.Delete("/agents/drivers/{driverID}", a.deleteDriver)
	}
}

type agentsHandler struct {
	registry core.AgentRegistry
	drivers  DriverConfigService
}

// --- Profiles ---

func (a *agentsHandler) createProfile(w http.ResponseWriter, r *http.Request) {
	var p core.AgentProfile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if p.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := a.normalizeProfileConfig(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.registry.CreateProfile(r.Context(), &p); err != nil {
		writeRegistryError(w, err)
		return
	}
	a.syncProfileToConfig(r.Context(), &p)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

func (a *agentsHandler) listProfiles(w http.ResponseWriter, r *http.Request) {
	list, err := a.registry.ListProfiles(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (a *agentsHandler) getProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "profileID")
	p, err := a.registry.GetProfile(r.Context(), id)
	if err != nil {
		writeRegistryError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (a *agentsHandler) updateProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "profileID")
	var p core.AgentProfile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.ID = id
	if err := a.normalizeProfileConfig(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.registry.UpdateProfile(r.Context(), &p); err != nil {
		writeRegistryError(w, err)
		return
	}
	a.syncProfileToConfig(r.Context(), &p)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (a *agentsHandler) deleteProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "profileID")
	if err := a.registry.DeleteProfile(r.Context(), id); err != nil {
		writeRegistryError(w, err)
		return
	}
	if a.drivers != nil {
		if _, err := a.drivers.DeleteProfileConfig(r.Context(), id); err != nil {
			slog.Warn("profile deleted from store but failed to sync config.toml", "id", id, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Drivers ---

func (a *agentsHandler) listDrivers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.drivers.ListDriverConfigs())
}

func (a *agentsHandler) createDriver(w http.ResponseWriter, r *http.Request) {
	var driver config.RuntimeDriverConfig
	if err := json.NewDecoder(r.Body).Decode(&driver); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	driver.ID = strings.TrimSpace(driver.ID)
	if driver.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if _, err := a.drivers.CreateDriverConfig(r.Context(), driver); err != nil {
		writeDriverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, driver)
}

func (a *agentsHandler) updateDriver(w http.ResponseWriter, r *http.Request) {
	driverID := strings.TrimSpace(chi.URLParam(r, "driverID"))
	var driver config.RuntimeDriverConfig
	if err := json.NewDecoder(r.Body).Decode(&driver); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := a.drivers.UpdateDriverConfig(r.Context(), driverID, driver); err != nil {
		writeDriverError(w, err)
		return
	}
	driver.ID = driverID
	writeJSON(w, http.StatusOK, driver)
}

func (a *agentsHandler) deleteDriver(w http.ResponseWriter, r *http.Request) {
	driverID := strings.TrimSpace(chi.URLParam(r, "driverID"))
	if _, err := a.drivers.DeleteDriverConfig(r.Context(), driverID); err != nil {
		writeDriverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeRegistryError maps registry errors to HTTP status codes.
func writeRegistryError(w http.ResponseWriter, err error) {
	msg := err.Error()
	var invalidSkills *skillset.InvalidSkillsError
	if errors.As(err, &invalidSkills) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":  msg,
			"code":   "invalid_skills",
			"skills": invalidSkills.Issues,
		})
		return
	}
	switch {
	case isErrorType(err, core.ErrProfileNotFound),
		isErrorType(err, core.ErrNoMatchingAgent):
		http.Error(w, msg, http.StatusNotFound)
	case isErrorType(err, core.ErrDuplicateProfile):
		http.Error(w, msg, http.StatusConflict)
	case isErrorType(err, core.ErrCapabilityOverflow):
		http.Error(w, msg, http.StatusUnprocessableEntity)
	default:
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

func writeDriverError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case isErrorType(err, configruntime.ErrDriverNotFound):
		http.Error(w, msg, http.StatusNotFound)
	case isErrorType(err, configruntime.ErrDuplicateDriver):
		http.Error(w, msg, http.StatusConflict)
	case isErrorType(err, configruntime.ErrDriverInUse):
		http.Error(w, msg, http.StatusConflict)
	default:
		http.Error(w, msg, http.StatusBadRequest)
	}
}

// isErrorType checks if err's chain contains the target sentinel.
func isErrorType(err, target error) bool {
	for e := err; e != nil; e = unwrapError(e) {
		if e == target {
			return true
		}
	}
	return false
}

func unwrapError(err error) error {
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return nil
}

// syncProfileToConfig persists the profile into config.toml so it survives restarts.
func (a *agentsHandler) syncProfileToConfig(ctx context.Context, p *core.AgentProfile) {
	if a.drivers == nil {
		return
	}
	rc := configruntime.CoreProfileToRuntimeConfig(p)
	if _, err := a.drivers.UpdateProfileConfig(ctx, p.ID, rc); err != nil {
		// Profile may not exist in config.toml yet (created via API only); try create.
		if _, err2 := a.drivers.CreateProfileConfig(ctx, rc); err2 != nil {
			slog.Warn("failed to sync profile to config.toml", "id", p.ID, "update_err", err, "create_err", err2)
		}
	}
}

func (a *agentsHandler) normalizeProfileConfig(p *core.AgentProfile) error {
	if p == nil || a.drivers == nil {
		return nil
	}

	if driverID := strings.TrimSpace(p.DriverID); driverID != "" {
		driverCfg, err := a.drivers.ResolveDriverConfig(driverID)
		if err != nil {
			return fmt.Errorf("resolve driver %q: %w", driverID, err)
		}
		p.Driver = *driverCfg
	}

	llmConfigID := strings.TrimSpace(p.LLMConfigID)
	if profilellm.IsSystemLLMConfig(llmConfigID) {
		return nil
	}
	llmCfg, err := a.drivers.ResolveLLMConfig(llmConfigID)
	if err != nil {
		return fmt.Errorf("resolve llm config %q: %w", llmConfigID, err)
	}
	if err := profileCompatibleWithRuntimeLLM(p, llmCfg); err != nil {
		return err
	}
	return nil
}

func profileCompatibleWithRuntimeLLM(p *core.AgentProfile, llmCfg *config.RuntimeLLMEntryConfig) error {
	if p == nil || llmCfg == nil {
		return nil
	}
	if err := profilellm.ValidateDriverProviderCompatibility(p.DriverID, p.Driver.LaunchCommand, p.Driver.LaunchArgs, llmCfg.Type); err != nil {
		return fmt.Errorf("profile %q llm_config_id %q incompatible with driver: %w", p.ID, llmCfg.ID, err)
	}
	return nil
}
