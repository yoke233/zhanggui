package appcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const (
	DefaultServerPort  = 8080
	defaultFrontendDir = "/opt/ai-workflow/web/dist"
	repoFrontendDir    = "web/dist"
	frontendDirEnvVar  = "AI_WORKFLOW_FRONTEND_DIR"
)

func ResolveFrontendFS() (fs.FS, error) {
	rawDir, hasOverride := os.LookupEnv(frontendDirEnvVar)
	if hasOverride {
		return resolveFrontendDirFS(strings.TrimSpace(rawDir))
	}
	for _, candidate := range []string{defaultFrontendDir, repoFrontendDir} {
		frontendFS, err := resolveFrontendDirFS(candidate)
		if err == nil && frontendFS != nil {
			return frontendFS, nil
		}
	}
	return nil, nil
}

func resolveFrontendDirFS(frontendDir string) (fs.FS, error) {
	if frontendDir == "" {
		return nil, nil
	}
	info, err := os.Stat(frontendDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve frontend dir %q: %w", frontendDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("resolve frontend dir %q: not a directory", frontendDir)
	}
	return os.DirFS(frontendDir), nil
}
