package mcpserver

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"time"
)

// ssrfCheckEnabled controls whether SSRF protection (private IP blocking) is active.
// Tests may set this to false to allow httptest servers on 127.0.0.1.
var ssrfCheckEnabled = true

// ssrfSafeClient returns an http.Client that validates every redirect target
// against private IP ranges, preventing redirect-based SSRF.
func ssrfSafeClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if ssrfCheckEnabled {
				host := req.URL.Hostname()
				if isPrivateIP(host) {
					return fmt.Errorf("redirect to private/internal address %q is not allowed", host)
				}
			}
			return nil
		},
	}
}

// fetchURLContent downloads content from a URL with safety checks.
func fetchURLContent(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", fmt.Errorf("unsupported scheme %q (only http/https allowed)", u.Scheme)
	}

	if ssrfCheckEnabled {
		host := u.Hostname()
		if isPrivateIP(host) {
			return nil, "", fmt.Errorf("private/internal addresses are not allowed")
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	resp, err := ssrfSafeClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, "", fmt.Errorf("response exceeds %d bytes limit", maxBytes)
	}

	mediaType := ""
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mt, _, _ := mime.ParseMediaType(ct)
		mediaType = mt
	}

	return body, mediaType, nil
}

// isPrivateIP checks if a hostname resolves to a private/loopback address.
func isPrivateIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		// Try to resolve hostname.
		addrs, err := net.LookupHost(host)
		if err != nil || len(addrs) == 0 {
			return false
		}
		ip = net.ParseIP(addrs[0])
		if ip == nil {
			return false
		}
	}

	privateRanges := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"::1/128",
		"fc00::/7",
	}
	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
