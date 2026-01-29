package planning

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

type DeliveryPlan struct {
	Teams       []Team        `yaml:"teams,omitempty" json:"teams,omitempty"`
	Roles       []Role        `yaml:"roles,omitempty" json:"roles,omitempty"`
	Quality     []QualityGate `yaml:"quality,omitempty" json:"quality,omitempty"`
	Budgets     Budgets       `yaml:"budgets,omitempty" json:"budgets,omitempty"`
	AuditPolicy AuditPolicy   `yaml:"audit_policy,omitempty" json:"audit_policy,omitempty"`
}

type Team struct {
	TeamID string `yaml:"team_id" json:"team_id"`
	Intent string `yaml:"intent,omitempty" json:"intent,omitempty"`
}

type Role struct {
	Role  string   `yaml:"role" json:"role"`
	Count int      `yaml:"count" json:"count"`
	Owns  []string `yaml:"owns,omitempty" json:"owns,omitempty"`
}

type QualityGate struct {
	Gate string `yaml:"gate" json:"gate"`
}

type Budgets struct {
	MaxParallel         int            `yaml:"max_parallel,omitempty" json:"max_parallel,omitempty"`
	PerRoleParallelCap  map[string]int `yaml:"per_role_parallel_cap,omitempty" json:"per_role_parallel_cap,omitempty"`
	PerTeamParallelCap  map[string]int `yaml:"per_team_parallel_cap,omitempty" json:"per_team_parallel_cap,omitempty"`
	PerAgentParallelCap int            `yaml:"per_agent_parallel_cap,omitempty" json:"per_agent_parallel_cap,omitempty"`
}

type AuditPolicy struct {
	ApprovalPolicy string   `yaml:"approval_policy,omitempty" json:"approval_policy,omitempty"`
	ApprovalGate   []string `yaml:"approval_gate,omitempty" json:"approval_gate,omitempty"`
}

func ParseDeliveryPlanYAML(data []byte) (DeliveryPlan, error) {
	var p DeliveryPlan
	if err := yaml.Unmarshal(data, &p); err != nil {
		return DeliveryPlan{}, err
	}
	return p, nil
}

func (p DeliveryPlan) Validate() error {
	seenTeams := map[string]struct{}{}
	for i, t := range p.Teams {
		if strings.TrimSpace(t.TeamID) == "" {
			return fmt.Errorf("teams[%d].team_id 不能为空", i)
		}
		if _, ok := seenTeams[t.TeamID]; ok {
			return fmt.Errorf("重复 team_id: %s", t.TeamID)
		}
		seenTeams[t.TeamID] = struct{}{}
	}

	seenRoles := map[string]struct{}{}
	for i, r := range p.Roles {
		if strings.TrimSpace(r.Role) == "" {
			return fmt.Errorf("roles[%d].role 不能为空", i)
		}
		if r.Count <= 0 {
			return fmt.Errorf("roles[%d].count 必须 >0", i)
		}
		// v1：同名 role 只允许出现一次；未来如需“按 team 定义 role”，再引入 team_id 字段。
		if _, ok := seenRoles[r.Role]; ok {
			return fmt.Errorf("重复 role: %s", r.Role)
		}
		seenRoles[r.Role] = struct{}{}
	}

	for i, q := range p.Quality {
		if strings.TrimSpace(q.Gate) == "" {
			return fmt.Errorf("quality[%d].gate 不能为空", i)
		}
	}

	if err := p.Budgets.Validate(); err != nil {
		return err
	}

	if err := p.AuditPolicy.Validate(); err != nil {
		return err
	}

	return nil
}

func (b Budgets) Validate() error {
	if b.MaxParallel < 0 {
		return fmt.Errorf("budgets.max_parallel 不能为负数")
	}
	if b.PerAgentParallelCap < 0 {
		return fmt.Errorf("budgets.per_agent_parallel_cap 不能为负数")
	}
	for role, cap := range b.PerRoleParallelCap {
		if strings.TrimSpace(role) == "" {
			return fmt.Errorf("budgets.per_role_parallel_cap 存在空 role")
		}
		if cap <= 0 {
			return fmt.Errorf("budgets.per_role_parallel_cap[%s] 必须 >0", role)
		}
	}
	for team, cap := range b.PerTeamParallelCap {
		if strings.TrimSpace(team) == "" {
			return fmt.Errorf("budgets.per_team_parallel_cap 存在空 team_id")
		}
		if cap <= 0 {
			return fmt.Errorf("budgets.per_team_parallel_cap[%s] 必须 >0", team)
		}
	}
	// 允许不设置 MaxParallel（交由运行时默认值），因此这里不做上界约束。
	return nil
}

func (a AuditPolicy) Validate() error {
	if strings.TrimSpace(a.ApprovalPolicy) == "" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(a.ApprovalPolicy)) {
	case "always", "warn", "gate", "never":
	default:
		return fmt.Errorf("audit_policy.approval_policy 非法: %s", a.ApprovalPolicy)
	}
	return nil
}
