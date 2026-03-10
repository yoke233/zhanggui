package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/ai-workflow/internal/config"
)

func cmdConfigInit(args []string) error {
	force := false
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		switch arg {
		case "":
			continue
		case "--force", "-f":
			force = true
		default:
			return fmt.Errorf("usage: ai-flow config init [--force]")
		}
	}

	dataDir, err := resolveDataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}

	cfgPath := filepath.Join(dataDir, "config.toml")
	if !force {
		if _, err := os.Stat(cfgPath); err == nil {
			return fmt.Errorf("config already exists: %s (use --force to overwrite)", cfgPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	content, err := loadDefaultConfigTemplate()
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		return err
	}
	fmt.Printf("Config initialized: %s\n", cfgPath)

	// Generate secrets.toml with admin token.
	secretsPath := filepath.Join(dataDir, "secrets.toml")
	secrets, _ := config.LoadSecrets(secretsPath)
	if config.EnsureSecrets(secrets) {
		if err := config.SaveSecrets(secretsPath, secrets); err != nil {
			return fmt.Errorf("save secrets: %w", err)
		}
		fmt.Printf("Secrets initialized: %s (admin token: %s)\n", secretsPath, secrets.AdminToken())
	}
	return nil
}

func cmdConfigValidate(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("usage: ai-flow config validate")
	}
	dataDir, err := resolveDataDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(dataDir, "config.toml")
	secretsPath := secretsFilePath(dataDir)
	if _, err := config.LoadGlobal(cfgPath, secretsPath); err != nil {
		return err
	}
	fmt.Printf("Config valid: %s\n", cfgPath)
	return nil
}

func loadDefaultConfigTemplate() ([]byte, error) {
	return config.DefaultsTOML(), nil
}
