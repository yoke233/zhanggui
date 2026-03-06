package mcpserver

import (
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func newTestStore(t *testing.T) core.Store {
	t.Helper()
	s, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func addProject(t *testing.T, s core.Store, id, name string) {
	t.Helper()
	if err := s.CreateProject(&core.Project{ID: id, Name: name, RepoPath: "/tmp/" + id}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveProjectID_IDTakesPriority(t *testing.T) {
	s := newTestStore(t)
	addProject(t, s, "p1", "Alpha")

	got, err := resolveProjectID(s, "p1", "Beta")
	if err != nil {
		t.Fatal(err)
	}
	if got != "p1" {
		t.Errorf("expected p1, got %s", got)
	}
}

func TestResolveProjectID_ExactNameMatch(t *testing.T) {
	s := newTestStore(t)
	addProject(t, s, "p1", "Alpha")
	addProject(t, s, "p2", "AlphaBeta")

	got, err := resolveProjectID(s, "", "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got != "p1" {
		t.Errorf("expected p1, got %s", got)
	}
}

func TestResolveProjectID_UniqueSubstringMatch(t *testing.T) {
	s := newTestStore(t)
	addProject(t, s, "p1", "Alpha")
	addProject(t, s, "p2", "Beta")

	got, err := resolveProjectID(s, "", "Alph")
	if err != nil {
		t.Fatal(err)
	}
	if got != "p1" {
		t.Errorf("expected p1, got %s", got)
	}
}

func TestResolveProjectID_AmbiguousName(t *testing.T) {
	s := newTestStore(t)
	addProject(t, s, "p1", "AlphaOne")
	addProject(t, s, "p2", "AlphaTwo")

	_, err := resolveProjectID(s, "", "Alpha")
	if err == nil {
		t.Fatal("expected error for ambiguous name")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected ambiguous error, got: %v", err)
	}
}

func TestResolveProjectID_NoMatch(t *testing.T) {
	s := newTestStore(t)
	addProject(t, s, "p1", "Alpha")

	_, err := resolveProjectID(s, "", "Gamma")
	if err == nil {
		t.Fatal("expected error for no match")
	}
	if !strings.Contains(err.Error(), "no project found") {
		t.Errorf("expected no project found error, got: %v", err)
	}
}

func TestResolveProjectID_SingleProjectAutoInfer(t *testing.T) {
	s := newTestStore(t)
	addProject(t, s, "p1", "Alpha")

	got, err := resolveProjectID(s, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "p1" {
		t.Errorf("expected p1, got %s", got)
	}
}

func TestResolveProjectID_MultipleProjectsNoInput(t *testing.T) {
	s := newTestStore(t)
	addProject(t, s, "p1", "Alpha")
	addProject(t, s, "p2", "Beta")

	_, err := resolveProjectID(s, "", "")
	if err == nil {
		t.Fatal("expected error for multiple projects")
	}
	if !strings.Contains(err.Error(), "multiple projects") {
		t.Errorf("expected multiple projects error, got: %v", err)
	}
}

func TestResolveProjectID_DuplicateExactName(t *testing.T) {
	s := newTestStore(t)
	addProject(t, s, "p1", "Alpha")
	addProject(t, s, "p2", "Alpha")

	_, err := resolveProjectID(s, "", "Alpha")
	if err == nil {
		t.Fatal("expected error for duplicate exact name")
	}
	if !strings.Contains(err.Error(), "multiple projects named") {
		t.Errorf("expected duplicate name error, got: %v", err)
	}
}

func TestResolveProjectID_EmptyStore(t *testing.T) {
	s := newTestStore(t)

	_, err := resolveProjectID(s, "", "")
	if err == nil {
		t.Fatal("expected error for empty store")
	}
	if !strings.Contains(err.Error(), "no projects exist") {
		t.Errorf("expected no projects error, got: %v", err)
	}
}
