package github

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"strings"
	"testing"

	"github.com/user/ai-workflow/internal/config"
)

func TestNewGitHubClient_PAT_Success(t *testing.T) {
	cfg := config.GitHubConfig{
		Enabled: true,
		Token:   "ghp_test_token",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be initialized")
	}
	if client.Client() == nil {
		t.Fatal("expected underlying github client to be initialized")
	}
}

func TestNewGitHubClient_AppAuth_Success(t *testing.T) {
	keyPath := writeTestPrivateKey(t)
	cfg := config.GitHubConfig{
		Enabled:        true,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPath: keyPath,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be initialized")
	}
	if client.Client() == nil {
		t.Fatal("expected underlying github client to be initialized")
	}
}

func TestNewGitHubClient_MissingCredentials_Error(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.GitHubConfig
	}{
		{
			name: "disabled config",
			cfg: config.GitHubConfig{
				Enabled: false,
			},
		},
		{
			name: "enabled without token or app credentials",
			cfg: config.GitHubConfig{
				Enabled: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.cfg)
			if err == nil {
				t.Fatal("expected NewClient to return error")
			}
			if !strings.Contains(err.Error(), "credentials") &&
				!strings.Contains(err.Error(), "enabled") &&
				!strings.Contains(err.Error(), "disabled") {
				t.Fatalf("expected error mentioning credentials or enabled state, got %v", err)
			}
		})
	}
}

func writeTestPrivateKey(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}

	encoded := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	path := t.TempDir() + "/app-private-key.pem"
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	return path
}
