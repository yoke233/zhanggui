package core

import (
	"strings"
	"testing"
)

func TestResolveGates(t *testing.T) {
	t.Run("default normal", func(t *testing.T) {
		p := WorkflowProfile{Type: WorkflowProfileNormal, SLAMinutes: 10}
		gates := p.ResolveGates()
		if len(gates) != 1 || gates[0].Name != "demand_review" {
			t.Errorf("unexpected gates: %+v", gates)
		}
	})
	t.Run("default strict", func(t *testing.T) {
		p := WorkflowProfile{Type: WorkflowProfileStrict, SLAMinutes: 10}
		gates := p.ResolveGates()
		if len(gates) != 2 {
			t.Errorf("expected 2 gates, got %d", len(gates))
		}
	})
	t.Run("custom gates override", func(t *testing.T) {
		custom := []Gate{{Name: "my_gate", Type: GateTypeAuto}}
		p := WorkflowProfile{Type: WorkflowProfileNormal, SLAMinutes: 10, Gates: custom}
		gates := p.ResolveGates()
		if len(gates) != 1 || gates[0].Name != "my_gate" {
			t.Errorf("expected custom gate, got: %+v", gates)
		}
	})
}

func TestWorkflowProfileTypeValidate(t *testing.T) {
	cases := []struct {
		name    string
		profile WorkflowProfileType
		wantErr bool
	}{
		{name: "normal", profile: WorkflowProfileNormal, wantErr: false},
		{name: "strict", profile: WorkflowProfileStrict, wantErr: false},
		{name: "fast_release", profile: WorkflowProfileFastRelease, wantErr: false},
		{name: "empty", profile: "", wantErr: true},
		{name: "unknown", profile: "urgent", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.profile.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected validation error for profile %q", tc.profile)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected validation success for profile %q, got: %v", tc.profile, err)
			}
		})
	}
}

func TestWorkflowProfileValidate(t *testing.T) {
	cases := []struct {
		name      string
		profile   WorkflowProfile
		wantErr   bool
		errSubstr string
	}{
		{
			name: "normal with valid sla",
			profile: WorkflowProfile{
				Type:       WorkflowProfileNormal,
				SLAMinutes: 60,
			},
			wantErr: false,
		},
		{
			name: "strict with minimum sla",
			profile: WorkflowProfile{
				Type:       WorkflowProfileStrict,
				SLAMinutes: 1,
			},
			wantErr: false,
		},
		{
			name: "invalid profile type",
			profile: WorkflowProfile{
				Type:       "rush",
				SLAMinutes: 60,
			},
			wantErr:   true,
			errSubstr: "invalid workflow profile type",
		},
		{
			name: "sla is zero",
			profile: WorkflowProfile{
				Type:       WorkflowProfileNormal,
				SLAMinutes: 0,
			},
			wantErr:   true,
			errSubstr: "sla_minutes",
		},
		{
			name: "sla is negative",
			profile: WorkflowProfile{
				Type:       WorkflowProfileStrict,
				SLAMinutes: -1,
			},
			wantErr:   true,
			errSubstr: "sla_minutes",
		},
		{
			name: "sla exceeds max limit",
			profile: WorkflowProfile{
				Type:       WorkflowProfileFastRelease,
				SLAMinutes: 61,
			},
			wantErr:   true,
			errSubstr: "sla_minutes",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.profile.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected validation success, got: %v", err)
			}
			if tc.wantErr && tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
				t.Fatalf("expected error to contain %q, got: %v", tc.errSubstr, err)
			}
		})
	}
}
