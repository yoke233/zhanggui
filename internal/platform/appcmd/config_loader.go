package appcmd

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/yoke233/ai-workflow/internal/platform/appdata"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func LoadConfig() (*config.Config, string, *config.Secrets, error) {
	dataDir, err := appdata.ResolveDataDir()
	if err != nil {
		return nil, "", nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, "", nil, err
	}
	cfgPath := filepath.Join(dataDir, "config.toml")
	secretsPath := resolveSecretsFilePath(dataDir)
	secrets, err := config.LoadSecrets(secretsPath)
	if err != nil {
		return nil, "", nil, err
	}
	if config.EnsureSecrets(secrets) {
		if err := config.SaveSecrets(secretsPath, secrets); err != nil {
			return nil, "", nil, err
		}
	}
	if _, statErr := os.Stat(cfgPath); errors.Is(statErr, os.ErrNotExist) {
		if err := os.WriteFile(cfgPath, config.DefaultsTOML(), 0o644); err != nil {
			return nil, "", nil, err
		}
	} else if statErr != nil {
		return nil, "", nil, statErr
	}
	cfg, err := config.LoadGlobal(cfgPath, secretsPath)
	if err != nil {
		return nil, "", nil, err
	}
	if config.EnsureSecrets(secrets) {
		config.ApplySecrets(cfg, secrets)
		_ = config.SaveSecrets(secretsPath, secrets)
	}
	if err := config.Validate(cfg); err != nil {
		return nil, "", nil, err
	}
	return cfg, dataDir, secrets, nil
}
