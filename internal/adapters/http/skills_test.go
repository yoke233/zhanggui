package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	agentapp "github.com/yoke233/ai-workflow/internal/application/agent"
	"github.com/yoke233/ai-workflow/internal/core"
	skillset "github.com/yoke233/ai-workflow/internal/skills"
	"net/http/httptest"
)

func setupSkillTestServer(t *testing.T) (*httptest.Server, *agentapp.ConfigRegistry, string) {
	return setupSkillTestServerWithImporter(t, nil)
}

func setupSkillTestServerWithImporter(t *testing.T, importer skillset.GitHubImporter) (*httptest.Server, *agentapp.ConfigRegistry, string) {
	t.Helper()

	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)
	skillsRoot := filepath.Join(dataDir, "skills")

	registry := agentapp.NewConfigRegistry()

	h := NewHandler(nil, nil, nil,
		WithRegistry(registry),
		WithSkillsRoot(skillsRoot),
		WithSkillGitHubImporter(importer),
	)
	r := chi.NewRouter()
	h.Register(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts, registry, skillsRoot
}

func writeSkillFile(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill %s: %v", name, err)
	}
}

func TestSkillRoutesExposeValidationAndProfileRefs(t *testing.T) {
	ts, registry, skillsRoot := setupSkillTestServer(t)

	writeSkillFile(t, skillsRoot, "strict-review", skillset.DefaultSkillMD("strict-review"))
	writeSkillFile(t, skillsRoot, "broken-skill", "# broken")
	registry.LoadProfiles([]*core.AgentProfile{
		{ID: "worker-a", Name: "worker-a", Role: core.RoleWorker, Skills: []string{"strict-review", "broken-skill"}},
	})

	resp, err := getJSON(ts, "/skills/")
	if err != nil {
		t.Fatalf("GET /skills: %v", err)
	}
	requireStatus(t, resp, 200)
	var list []skillInfo
	decodeJSON(resp, &list)
	if len(list) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(list))
	}

	var broken skillInfo
	for _, item := range list {
		if item.Name == "broken-skill" {
			broken = item
			break
		}
	}
	if broken.Name == "" {
		t.Fatal("expected broken-skill in list")
	}
	if broken.Valid {
		t.Fatalf("expected broken-skill invalid, got %+v", broken)
	}
	if len(broken.ProfilesUsing) != 1 || broken.ProfilesUsing[0] != "worker-a" {
		t.Fatalf("expected profile refs for broken skill, got %+v", broken.ProfilesUsing)
	}

	resp, err = getJSON(ts, "/skills/broken-skill")
	if err != nil {
		t.Fatalf("GET /skills/broken-skill: %v", err)
	}
	requireStatus(t, resp, 200)
	var detail getSkillResponse
	decodeJSON(resp, &detail)
	if detail.Valid {
		t.Fatalf("expected invalid detail response, got %+v", detail)
	}
	if len(detail.ValidationErrors) == 0 {
		t.Fatalf("expected validation errors in detail, got %+v", detail)
	}
}

func TestCreateSkillRejectsInvalidSkillMD(t *testing.T) {
	ts, _, _ := setupSkillTestServer(t)

	resp, err := postJSON(ts, "/skills/", map[string]any{
		"name":     "bad-skill",
		"skill_md": "# invalid",
	})
	if err != nil {
		t.Fatalf("POST /skills: %v", err)
	}
	requireStatus(t, resp, 400)
	var body map[string]any
	decodeJSON(resp, &body)
	if body["code"] != "invalid_skill_md" {
		t.Fatalf("expected invalid_skill_md, got %+v", body)
	}
}

func TestDeleteSkillFailsWhenReferenced(t *testing.T) {
	ts, registry, skillsRoot := setupSkillTestServer(t)

	writeSkillFile(t, skillsRoot, "strict-review", skillset.DefaultSkillMD("strict-review"))
	registry.LoadProfiles([]*core.AgentProfile{
		{ID: "worker-a", Name: "worker-a", Role: core.RoleWorker, Skills: []string{"strict-review"}},
	})

	resp, err := deleteReq(ts, "/skills/strict-review")
	if err != nil {
		t.Fatalf("DELETE /skills/strict-review: %v", err)
	}
	requireStatus(t, resp, 409)
	var body map[string]any
	decodeJSON(resp, &body)
	if body["code"] != "skill_in_use" {
		t.Fatalf("expected skill_in_use, got %+v", body)
	}
}

func TestCreateProfileRejectsInvalidSkills(t *testing.T) {
	ts, _, skillsRoot := setupSkillTestServer(t)
	writeSkillFile(t, skillsRoot, "strict-review", skillset.DefaultSkillMD("strict-review"))

	resp, err := postJSON(ts, "/agents/profiles", map[string]any{
		"id":     "worker-a",
		"role":   "worker",
		"skills": []string{"strict-review", "missing-skill"},
	})
	if err != nil {
		t.Fatalf("POST /agents/profiles: %v", err)
	}
	requireStatus(t, resp, 400)
	var body map[string]any
	decodeJSON(resp, &body)
	if body["code"] != "invalid_skills" {
		t.Fatalf("expected invalid_skills, got %+v", body)
	}
}

func TestCreateSkillDefaultStubIsValid(t *testing.T) {
	ts, _, _ := setupSkillTestServer(t)

	resp, err := postJSON(ts, "/skills/", map[string]any{
		"name": "new-skill",
	})
	if err != nil {
		t.Fatalf("POST /skills default stub: %v", err)
	}
	requireStatus(t, resp, 201)

	resp, err = getJSON(ts, "/skills/new-skill")
	if err != nil {
		t.Fatalf("GET /skills/new-skill: %v", err)
	}
	requireStatus(t, resp, 200)
	var detail getSkillResponse
	decodeJSON(resp, &detail)
	if !detail.Valid || detail.Metadata == nil || detail.Metadata.Name != "new-skill" {
		t.Fatalf("expected valid default stub, got %+v", detail)
	}
}

func TestImportGitHubSkillSuccess(t *testing.T) {
	ts, _, _ := setupSkillTestServerWithImporter(t, &stubGitHubImporter{
		imported: &skillset.ParsedSkill{
			Name:       "vercel-react-best-practices",
			HasSkillMD: true,
			Valid:      true,
			Metadata: &skillset.Metadata{
				Name:        "vercel-react-best-practices",
				Description: "React best practices",
			},
		},
	})

	resp, err := postJSON(ts, "/skills/import/github", map[string]any{
		"repo_url":   "https://github.com/vercel-labs/agent-skills",
		"skill_name": "vercel-react-best-practices",
	})
	if err != nil {
		t.Fatalf("POST /skills/import/github: %v", err)
	}
	requireStatus(t, resp, 201)
	var body skillInfo
	decodeJSON(resp, &body)
	if body.Name != "vercel-react-best-practices" || !body.Valid {
		t.Fatalf("unexpected import response: %+v", body)
	}
}

func TestImportGitHubSkillRejectsInvalidRepoURL(t *testing.T) {
	ts, _, _ := setupSkillTestServerWithImporter(t, &stubGitHubImporter{
		err: errors.Join(skillset.ErrInvalidGitHubRepoURL, errors.New("remote_url is required")),
	})

	resp, err := postJSON(ts, "/skills/import/github", map[string]any{
		"repo_url":   "notaurl",
		"skill_name": "vercel-react-best-practices",
	})
	if err != nil {
		t.Fatalf("POST /skills/import/github invalid repo: %v", err)
	}
	requireStatus(t, resp, 400)
	var body map[string]any
	decodeJSON(resp, &body)
	if body["code"] != "invalid_repo_url" {
		t.Fatalf("expected invalid_repo_url, got %+v", body)
	}
}

func TestImportGitHubSkillReturnsNotFound(t *testing.T) {
	ts, _, _ := setupSkillTestServerWithImporter(t, &stubGitHubImporter{
		err: errors.Join(skillset.ErrGitHubSkillNotFound, errors.New("missing-skill")),
	})

	resp, err := postJSON(ts, "/skills/import/github", map[string]any{
		"repo_url":   "https://github.com/vercel-labs/agent-skills",
		"skill_name": "missing-skill",
	})
	if err != nil {
		t.Fatalf("POST /skills/import/github missing skill: %v", err)
	}
	requireStatus(t, resp, 404)
	var body map[string]any
	decodeJSON(resp, &body)
	if body["code"] != "repo_skill_not_found" {
		t.Fatalf("expected repo_skill_not_found, got %+v", body)
	}
}

func TestImportGitHubSkillReturnsValidationErrors(t *testing.T) {
	ts, _, _ := setupSkillTestServerWithImporter(t, &stubGitHubImporter{
		err: &skillset.RepoSkillValidationError{
			Name:             "broken-skill",
			ValidationErrors: []string{"missing YAML frontmatter"},
		},
	})

	resp, err := postJSON(ts, "/skills/import/github", map[string]any{
		"repo_url":   "https://github.com/vercel-labs/agent-skills",
		"skill_name": "broken-skill",
	})
	if err != nil {
		t.Fatalf("POST /skills/import/github invalid skill: %v", err)
	}
	requireStatus(t, resp, 400)
	var body map[string]any
	decodeJSON(resp, &body)
	if body["code"] != "invalid_skill_md" {
		t.Fatalf("expected invalid_skill_md, got %+v", body)
	}
}

func TestSkillRoutesWorkWithoutRegistry(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)
	skillsRoot := filepath.Join(dataDir, "skills")
	writeSkillFile(t, skillsRoot, "strict-review", skillset.DefaultSkillMD("strict-review"))

	h := NewHandler(nil, nil, nil, WithSkillsRoot(skillsRoot))
	r := chi.NewRouter()
	h.Register(r)
	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := getJSON(ts, "/skills/")
	if err != nil {
		t.Fatalf("GET /skills without registry: %v", err)
	}
	requireStatus(t, resp, 200)
	var list []skillInfo
	decodeJSON(resp, &list)
	if len(list) != 1 || len(list[0].ProfilesUsing) != 0 {
		t.Fatalf("expected empty profile refs without registry, got %+v", list)
	}
}

func TestProfileValidationUsesResolvedSkillsRoot(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)
	skillsRoot := filepath.Join(dataDir, "skills")
	writeSkillFile(t, skillsRoot, "strict-review", skillset.DefaultSkillMD("strict-review"))

	registry := agentapp.NewConfigRegistry()
	err := registry.CreateProfile(context.Background(), &core.AgentProfile{
		ID:     "worker-a",
		Role:   core.RoleWorker,
		Skills: []string{"strict-review"},
	})
	if err != nil {
		t.Fatalf("expected resolved skills root to validate profile, got %v", err)
	}
}

type stubGitHubImporter struct {
	imported *skillset.ParsedSkill
	err      error
}

func (s *stubGitHubImporter) Import(_ context.Context, _ string, _ skillset.GitHubImportRequest) (*skillset.ParsedSkill, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.imported, nil
}
