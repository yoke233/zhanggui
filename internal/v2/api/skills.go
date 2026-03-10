package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

var skillNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

type skillInfo struct {
	Name       string `json:"name"`
	HasSkillMD bool   `json:"has_skill_md"`
}

type getSkillResponse struct {
	Name    string `json:"name"`
	SkillMD string `json:"skill_md"`
}

type createSkillRequest struct {
	Name    string `json:"name"`
	SkillMD string `json:"skill_md,omitempty"`
	GitHub  string `json:"github_url,omitempty"`
	Subdir  string `json:"subdir,omitempty"`
	DirName string `json:"dir_name,omitempty"`
}

type updateSkillRequest struct {
	SkillMD string `json:"skill_md"`
}

func registerSkillRoutes(r chi.Router, root string) {
	h := &skillsHandler{root: strings.TrimSpace(root)}
	r.Route("/skills", func(r chi.Router) {
		r.Get("/", h.listSkills)
		r.Post("/", h.createSkill)
		r.Get("/{skillName}", h.getSkill)
		r.Put("/{skillName}", h.updateSkill)
		r.Delete("/{skillName}", h.deleteSkill)
	})
}

type skillsHandler struct {
	root string
}

func (h *skillsHandler) skillsRoot() (string, error) {
	if h.root != "" {
		return filepath.Clean(h.root), nil
	}
	return "", errors.New("skills root is not configured")
}

func (h *skillsHandler) listSkills(w http.ResponseWriter, r *http.Request) {
	root, err := h.skillsRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "skills_root_error")
		return
	}
	_ = os.MkdirAll(root, 0o755)

	entries, err := os.ReadDir(root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "read_dir_error")
		return
	}
	out := make([]skillInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := strings.TrimSpace(e.Name())
		if name == "" || !skillNameRe.MatchString(name) {
			continue
		}
		_, statErr := os.Stat(filepath.Join(root, name, "SKILL.md"))
		out = append(out, skillInfo{
			Name:       name,
			HasSkillMD: statErr == nil,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

func (h *skillsHandler) getSkill(w http.ResponseWriter, r *http.Request) {
	name, ok := parseSkillName(w, r)
	if !ok {
		return
	}
	root, err := h.skillsRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "skills_root_error")
		return
	}
	path := filepath.Join(root, name, "SKILL.md")
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "skill not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "read_file_error")
		return
	}
	writeJSON(w, http.StatusOK, getSkillResponse{Name: name, SkillMD: string(b)})
}

func (h *skillsHandler) createSkill(w http.ResponseWriter, r *http.Request) {
	var req createSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "bad_request")
		return
	}

	name := strings.TrimSpace(req.Name)
	if req.DirName != "" && name == "" {
		name = strings.TrimSpace(req.DirName)
	}
	if name == "" || !skillNameRe.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid skill name", "invalid_name")
		return
	}

	if strings.TrimSpace(req.GitHub) != "" {
		writeError(w, http.StatusNotImplemented, "download from github not implemented yet", "not_implemented")
		return
	}

	root, err := h.skillsRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "skills_root_error")
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "mkdir_error")
		return
	}

	dir := filepath.Join(root, name)
	if _, err := os.Stat(dir); err == nil {
		writeError(w, http.StatusConflict, "skill already exists", "already_exists")
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, err.Error(), "stat_error")
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "mkdir_error")
		return
	}

	skillMD := strings.TrimSpace(req.SkillMD)
	if skillMD == "" {
		skillMD = defaultSkillMD(name)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD+"\n"), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "write_error")
		return
	}
	writeJSON(w, http.StatusCreated, skillInfo{Name: name, HasSkillMD: true})
}

func (h *skillsHandler) updateSkill(w http.ResponseWriter, r *http.Request) {
	name, ok := parseSkillName(w, r)
	if !ok {
		return
	}
	var req updateSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "bad_request")
		return
	}
	content := strings.TrimSpace(req.SkillMD)
	if content == "" {
		writeError(w, http.StatusBadRequest, "skill_md is required", "missing_field")
		return
	}

	root, err := h.skillsRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "skills_root_error")
		return
	}
	dir := filepath.Join(root, name)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "skill not found", "not_found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "stat_error")
			return
		}
		writeError(w, http.StatusBadRequest, "skill path is not a directory", "invalid_skill")
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content+"\n"), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "write_error")
		return
	}
	writeJSON(w, http.StatusOK, skillInfo{Name: name, HasSkillMD: true})
}

func (h *skillsHandler) deleteSkill(w http.ResponseWriter, r *http.Request) {
	name, ok := parseSkillName(w, r)
	if !ok {
		return
	}
	root, err := h.skillsRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "skills_root_error")
		return
	}
	dir := filepath.Join(root, name)
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "stat_error")
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "remove_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseSkillName(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := strings.TrimSpace(chi.URLParam(r, "skillName"))
	if name == "" || !skillNameRe.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid skill name", "invalid_name")
		return "", false
	}
	return name, true
}

func defaultSkillMD(name string) string {
	// Keep the stub minimal; users can update it via PUT.
	return strings.TrimSpace(`
---
name: ` + name + `
description: TODO
---

# ` + name + `
`)
}
