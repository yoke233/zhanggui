package core

import "fmt"

// WorkflowProfileType represents built-in orchestration profiles in V2.
type WorkflowProfileType string

const (
	WorkflowProfileNormal      WorkflowProfileType = "normal"
	WorkflowProfileStrict      WorkflowProfileType = "strict"
	WorkflowProfileFastRelease WorkflowProfileType = "fast_release"
)

const (
	MinWorkflowProfileSLAMinutes = 1
	MaxWorkflowProfileSLAMinutes = 60
)

var validWorkflowProfileTypes = map[WorkflowProfileType]struct{}{
	WorkflowProfileNormal:      {},
	WorkflowProfileStrict:      {},
	WorkflowProfileFastRelease: {},
}

// Validate checks whether the profile type is one of supported values.
func (t WorkflowProfileType) Validate() error {
	if _, ok := validWorkflowProfileTypes[t]; !ok {
		return fmt.Errorf("invalid workflow profile type %q", t)
	}
	return nil
}

// WorkflowProfile defines orchestration behavior and SLA timeout policy.
type WorkflowProfile struct {
	Type       WorkflowProfileType `json:"type"`
	SLAMinutes int                 `json:"sla_minutes"`
	Gates      []Gate              `json:"gates,omitempty"`
}

var defaultProfileGates = map[WorkflowProfileType][]Gate{
	WorkflowProfileNormal: {
		{Name: "demand_review", Type: GateTypeAuto, Rules: "需求完整性和可行性检查", MaxAttempts: 2, Fallback: GateFallbackEscalate},
	},
	WorkflowProfileStrict: {
		{Name: "demand_review", Type: GateTypeAuto, Rules: "需求完整性和可行性检查", MaxAttempts: 2, Fallback: GateFallbackEscalate},
		{Name: "peer_review", Type: GateTypePeerReview, Rules: "代码和方案质量互审", MaxAttempts: 3, Fallback: GateFallbackEscalate},
	},
	WorkflowProfileFastRelease: {
		{Name: "auto_pass", Type: GateTypeAuto, Rules: "快速通过，仅检查基本格式", MaxAttempts: 1, Fallback: GateFallbackForcePass},
	},
}

// ResolveGates returns the gate chain for this profile.
// If Gates is explicitly set, use that; otherwise use defaults.
func (p WorkflowProfile) ResolveGates() []Gate {
	if len(p.Gates) > 0 {
		return p.Gates
	}
	if gates, ok := defaultProfileGates[p.Type]; ok {
		return gates
	}
	return defaultProfileGates[WorkflowProfileNormal]
}

// Validate checks whether the workflow profile contains valid V2 settings.
func (p WorkflowProfile) Validate() error {
	if err := p.Type.Validate(); err != nil {
		return err
	}
	if p.SLAMinutes < MinWorkflowProfileSLAMinutes || p.SLAMinutes > MaxWorkflowProfileSLAMinutes {
		return fmt.Errorf(
			"sla_minutes must be between %d and %d",
			MinWorkflowProfileSLAMinutes,
			MaxWorkflowProfileSLAMinutes,
		)
	}
	if err := ValidateGates(p.Gates); err != nil {
		return fmt.Errorf("gates: %w", err)
	}
	return nil
}
