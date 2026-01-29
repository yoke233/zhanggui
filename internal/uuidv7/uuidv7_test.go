package uuidv7

import (
	"regexp"
	"testing"
	"time"
)

var uuidV7Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNew_WellFormedUUIDv7(t *testing.T) {
	id := New()
	if !uuidV7Re.MatchString(id) {
		t.Fatalf("not uuidv7: %q", id)
	}
}

func TestNewAt_SortsByTime(t *testing.T) {
	t1 := time.UnixMilli(1700000000000) // fixed ms
	t2 := t1.Add(1 * time.Millisecond)

	id1 := NewAt(t1)
	id2 := NewAt(t2)

	if !(id1 < id2) {
		t.Fatalf("expected id1 < id2, got id1=%q id2=%q", id1, id2)
	}
}
