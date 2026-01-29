package execution

import "testing"

func TestRegistry_Get_Unknown(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("nope"); err == nil {
		t.Fatalf("expected error for unknown workflow")
	}
}
