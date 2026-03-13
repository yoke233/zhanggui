package provider

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Registry holds all registered ResourceProvider implementations and dispatches
// Fetch/Deposit calls to the appropriate provider based on the locator's Kind.
type Registry struct {
	providers map[core.ResourceLocatorKind]core.ResourceProvider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[core.ResourceLocatorKind]core.ResourceProvider),
	}
}

// NewDefaultRegistry creates a registry pre-loaded with built-in providers.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&LocalFSProvider{})
	r.Register(&HTTPProvider{})
	return r
}

// Register adds a provider to the registry.
func (r *Registry) Register(p core.ResourceProvider) {
	r.providers[p.Kind()] = p
}

// Fetch dispatches to the correct provider for the given locator.
func (r *Registry) Fetch(ctx context.Context, locator *core.ResourceLocator, path string, destDir string) (string, error) {
	p, ok := r.providers[locator.Kind]
	if !ok {
		return "", fmt.Errorf("no resource provider registered for kind %q", locator.Kind)
	}
	return p.Fetch(ctx, locator, path, destDir)
}

// Deposit dispatches to the correct provider for the given locator.
func (r *Registry) Deposit(ctx context.Context, locator *core.ResourceLocator, path string, localPath string) error {
	p, ok := r.providers[locator.Kind]
	if !ok {
		return fmt.Errorf("no resource provider registered for kind %q", locator.Kind)
	}
	return p.Deposit(ctx, locator, path, localPath)
}
