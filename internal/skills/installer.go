package skills

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/appdata"
)

// EnsureSkillsLinked links each skill directory from skillsRoot into targetSkillsDir.
func EnsureSkillsLinked(skillsRoot, targetSkillsDir string, skillNames []string) error {
	if len(skillNames) == 0 {
		return nil
	}
	if strings.TrimSpace(skillsRoot) == "" {
		return fmt.Errorf("skills root is empty")
	}
	if strings.TrimSpace(targetSkillsDir) == "" {
		return fmt.Errorf("target skills dir is empty")
	}
	if err := os.MkdirAll(targetSkillsDir, 0o755); err != nil {
		return fmt.Errorf("create target skills dir: %w", err)
	}

	for _, raw := range skillNames {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !IsValidName(name) {
			return fmt.Errorf("invalid skill name %q", name)
		}
		skill, inspectErr := InspectSkill(skillsRoot, name)
		if inspectErr != nil {
			if errors.Is(inspectErr, os.ErrNotExist) {
				return fmt.Errorf("skill %q not found at %s", name, filepath.Join(skillsRoot, name))
			}
			return fmt.Errorf("inspect skill %q: %w", name, inspectErr)
		}
		if !skill.Valid {
			return fmt.Errorf("skill %q is invalid: %s", name, strings.Join(skill.ValidationErrors, "; "))
		}
		src := filepath.Join(skillsRoot, name)

		dst := filepath.Join(targetSkillsDir, name)
		if _, statErr := os.Lstat(dst); statErr == nil {
			// Already present (either link or a real folder). Keep it as-is.
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("lstat target skill %q: %w", name, statErr)
		}

		if err := linkDir(dst, src); err != nil {
			// If another goroutine/process created it concurrently, treat as success.
			if _, statErr := os.Lstat(dst); statErr == nil {
				continue
			}
			return fmt.Errorf("link skill %q: %w", name, err)
		}
	}

	return nil
}

// EnsureProfileSkills ensures the given profile's skills are available to the agent process
// by linking the global shared skills root into the agent home skills directory.
//
// Target locations:
//   - codex-acp:  $CODEX_HOME/skills
//   - claude-acp: $CLAUDE_CONFIG_DIR/skills
func EnsureProfileSkills(profile *core.AgentProfile) error {
	if profile == nil {
		return nil
	}
	if len(profile.Skills) == 0 {
		return nil
	}

	skillsRoot, err := resolveSkillsRoot()
	if err != nil {
		return err
	}

	targetSkillsDir, err := resolveTargetSkillsDir(profile)
	if err != nil {
		return err
	}
	return EnsureSkillsLinked(skillsRoot, targetSkillsDir, profile.Skills)
}

func resolveSkillsRoot() (string, error) {
	dataDir, err := appdata.ResolveDataDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(dataDir, "skills")
	return filepath.Clean(root), nil
}

// ResolveSkillsRoot returns the on-disk directory where ai-workflow stores skills.
// Default: <dataDir>/skills, where dataDir is resolved by appdata.ResolveDataDir().
func ResolveSkillsRoot() (string, error) {
	return resolveSkillsRoot()
}

// EnsureProfileSkillsFromRoot ensures the given profile's skills are linked from an explicit
// global shared skills root into the target agent home skills directory.
func EnsureProfileSkillsFromRoot(profile *core.AgentProfile, skillsRoot string) error {
	if profile == nil {
		return nil
	}
	if len(profile.Skills) == 0 {
		return nil
	}
	targetSkillsDir, err := resolveTargetSkillsDir(profile)
	if err != nil {
		return err
	}
	return EnsureSkillsLinked(skillsRoot, targetSkillsDir, profile.Skills)
}

func resolveTargetSkillsDir(profile *core.AgentProfile) (string, error) {
	if profile == nil {
		return "", fmt.Errorf("nil profile")
	}

	id := strings.ToLower(strings.TrimSpace(profile.ID))
	cmd := strings.ToLower(strings.TrimSpace(profile.Driver.LaunchCommand))
	homeKey := ""
	defaultDirName := ""

	switch {
	case strings.Contains(id, "codex") || strings.Contains(cmd, "codex"):
		homeKey = "CODEX_HOME"
		defaultDirName = ".codex"
	case strings.Contains(id, "claude") || strings.Contains(cmd, "claude"):
		homeKey = "CLAUDE_CONFIG_DIR"
		defaultDirName = ".claude"
	default:
		// Fallback: infer by env keys present.
		if _, ok := profile.Driver.Env["CODEX_HOME"]; ok || strings.TrimSpace(os.Getenv("CODEX_HOME")) != "" {
			homeKey = "CODEX_HOME"
			defaultDirName = ".codex"
		} else if _, ok := profile.Driver.Env["CLAUDE_CONFIG_DIR"]; ok || strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")) != "" {
			homeKey = "CLAUDE_CONFIG_DIR"
			defaultDirName = ".claude"
		} else {
			return "", fmt.Errorf("cannot infer agent home dir (profile id=%q)", profile.ID)
		}
	}

	home := strings.TrimSpace(profile.Driver.Env[homeKey])
	if home == "" {
		home = strings.TrimSpace(os.Getenv(homeKey))
	}
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		home = filepath.Join(userHome, defaultDirName)
	}

	if strings.HasPrefix(home, "~") {
		userHome, err := os.UserHomeDir()
		if err == nil {
			home = filepath.Join(userHome, strings.TrimPrefix(home, "~"))
		}
	}
	if !filepath.IsAbs(home) {
		if abs, err := filepath.Abs(home); err == nil {
			home = abs
		}
	}

	return filepath.Join(home, "skills"), nil
}

func linkDir(dst, src string) error {
	// Prefer native symlink.
	if err := os.Symlink(src, dst); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	// Windows fallback: junction usually works without elevated privileges.
	// mklink /J <link> <target>
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
