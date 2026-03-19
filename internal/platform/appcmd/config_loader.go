package appcmd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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
	cfgPath := resolveGlobalConfigFilePath(dataDir)
	secretsPath := resolveSecretsFilePath(dataDir)
	secrets, err := config.LoadSecrets(secretsPath)
	if err != nil {
		return nil, "", nil, err
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
	secrets, err = ensureAdminToken(secretsPath, cfg, secrets)
	if err != nil {
		return nil, "", nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return nil, "", nil, err
	}
	return cfg, dataDir, secrets, nil
}

func ensureAdminToken(secretsPath string, cfg *config.Config, secrets *config.Secrets) (*config.Secrets, error) {
	if cfg == nil || !cfg.Server.IsAuthRequired() {
		return secrets, nil
	}
	if secrets == nil {
		secrets = &config.Secrets{}
	}
	if secrets.Tokens == nil {
		secrets.Tokens = map[string]config.TokenEntry{}
	}
	if entry, ok := secrets.Tokens["admin"]; ok && entry.Token != "" {
		return secrets, nil
	}
	if filepath.Ext(secretsPath) != ".toml" {
		return secrets, nil
	}

	token, err := generateBootstrapToken()
	if err != nil {
		return nil, fmt.Errorf("generate bootstrap admin token: %w", err)
	}
	secrets.Tokens["admin"] = config.TokenEntry{
		Token:     token,
		Scopes:    []string{"*"},
		Submitter: "system.bootstrap",
	}
	if err := config.SaveSecrets(secretsPath, secrets); err != nil {
		return nil, fmt.Errorf("save bootstrap secrets: %w", err)
	}
	return secrets, nil
}

func generateBootstrapToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
