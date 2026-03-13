package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// Secrets holds authentication credentials loaded from secrets.toml.
// Kept separate from config.toml for security isolation.
type Secrets struct {
	Tokens map[string]TokenEntry `toml:"tokens" yaml:"tokens"`
	GitHub GitHubSecrets         `toml:"github" yaml:"github"`
	Codeup CodeupSecrets         `toml:"codeup" yaml:"codeup"`
}

// TokenEntry defines a named token with scoped permissions.
//
// Scopes use "resource:action" format:
//
//	"*"              — full access (wildcard)
//	"issues:read"    — read issues
//	"issues:write"   — create/update issues
//	"runs:read"      — read runs and events
//	"runs:write"     — create/cancel runs
//	"projects:read"  — list/get projects
//	"projects:write" — create/update projects
//	"chat:read"      — read chat sessions
//	"chat:write"     — send chat messages
//	"admin"          — admin operations (restart, force-ready, audit)
//	"mcp"            — MCP endpoint access
type TokenEntry struct {
	Token     string   `toml:"token"               yaml:"token"`
	Scopes    []string `toml:"scopes"              yaml:"scopes"`
	Submitter string   `toml:"submitter,omitempty"  yaml:"submitter,omitempty"` // identity for audit
	Projects  []string `toml:"projects,omitempty"   yaml:"projects,omitempty"`  // project whitelist (empty = all)
}

// GitHubSecrets holds GitHub-related credentials.
type GitHubSecrets struct {
	Token          string `toml:"token"            yaml:"token"`
	PAT            string `toml:"pat"              yaml:"pat"`
	PrivateKeyPath string `toml:"private_key_path" yaml:"private_key_path"`
	WebhookSecret  string `toml:"webhook_secret"   yaml:"webhook_secret"`
}

// CodeupSecrets holds Codeup-related credentials for SCM automation.
type CodeupSecrets struct {
	Token string `toml:"token" yaml:"token"`
	PAT   string `toml:"pat"   yaml:"pat"`
}

// AdminToken returns the token value for the "admin" role entry, or empty if none.
func (s *Secrets) AdminToken() string {
	if s == nil {
		return ""
	}
	if entry, ok := s.Tokens["admin"]; ok {
		return entry.Token
	}
	return ""
}

// LoadSecrets reads secrets from a TOML or YAML file (detected by extension).
// Returns empty Secrets (not error) if the file does not exist.
func LoadSecrets(path string) (*Secrets, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Secrets{Tokens: map[string]TokenEntry{}}, nil
		}
		return nil, fmt.Errorf("read secrets: %w", err)
	}

	s := &Secrets{}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		dec := toml.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(s); err != nil {
			return nil, fmt.Errorf("parse secrets toml: %w", err)
		}
	default:
		decoder := yaml.NewDecoder(strings.NewReader(string(data)))
		decoder.KnownFields(true)
		if err := decoder.Decode(s); err != nil {
			return nil, fmt.Errorf("parse secrets yaml: %w", err)
		}
	}

	if s.Tokens == nil {
		s.Tokens = map[string]TokenEntry{}
	}
	return s, nil
}

// SaveSecrets writes secrets to a TOML file with restricted permissions (0600).
func SaveSecrets(path string, s *Secrets) error {
	data, err := toml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// ApplySecrets merges loaded secrets into a Config (GitHub credentials only).
// Token-based auth is handled by TokenRegistry, not Config fields.
func ApplySecrets(cfg *Config, s *Secrets) {
	if cfg == nil || s == nil {
		return
	}
	if s.GitHub.Token != "" {
		cfg.GitHub.Token = s.GitHub.Token
	}
	if s.GitHub.PrivateKeyPath != "" {
		cfg.GitHub.PrivateKeyPath = s.GitHub.PrivateKeyPath
	}
	if s.GitHub.WebhookSecret != "" {
		cfg.GitHub.WebhookSecret = s.GitHub.WebhookSecret
	}
}
