package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	ghapi "github.com/google/go-github/v68/github"
	"github.com/user/ai-workflow/internal/config"
	"golang.org/x/oauth2"
)

// Client wraps authenticated GitHub SDK client and the underlying http client.
type Client struct {
	client     *ghapi.Client
	httpClient *http.Client
}

// NewClient builds a GitHub API client from application config.
func NewClient(cfg config.GitHubConfig) (*Client, error) {
	if !cfg.Enabled {
		return nil, errors.New("github client is disabled")
	}

	httpClient, err := buildHTTPClient(cfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		client:     ghapi.NewClient(httpClient),
		httpClient: httpClient,
	}, nil
}

// Client returns underlying go-github client.
func (c *Client) Client() *ghapi.Client {
	if c == nil {
		return nil
	}
	return c.client
}

// HTTPClient returns authenticated http client used by go-github.
func (c *Client) HTTPClient() *http.Client {
	if c == nil {
		return nil
	}
	return c.httpClient
}

func buildHTTPClient(cfg config.GitHubConfig) (*http.Client, error) {
	if token := strings.TrimSpace(cfg.Token); token != "" {
		src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		return oauth2.NewClient(context.Background(), src), nil
	}

	hasAnyAppCredential := cfg.AppID != 0 || cfg.InstallationID != 0 || strings.TrimSpace(cfg.PrivateKeyPath) != ""
	if hasAnyAppCredential {
		if cfg.AppID == 0 || cfg.InstallationID == 0 || strings.TrimSpace(cfg.PrivateKeyPath) == "" {
			return nil, errors.New("github app credentials are incomplete")
		}
		privateKeyPath := strings.TrimSpace(cfg.PrivateKeyPath)
		if _, err := os.Stat(privateKeyPath); err != nil {
			return nil, fmt.Errorf("github app private key not readable: %w", err)
		}
		transport, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, cfg.AppID, cfg.InstallationID, privateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("create github app transport: %w", err)
		}
		return &http.Client{Transport: transport}, nil
	}

	return nil, errors.New("github credentials are required")
}
