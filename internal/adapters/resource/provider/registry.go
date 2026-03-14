package provider

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Registry holds all registered ResourceProvider implementations and dispatches
// Fetch/Deposit calls to the appropriate provider based on the binding's Kind.
type Registry struct {
	providers map[string]core.ResourceProvider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]core.ResourceProvider),
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

// Fetch dispatches to the correct provider for the given binding.
func (r *Registry) Fetch(ctx context.Context, binding *core.ResourceBinding, path string, destDir string) (string, error) {
	p, ok := r.providers[binding.Kind]
	if !ok {
		return "", fmt.Errorf("no resource provider registered for kind %q", binding.Kind)
	}
	return p.Fetch(ctx, binding, path, destDir)
}

// Deposit dispatches to the correct provider for the given binding.
func (r *Registry) Deposit(ctx context.Context, binding *core.ResourceBinding, path string, localPath string) error {
	p, ok := r.providers[binding.Kind]
	if !ok {
		return fmt.Errorf("no resource provider registered for kind %q", binding.Kind)
	}
	return p.Deposit(ctx, binding, path, localPath)
}
