package configruntime

import (
	"runtime"
	"strings"

	"github.com/yoke233/zhanggui/internal/platform/config"
)

const windowsCodexACPVersion = "0.9.5"

func normalizeDriverConfigForPlatform(driver config.RuntimeDriverConfig) config.RuntimeDriverConfig {
	if !isCodexACPDriver(driver) {
		return driver
	}

	cloned := driver
	cloned.LaunchArgs = append([]string(nil), driver.LaunchArgs...)
	sandboxMode := currentCodexACPConfigOverride(cloned.LaunchArgs, "sandbox_mode")
	if sandboxMode == "" {
		sandboxMode = `"workspace-write"`
	}
	if runtime.GOOS == "windows" {
		for i, arg := range cloned.LaunchArgs {
			trimmed := strings.TrimSpace(arg)
			if trimmed == "@zed-industries/codex-acp" {
				cloned.LaunchArgs[i] = "@zed-industries/codex-acp@" + windowsCodexACPVersion
			}
		}
		sandboxMode = `"danger-full-access"`
	}
	cloned.LaunchArgs = ensureCodexACPConfigOverride(cloned.LaunchArgs, "approval_policy", `"never"`)
	cloned.LaunchArgs = setCodexACPConfigOverride(cloned.LaunchArgs, "sandbox_mode", sandboxMode)
	if sandboxMode == `"workspace-write"` {
		cloned.LaunchArgs = setCodexACPConfigOverride(cloned.LaunchArgs, "sandbox_workspace_write.network_access", "true")
	} else {
		cloned.LaunchArgs = removeCodexACPConfigOverride(cloned.LaunchArgs, "sandbox_workspace_write.network_access")
	}
	return cloned
}

func isCodexACPDriver(driver config.RuntimeDriverConfig) bool {
	if strings.EqualFold(strings.TrimSpace(driver.ID), "codex-acp") {
		return true
	}
	for _, arg := range driver.LaunchArgs {
		trimmed := strings.TrimSpace(arg)
		if strings.Contains(trimmed, "@zed-industries/codex-acp") {
			return true
		}
	}
	return false
}

func ensureCodexACPConfigOverride(args []string, key string, value string) []string {
	if len(args) == 0 {
		return []string{"-c", key + "=" + value}
	}
	for i := 0; i < len(args)-1; i++ {
		if strings.TrimSpace(args[i]) != "-c" {
			continue
		}
		override := strings.TrimSpace(args[i+1])
		if strings.HasPrefix(override, key+"=") {
			return args
		}
	}
	return append(args, "-c", key+"="+value)
}

func setCodexACPConfigOverride(args []string, key string, value string) []string {
	if len(args) == 0 {
		return []string{"-c", key + "=" + value}
	}
	for i := 0; i < len(args)-1; i++ {
		if strings.TrimSpace(args[i]) != "-c" {
			continue
		}
		override := strings.TrimSpace(args[i+1])
		if strings.HasPrefix(override, key+"=") {
			args[i+1] = key + "=" + value
			return args
		}
	}
	return append(args, "-c", key+"="+value)
}

func removeCodexACPConfigOverride(args []string, key string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if strings.TrimSpace(args[i]) == "-c" && i+1 < len(args) {
			override := strings.TrimSpace(args[i+1])
			if strings.HasPrefix(override, key+"=") {
				i++
				continue
			}
		}
		out = append(out, args[i])
	}
	return out
}

func currentCodexACPConfigOverride(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if strings.TrimSpace(args[i]) != "-c" {
			continue
		}
		override := strings.TrimSpace(args[i+1])
		if strings.HasPrefix(override, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(override, key+"="))
		}
	}
	return ""
}
