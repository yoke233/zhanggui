package provider

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Registry holds all registered SpaceProvider implementations and dispatches
// Fetch/Deposit calls to the appropriate provider based on the space's Kind.
type Registry struct {
	providers map[string]core.SpaceProvider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]core.SpaceProvider),
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
func (r *Registry) Register(p core.SpaceProvider) {
	r.providers[p.Kind()] = p
}

// Fetch dispatches to the correct provider for the given space.
func (r *Registry) Fetch(ctx context.Context, space *core.ResourceSpace, path string, destDir string) (string, error) {
	p, ok := r.providers[space.Kind]
	if !ok {
		return "", fmt.Errorf("no resource provider registered for kind %q", space.Kind)
	}
	return p.Fetch(ctx, space, path, destDir)
}

// Deposit dispatches to the correct provider for the given space.
func (r *Registry) Deposit(ctx context.Context, space *core.ResourceSpace, path string, localPath string) error {
	p, ok := r.providers[space.Kind]
	if !ok {
		return fmt.Errorf("no resource provider registered for kind %q", space.Kind)
	}
	return p.Deposit(ctx, space, path, localPath)
}
