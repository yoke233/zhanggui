package config

import (
	"crypto/rand"
	"encoding/hex"
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

	// Backwards-compatible top-level fine-grained PAT fields.
	// For convenience, they can also be set under [github] and will be backfilled.
	CommitPAT string `toml:"commit_pat" yaml:"commit_pat"`
	MergePAT  string `toml:"merge_pat"  yaml:"merge_pat"`
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
//	"a2a"            — A2A endpoint access
type TokenEntry struct {
	Token     string   `toml:"token"               yaml:"token"`
	Scopes    []string `toml:"scopes"              yaml:"scopes"`
	Submitter string   `toml:"submitter,omitempty"  yaml:"submitter,omitempty"` // identity for A2A/audit
	Projects  []string `toml:"projects,omitempty"   yaml:"projects,omitempty"`  // project whitelist (empty = all)
}

// GitHubSecrets holds GitHub-related credentials.
type GitHubSecrets struct {
	Token          string `toml:"token"            yaml:"token"`
	PrivateKeyPath string `toml:"private_key_path" yaml:"private_key_path"`
	WebhookSecret  string `toml:"webhook_secret"   yaml:"webhook_secret"`

	// Optional fine-grained PATs for automation flows.
	CommitPAT string `toml:"commit_pat" yaml:"commit_pat"`
	MergePAT  string `toml:"merge_pat"  yaml:"merge_pat"`
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
		if err := toml.Unmarshal(data, s); err != nil {
			return nil, fmt.Errorf("parse secrets toml: %w", err)
		}
	default:
		if err := yaml.Unmarshal(data, s); err != nil {
			return nil, fmt.Errorf("parse secrets yaml: %w", err)
		}
	}

	if s.Tokens == nil {
		s.Tokens = map[string]TokenEntry{}
	}

	// Backfill PATs from [github] section if present.
	if strings.TrimSpace(s.CommitPAT) == "" && strings.TrimSpace(s.GitHub.CommitPAT) != "" {
		s.CommitPAT = s.GitHub.CommitPAT
	}
	if strings.TrimSpace(s.MergePAT) == "" && strings.TrimSpace(s.GitHub.MergePAT) != "" {
		s.MergePAT = s.GitHub.MergePAT
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

// EnsureSecrets fills in missing admin token with a random value.
// Returns true if any value was generated (caller should persist).
func EnsureSecrets(s *Secrets) (changed bool) {
	if s.AdminToken() != "" {
		return false
	}
	s.Tokens["admin"] = TokenEntry{
		Token:  mustRandomHex(16),
		Scopes: []string{"*"},
	}
	return true
}

func mustRandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}
