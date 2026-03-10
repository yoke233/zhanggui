package contextsqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
	_ "modernc.org/sqlite"
)

const schemaDDL = `
CREATE TABLE IF NOT EXISTS context_entries (
    uri        TEXT PRIMARY KEY,
    content    BLOB NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS context_links (
    from_uri TEXT NOT NULL,
    to_uri   TEXT NOT NULL,
    reason   TEXT,
    PRIMARY KEY (from_uri, to_uri)
);
CREATE TABLE IF NOT EXISTS context_sessions (
    id         TEXT PRIMARY KEY,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS context_messages (
    session_id TEXT NOT NULL REFERENCES context_sessions(id),
    seq        INTEGER NOT NULL,
    role       TEXT NOT NULL,
    parts_json TEXT NOT NULL,
    PRIMARY KEY (session_id, seq)
);
`

// Store is a SQLite-backed implementation of core.ContextStore.
type Store struct {
	db *sql.DB
}

var _ core.ContextStore = (*Store)(nil)

func New(path string) (*Store, error) {
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("context-sqlite: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(schemaDDL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("context-sqlite: init schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Name() string               { return "context-sqlite" }
func (s *Store) Init(context.Context) error { return nil }
func (s *Store) Close() error               { return s.db.Close() }

func (s *Store) Read(ctx context.Context, uri string) ([]byte, error) {
	var content []byte
	err := s.db.QueryRowContext(ctx, `SELECT content FROM context_entries WHERE uri=?`, uri).Scan(&content)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("context-sqlite: not found: %s", uri)
	}
	if err != nil {
		return nil, fmt.Errorf("context-sqlite: read: %w", err)
	}
	return content, nil
}

func (s *Store) Write(ctx context.Context, uri string, content []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO context_entries (uri, content, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		uri, content)
	if err != nil {
		return fmt.Errorf("context-sqlite: write: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context, uri string) ([]core.ContextEntry, error) {
	prefix := uri
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	rows, err := s.db.QueryContext(ctx, `SELECT uri FROM context_entries WHERE uri LIKE ?`, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("context-sqlite: list: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var entries []core.ContextEntry
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("context-sqlite: list scan: %w", err)
		}
		rest := strings.TrimPrefix(k, prefix)
		if rest == "" {
			continue
		}
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

func (s *Store) Remove(ctx context.Context, uri string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM context_entries WHERE uri=?`, uri)
	if err != nil {
		return fmt.Errorf("context-sqlite: remove: %w", err)
	}
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

func (s *Store) AddResource(ctx context.Context, path string, opts core.AddResourceOpts) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("context-sqlite: read source file: %w", err)
	}
	target := opts.TargetURI
	if strings.HasSuffix(target, "/") {
		target += filepath.Base(path)
	}
	return s.Write(ctx, target, data)
}

func (s *Store) Link(ctx context.Context, from string, to []string, reason string) error {
	for _, t := range to {
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO context_links (from_uri, to_uri, reason) VALUES (?, ?, ?)`,
			from, t, reason); err != nil {
			return fmt.Errorf("context-sqlite: link: %w", err)
		}
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, id string) (core.ContextSession, error) {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO context_sessions (id) VALUES (?)`, id); err != nil {
		return nil, fmt.Errorf("context-sqlite: create session: %w", err)
	}
	return &sqliteSession{db: s.db, id: id}, nil
}

func (s *Store) GetSession(ctx context.Context, id string) (core.ContextSession, error) {
	var exists string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM context_sessions WHERE id=?`, id).Scan(&exists)
	if err == sql.ErrNoRows {
		// Auto-create if not found.
		return s.CreateSession(ctx, id)
	}
	if err != nil {
		return nil, fmt.Errorf("context-sqlite: get session: %w", err)
	}
	return &sqliteSession{db: s.db, id: id}, nil
}

func (s *Store) Materialize(ctx context.Context, uri, targetDir string) error {
	prefix := uri
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	rows, err := s.db.QueryContext(ctx, `SELECT uri, content FROM context_entries WHERE uri LIKE ?`, prefix+"%")
	if err != nil {
		return fmt.Errorf("context-sqlite: materialize query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var k string
		var v []byte
		if err := rows.Scan(&k, &v); err != nil {
			return fmt.Errorf("context-sqlite: materialize scan: %w", err)
		}
		rel := strings.TrimPrefix(k, prefix)
		dst := filepath.Join(targetDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("context-sqlite: mkdir: %w", err)
		}
		if err := os.WriteFile(dst, v, 0o644); err != nil {
			return fmt.Errorf("context-sqlite: write file: %w", err)
		}
	}
	return nil
}

// Module returns a PluginModule for factory registration.
func Module() core.PluginModule {
	return core.PluginModule{
		Name: "context-sqlite",
		Slot: core.SlotContext,
		Factory: func(cfg map[string]any) (core.Plugin, error) {
			path := ".ai-workflow/context.db"
			if cfg != nil {
				if p, ok := cfg["path"].(string); ok && p != "" {
					path = p
				}
			}
			return New(path)
		},
	}
}

// sqliteSession implements core.ContextSession backed by SQLite.
type sqliteSession struct {
	db *sql.DB
	id string
}

func (s *sqliteSession) ID() string { return s.id }

func (s *sqliteSession) AddMessage(role string, parts []core.MessagePart) error {
	data, err := json.Marshal(parts)
	if err != nil {
		return fmt.Errorf("context-sqlite: marshal parts: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO context_messages (session_id, seq, role, parts_json) VALUES (?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM context_messages WHERE session_id = ?), ?, ?)`,
		s.id, s.id, role, string(data))
	if err != nil {
		return fmt.Errorf("context-sqlite: add message: %w", err)
	}
	return nil
}

func (s *sqliteSession) Used([]string) error { return nil }

func (s *sqliteSession) Commit() (core.CommitResult, error) {
	return core.CommitResult{Status: "committed"}, nil
}
