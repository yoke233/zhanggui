package outbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRoleRepoDirRelative(t *testing.T) {
	tempDir := t.TempDir()
	workflowFile := filepath.Join(tempDir, "configs", "workflow.toml")

	profile := workflowProfile{
		RoleRepo: map[string]string{
			"backend": "main",
		},
		Repos: map[string]string{
			"main": "../repo-main",
		},
	}

	got, err := resolveRoleRepoDir(profile, workflowFile, "backend")
	if err != nil {
		t.Fatalf("resolveRoleRepoDir() error = %v", err)
	}

	want := filepath.Clean(filepath.Join(filepath.Dir(workflowFile), "../repo-main"))
	if got != want {
		t.Fatalf("resolveRoleRepoDir() = %q, want %q", got, want)
	}
}

func TestResolveRoleRepoDirAbsolute(t *testing.T) {
	tempDir := t.TempDir()
	workflowFile := filepath.Join(tempDir, "workflow.toml")
	repoDir := filepath.Join(tempDir, "repo-main")

	profile := workflowProfile{
		RoleRepo: map[string]string{
			"backend": "main",
		},
		Repos: map[string]string{
			"main": repoDir,
		},
	}

	got, err := resolveRoleRepoDir(profile, workflowFile, "backend")
	if err != nil {
		t.Fatalf("resolveRoleRepoDir() error = %v", err)
	}
	if got != filepath.Clean(repoDir) {
		t.Fatalf("resolveRoleRepoDir() = %q, want %q", got, filepath.Clean(repoDir))
	}
}

func TestResolveRoleRepoDirMissingMappings(t *testing.T) {
	testCases := []struct {
		name    string
		profile workflowProfile
		wantErr string
	}{
		{
			name: "missing role_repo",
			profile: workflowProfile{
				RoleRepo: map[string]string{},
				Repos:    map[string]string{"main": "."},
			},
			wantErr: "role_repo mapping is required",
		},
		{
			name: "missing repos",
			profile: workflowProfile{
				RoleRepo: map[string]string{"backend": "main"},
				Repos:    map[string]string{},
			},
			wantErr: "repos mapping is required",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := resolveRoleRepoDir(testCase.profile, "workflow.toml", "backend")
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("resolveRoleRepoDir() error = %v, want contains %q", err, testCase.wantErr)
			}
		})
	}
}

func TestResolveExecutorDefaults(t *testing.T) {
	profile := workflowProfile{
		Executors: map[string]workflowExecutorConfig{},
	}

	got := resolveExecutor(profile, "backend")
	if got.Program != "go" {
		t.Fatalf("Program = %q", got.Program)
	}
	if len(got.Args) != 2 || got.Args[0] != "test" || got.Args[1] != "./..." {
		t.Fatalf("Args = %#v", got.Args)
	}
	if got.TimeoutSeconds != 1800 {
		t.Fatalf("TimeoutSeconds = %d", got.TimeoutSeconds)
	}
}

func TestResolveExecutorNormalizesEmptyFields(t *testing.T) {
	profile := workflowProfile{
		Executors: map[string]workflowExecutorConfig{
			"backend": {
				Program:        "   ",
				Args:           nil,
				TimeoutSeconds: 0,
			},
		},
	}

	got := resolveExecutor(profile, "backend")
	if got.Program != "go" {
		t.Fatalf("Program = %q", got.Program)
	}
	if len(got.Args) != 2 || got.Args[0] != "test" || got.Args[1] != "./..." {
		t.Fatalf("Args = %#v", got.Args)
	}
	if got.TimeoutSeconds != 1800 {
		t.Fatalf("TimeoutSeconds = %d", got.TimeoutSeconds)
	}
}

func TestIsRoleEnabledAndFindGroupByRole(t *testing.T) {
	profile := workflowProfile{
		Roles: workflowRolesConfig{
			Enabled: []string{" backend ", "qa"},
		},
		Groups: map[string]workflowGroupConfig{
			"backend-group": {
				Role: "backend",
			},
		},
	}

	if !isRoleEnabled(profile, "backend") {
		t.Fatalf("backend should be enabled")
	}
	if isRoleEnabled(profile, "frontend") {
		t.Fatalf("frontend should not be enabled")
	}
	if _, ok := findGroupByRole(profile, "backend"); !ok {
		t.Fatalf("backend group should be found")
	}
	if _, ok := findGroupByRole(profile, "frontend"); ok {
		t.Fatalf("frontend group should not be found")
	}
}

func TestResolveExecutor_UsesWorkflowCodexScripts(t *testing.T) {
	workflowPath := locateWorkflowFile(t)

	profile, err := loadWorkflowProfile(workflowPath)
	if err != nil {
		t.Fatalf("loadWorkflowProfile() error = %v", err)
	}

	testCases := []struct {
		role    string
		script  string
		timeout int
	}{
		{
			role:    "backend",
			script:  "scripts/codex-coder.ps1",
			timeout: 3600,
		},
		{
			role:    "reviewer",
			script:  "scripts/codex-review.ps1",
			timeout: 1800,
		},
		{
			role:    "qa",
			script:  "scripts/codex-test.ps1",
			timeout: 1800,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.role, func(t *testing.T) {
			got := resolveExecutor(profile, testCase.role)
			if got.Program != "pwsh" {
				t.Fatalf("Program = %q, want pwsh", got.Program)
			}
			if len(got.Args) != 3 {
				t.Fatalf("Args length = %d, want 3; args=%#v", len(got.Args), got.Args)
			}
			if got.Args[0] != "-NoProfile" || got.Args[1] != "-File" {
				t.Fatalf("Args prefix = %#v, want [-NoProfile -File ...]", got.Args)
			}
			if normalizeExecutorPathArg(got.Args[2]) != normalizeExecutorPathArg(testCase.script) {
				t.Fatalf("script arg = %q, want %q", got.Args[2], testCase.script)
			}
			if got.TimeoutSeconds != testCase.timeout {
				t.Fatalf("TimeoutSeconds = %d, want %d", got.TimeoutSeconds, testCase.timeout)
			}
		})
	}
}

func locateWorkflowFile(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}

	for {
		candidate := filepath.Join(dir, "workflow.toml")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("workflow.toml not found from %q", dir)
	return ""
}

func normalizeExecutorPathArg(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
}
