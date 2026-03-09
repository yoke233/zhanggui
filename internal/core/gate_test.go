package core

import (
	"strings"
	"testing"
)

func TestNewGateCheckID(t *testing.T) {
	id := NewGateCheckID()
	if !strings.HasPrefix(id, "gc-") {
		t.Errorf("expected prefix 'gc-', got %q", id)
	}
}

func TestGateValidate(t *testing.T) {
	tests := []struct {
		name    string
		gate    Gate
		wantErr bool
	}{
		{"valid auto", Gate{Name: "test", Type: GateTypeAuto}, false},
		{"valid with all fields", Gate{Name: "review", Type: GateTypeOwnerReview, Rules: "check quality", MaxAttempts: 3, Fallback: GateFallbackEscalate}, false},
		{"missing name", Gate{Type: GateTypeAuto}, true},
		{"invalid type", Gate{Name: "test", Type: "bad"}, true},
		{"invalid fallback", Gate{Name: "test", Type: GateTypeAuto, Fallback: "bad"}, true},
		{"negative max_attempts", Gate{Name: "test", Type: GateTypeAuto, MaxAttempts: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.gate.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateGates(t *testing.T) {
	t.Run("valid chain", func(t *testing.T) {
		gates := []Gate{
			{Name: "lint", Type: GateTypeAuto},
			{Name: "review", Type: GateTypeOwnerReview},
		}
		if err := ValidateGates(gates); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("duplicate names", func(t *testing.T) {
		gates := []Gate{
			{Name: "lint", Type: GateTypeAuto},
			{Name: "lint", Type: GateTypeOwnerReview},
		}
		err := ValidateGates(gates)
		if err == nil {
			t.Error("expected error for duplicate names")
		}
	})

	t.Run("empty chain is valid", func(t *testing.T) {
		if err := ValidateGates(nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
