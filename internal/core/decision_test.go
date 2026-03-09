package core

import (
	"strings"
	"testing"
)

func TestNewDecisionID(t *testing.T) {
	id := NewDecisionID()
	if !strings.HasPrefix(id, "dec-") {
		t.Errorf("expected prefix 'dec-', got %q", id)
	}
	if len(id) != 28 {
		t.Errorf("expected length 28, got %d for %q", len(id), id)
	}
}

func TestPromptHash(t *testing.T) {
	hash := PromptHash("hello world")
	if len(hash) != 16 {
		t.Errorf("expected 16 hex chars, got %d: %q", len(hash), hash)
	}
	if PromptHash("hello world") != hash {
		t.Error("PromptHash should be deterministic")
	}
	if PromptHash("goodbye world") == hash {
		t.Error("different inputs should produce different hashes")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel"},
		{"", 5, ""},
		{"你好世界", 2, "你好"},
	}
	for _, tt := range tests {
		got := TruncateString(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("TruncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}
