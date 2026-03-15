package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// HTTPProvider fetches resources from HTTP/HTTPS URLs.
// Deposit is not supported for plain HTTP (read-only).
type HTTPProvider struct {
	Client *http.Client
}

func (p *HTTPProvider) Kind() string {
	return core.ResourceKindHTTP
}

func (p *HTTPProvider) httpClient() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (p *HTTPProvider) Fetch(ctx context.Context, space *core.ResourceSpace, resourcePath string, destDir string) (string, error) {
	base := strings.TrimRight(space.RootURI, "/")
	url := base + "/" + strings.TrimLeft(resourcePath, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("http fetch: build request: %w", err)
	}

	// Apply auth headers from config if present.
	if token, ok := space.Config["auth_token"].(string); ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if headers, ok := space.Config["headers"].(map[string]any); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("http fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http fetch %s: status %d", url, resp.StatusCode)
	}

	filename := filepath.Base(path.Clean(resourcePath))
	if filename == "." || filename == "/" {
		filename = "download"
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("http fetch: mkdir %s: %w", destDir, err)
	}
	destPath := filepath.Join(destDir, filename)

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("http fetch: create %s: %w", destPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("http fetch: write %s: %w", destPath, err)
	}
	return destPath, out.Close()
}

func (p *HTTPProvider) Deposit(_ context.Context, _ *core.ResourceSpace, _ string, _ string) error {
	return fmt.Errorf("http provider does not support deposit (read-only)")
}
