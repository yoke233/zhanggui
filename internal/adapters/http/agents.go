package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
	skillset "github.com/yoke233/ai-workflow/internal/skills"
)

func registerAgentRoutes(r chi.Router, registry core.AgentRegistry) {
	if registry == nil {
		return
	}
	a := &agentsHandler{registry: registry}

	// Profiles
	r.Post("/agents/profiles", a.createProfile)
	r.Get("/agents/profiles", a.listProfiles)
	r.Get("/agents/profiles/{profileID}", a.getProfile)
	r.Put("/agents/profiles/{profileID}", a.updateProfile)
	r.Delete("/agents/profiles/{profileID}", a.deleteProfile)
}

type agentsHandler struct {
	registry core.AgentRegistry
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
	if err := a.registry.CreateProfile(r.Context(), &p); err != nil {
		writeRegistryError(w, err)
		return
	}
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
	if err := a.registry.UpdateProfile(r.Context(), &p); err != nil {
		writeRegistryError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (a *agentsHandler) deleteProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "profileID")
	if err := a.registry.DeleteProfile(r.Context(), id); err != nil {
		writeRegistryError(w, err)
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
