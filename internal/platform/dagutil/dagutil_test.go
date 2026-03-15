package dagutil

import "testing"

func TestAllDepsResolved(t *testing.T) {
	tests := []struct {
		name    string
		deps    []int64
		doneSet map[int64]bool
		want    bool
	}{
		{"empty deps", nil, nil, true},
		{"all done", []int64{1, 2}, map[int64]bool{1: true, 2: true, 3: true}, true},
		{"partial done", []int64{1, 2}, map[int64]bool{1: true}, false},
		{"none done", []int64{1}, map[int64]bool{}, false},
		{"single done", []int64{5}, map[int64]bool{5: true}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AllDepsResolved(tc.deps, tc.doneSet); got != tc.want {
				t.Fatalf("AllDepsResolved = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAllDepsResolved_StringKeys(t *testing.T) {
	done := map[string]bool{"a": true, "b": true}
	if !AllDepsResolved([]string{"a", "b"}, done) {
		t.Fatal("expected true for string keys")
	}
	if AllDepsResolved([]string{"a", "c"}, done) {
		t.Fatal("expected false when 'c' is missing")
	}
}

func TestAllMatch(t *testing.T) {
	tests := []struct {
		name  string
		items []int
		pred  func(int) bool
		want  bool
	}{
		{"empty", nil, func(int) bool { return false }, true},
		{"all match", []int{2, 4, 6}, func(x int) bool { return x%2 == 0 }, true},
		{"partial match", []int{2, 3, 6}, func(x int) bool { return x%2 == 0 }, false},
		{"single match", []int{4}, func(x int) bool { return x%2 == 0 }, true},
		{"single no match", []int{3}, func(x int) bool { return x%2 == 0 }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AllMatch(tc.items, tc.pred); got != tc.want {
				t.Fatalf("AllMatch = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnyMatch(t *testing.T) {
	tests := []struct {
		name  string
		items []int
		pred  func(int) bool
		want  bool
	}{
		{"empty", nil, func(int) bool { return true }, false},
		{"one match", []int{1, 2, 3}, func(x int) bool { return x == 2 }, true},
		{"no match", []int{1, 3, 5}, func(x int) bool { return x%2 == 0 }, false},
		{"all match", []int{2, 4}, func(x int) bool { return x%2 == 0 }, true},
		{"single match", []int{7}, func(x int) bool { return x == 7 }, true},
		{"single no match", []int{7}, func(x int) bool { return x == 8 }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AnyMatch(tc.items, tc.pred); got != tc.want {
				t.Fatalf("AnyMatch = %v, want %v", got, tc.want)
			}
		})
	}
}
