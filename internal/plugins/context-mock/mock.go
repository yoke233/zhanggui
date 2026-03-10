package contextmock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Store is an in-memory mock implementation of core.ContextStore.
type Store struct {
	mu    sync.RWMutex
	files map[string][]byte   // uri → content
	links map[string][]string // from-uri → []to-uri
}

func New() *Store {
	return &Store{
		files: make(map[string][]byte),
		links: make(map[string][]string),
	}
}

func (s *Store) Name() string               { return "context-mock" }
func (s *Store) Init(context.Context) error { return nil }
func (s *Store) Close() error               { return nil }

func (s *Store) Read(_ context.Context, uri string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.files[uri]
	if !ok {
		return nil, fmt.Errorf("context-mock: not found: %s", uri)
	}
	return append([]byte(nil), data...), nil
}

func (s *Store) Write(_ context.Context, uri string, content []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[uri] = append([]byte(nil), content...)
	return nil
}

func (s *Store) List(_ context.Context, uri string) ([]core.ContextEntry, error) {
	prefix := uri
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	var entries []core.ContextEntry
	for k := range s.files {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		if rest == "" {
			continue
		}
		// Immediate child: either a file or a directory segment.
		if idx := strings.Index(rest, "/"); idx >= 0 {
			dirName := rest[:idx]
			if !seen[dirName] {
				seen[dirName] = true
				entries = append(entries, core.ContextEntry{
					URI:   prefix + dirName + "/",
					Name:  dirName,
					IsDir: true,
				})
			}
		} else {
			if !seen[rest] {
				seen[rest] = true
				entries = append(entries, core.ContextEntry{
					URI:   k,
					Name:  rest,
					IsDir: false,
				})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func (s *Store) Remove(_ context.Context, uri string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, uri)
	return nil
}

func (s *Store) Abstract(context.Context, string) (string, error) { return "", nil }
func (s *Store) Overview(context.Context, string) (string, error) { return "", nil }

func (s *Store) Find(context.Context, string, core.FindOpts) ([]core.ContextResult, error) {
	return nil, nil
}

func (s *Store) Search(context.Context, string, string, core.SearchOpts) ([]core.ContextResult, error) {
	return nil, nil
}

func (s *Store) AddResource(_ context.Context, path string, opts core.AddResourceOpts) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("context-mock: read source file: %w", err)
	}
	target := opts.TargetURI
	if strings.HasSuffix(target, "/") {
		target += filepath.Base(path)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[target] = data
	return nil
}

func (s *Store) Link(_ context.Context, from string, to []string, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[from] = append(s.links[from], to...)
	return nil
}

func (s *Store) CreateSession(_ context.Context, id string) (core.ContextSession, error) {
	return newMockSession(id), nil
}

func (s *Store) GetSession(_ context.Context, id string) (core.ContextSession, error) {
	return newMockSession(id), nil
}

func (s *Store) Materialize(_ context.Context, uri, targetDir string) error {
	prefix := uri
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for k, v := range s.files {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rel := strings.TrimPrefix(k, prefix)
		dst := filepath.Join(targetDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("context-mock: mkdir: %w", err)
		}
		if err := os.WriteFile(dst, v, 0o644); err != nil {
			return fmt.Errorf("context-mock: write file: %w", err)
		}
	}
	return nil
}

// Module returns a PluginModule for factory registration.
func Module() core.PluginModule {
	return core.PluginModule{
		Name: "context-mock",
		Slot: core.SlotContext,
		Factory: func(map[string]any) (core.Plugin, error) {
			return New(), nil
		},
	}
}

var _ core.ContextStore = (*Store)(nil)

// mockSession implements core.ContextSession in memory.
type mockSession struct {
	id       string
	mu       sync.Mutex
	messages []sessionMessage
	used     []string
}

type sessionMessage struct {
	Role  string
	Parts []core.MessagePart
}

func newMockSession(id string) *mockSession {
	return &mockSession{id: id}
}

func (s *mockSession) ID() string { return s.id }

func (s *mockSession) AddMessage(role string, parts []core.MessagePart) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, sessionMessage{Role: role, Parts: parts})
	return nil
}

func (s *mockSession) Used(contexts []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.used = append(s.used, contexts...)
	return nil
}

func (s *mockSession) Commit() (core.CommitResult, error) {
	return core.CommitResult{Status: "committed"}, nil
}
