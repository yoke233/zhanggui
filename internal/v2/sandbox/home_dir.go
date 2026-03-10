package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/appdata"
	v2skills "github.com/yoke233/ai-workflow/internal/v2/skills"
)

// HomeDirSandbox isolates each ACP process by assigning a per-scope home/config directory:
//   - codex-acp:  CODEX_HOME = <dataDir>/acp-homes/codex/<profile>/<scope>
//   - claude-acp: CLAUDE_CONFIG_DIR = <dataDir>/acp-homes/claude/<profile>/<scope>
//
// It also:
//   - sets TMPDIR/TMP/TEMP to <home>/tmp
//   - links profile skills into <home>/skills
//   - links baseline auth/config files from the base home directory when present
type HomeDirSandbox struct {
	// DataDir points to the runtime data directory. If empty, uses appdata.ResolveDataDir().
	DataDir string

	// SkillsRoot points to the global shared skills repository (<dataDir>/skills).
	// It must not point at the sandbox-local skills directory.
	SkillsRoot string

	// RequireCodexAuth enforces presence of auth.json when running codex-acp.
	RequireCodexAuth bool
}

func (s HomeDirSandbox) Prepare(_ context.Context, in PrepareInput) (acpclient.LaunchConfig, error) {
	launch := in.Launch
	if launch.Env == nil {
		launch.Env = map[string]string{}
	}

	if in.Profile == nil || in.Driver == nil {
		return launch, nil
	}

	homeKey, baseHome, kind, err := detectHome(in.Driver.ID, in.Driver.Env, launch.Env)
	if err != nil {
		return launch, err
	}

	dataDir := filepath.Clean(s.DataDir)
	if strings.TrimSpace(dataDir) == "" {
		var err error
		dataDir, err = appdata.ResolveDataDir()
		if err != nil {
			return launch, err
		}
	}
	skillsRoot := filepath.Clean(s.SkillsRoot)
	if strings.TrimSpace(skillsRoot) == "" {
		skillsRoot = filepath.Join(dataDir, "skills")
	}

	profileID := sanitizeComponent(in.Profile.ID)
	scope := sanitizeComponent(in.Scope)
	if scope == "" {
		scope = "default"
	}

	home := filepath.Join(dataDir, "acp-homes", kind, profileID, scope)
	skillsDir := filepath.Join(home, "skills")
	tmpDir := filepath.Join(home, "tmp")

	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return launch, fmt.Errorf("create skills dir: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return launch, fmt.Errorf("create tmp dir: %w", err)
	}

	// Link baseline files from base home if present.
	switch kind {
	case "codex":
		_ = linkPathIfMissing(filepath.Join(skillsDir, ".system"), filepath.Join(baseHome, "skills", ".system"), true)
		if err := linkPathIfMissing(filepath.Join(home, "auth.json"), filepath.Join(baseHome, "auth.json"), false); err != nil {
			return launch, err
		}
		if s.RequireCodexAuth {
			if _, err := os.Stat(filepath.Join(home, "auth.json")); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return launch, fmt.Errorf("codex auth.json missing (base=%s, target=%s)", baseHome, home)
				}
				return launch, fmt.Errorf("stat auth.json: %w", err)
			}
		}
	case "claude":
		_ = linkPathIfMissing(filepath.Join(skillsDir, ".system"), filepath.Join(baseHome, "skills", ".system"), true)
		_ = linkPathIfMissing(filepath.Join(home, "CLAUDE.md"), filepath.Join(baseHome, "CLAUDE.md"), false)
	}

	// Install profile skills into the isolated home.
	if len(in.Profile.Skills) > 0 {
		if err := v2skills.EnsureSkillsLinked(skillsRoot, skillsDir, in.Profile.Skills); err != nil {
			return launch, fmt.Errorf("ensure skills linked: %w", err)
		}
	}

	launch.Env[homeKey] = home
	launch.Env["TMPDIR"] = tmpDir
	launch.Env["TMP"] = tmpDir
	launch.Env["TEMP"] = tmpDir

	return launch, nil
}

func detectHome(driverID string, driverEnv, launchEnv map[string]string) (homeKey, baseHome, kind string, err error) {
	id := strings.ToLower(strings.TrimSpace(driverID))
	switch {
	case strings.Contains(id, "codex"):
		homeKey, kind = "CODEX_HOME", "codex"
	case strings.Contains(id, "claude"):
		homeKey, kind = "CLAUDE_CONFIG_DIR", "claude"
	default:
		// Heuristic fallback based on available envs.
		if strings.TrimSpace(lookupEnv("CODEX_HOME", driverEnv, launchEnv)) != "" {
			homeKey, kind = "CODEX_HOME", "codex"
		} else if strings.TrimSpace(lookupEnv("CLAUDE_CONFIG_DIR", driverEnv, launchEnv)) != "" {
			homeKey, kind = "CLAUDE_CONFIG_DIR", "claude"
		} else {
			return "", "", "", fmt.Errorf("cannot infer agent home dir (driver id=%q)", driverID)
		}
	}

	baseHome = strings.TrimSpace(lookupEnv(homeKey, driverEnv, launchEnv))
	if baseHome == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", "", "", fmt.Errorf("resolve user home: %w", err)
		}
		if kind == "codex" {
			baseHome = filepath.Join(userHome, ".codex")
		} else {
			baseHome = filepath.Join(userHome, ".claude")
		}
	}
	baseHome = expandTilde(baseHome)
	if !filepath.IsAbs(baseHome) {
		if abs, err := filepath.Abs(baseHome); err == nil {
			baseHome = abs
		}
	}
	return homeKey, baseHome, kind, nil
}

func lookupEnv(key string, a, b map[string]string) string {
	if a != nil {
		if v, ok := a[key]; ok {
			return v
		}
	}
	if b != nil {
		if v, ok := b[key]; ok {
			return v
		}
	}
	return os.Getenv(key)
}

func expandTilde(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}

var invalidComponentRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeComponent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = invalidComponentRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "._-")
	if len(s) > 80 {
		s = s[:80]
	}
	if s == "" {
		return ""
	}
	return s
}

func linkPathIfMissing(dst, src string, wantDir bool) error {
	if _, err := os.Lstat(dst); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("lstat %s: %w", dst, err)
	}
	fi, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if fi.IsDir() != wantDir {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.Symlink(src, dst); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	if wantDir {
		return createWindowsJunction(dst, src)
	}

	return copyFile(dst, src)
}

func createWindowsJunction(dst, src string) error {
	cmd := exec.Command("cmd", "/c", "mklink", "/J", dst, src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("mklink /J failed: %s", msg)
	}
	return nil
}

func copyFile(dst, src string) error {
	b, rErr := os.ReadFile(src)
	if rErr != nil {
		return fmt.Errorf("read %s: %w", src, rErr)
	}
	if wErr := os.WriteFile(dst, b, 0o600); wErr != nil {
		return fmt.Errorf("write %s: %w", dst, wErr)
	}
	return nil
}
