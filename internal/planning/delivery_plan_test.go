package planning

import "testing"

func TestParseDeliveryPlanYAML_Validate_OK(t *testing.T) {
	data := []byte(`
teams:
  - team_id: team_a
    intent: "主方案（稳健）"
  - team_id: team_b
    intent: "备选方案（激进/更快/更省成本）"
roles:
  - role: planner_editor
    count: 1
    owns: ["master_ir", "outline", "acceptance_gate"]
  - role: domain_writer
    count: 2
    owns: ["cards", "sections"]
  - role: ppt_transformer
    count: 1
    owns: ["ppt_ir"]
  - role: verifier
    count: 1
    owns: ["coverage_map", "issue_list"]
quality:
  - gate: must_answer_coverage
  - gate: no_new_facts
  - gate: cross_deliverable_consistency
budgets:
  max_parallel: 10
  per_role_parallel_cap:
    domain_writer: 3
    ppt_transformer: 1
audit_policy:
  approval_policy: gate
  approval_gate: ["verify", "pack"]
`)
	p, err := ParseDeliveryPlanYAML(data)
	if err != nil {
		t.Fatalf("ParseDeliveryPlanYAML: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if p.Budgets.MaxParallel != 10 {
		t.Fatalf("expected max_parallel=10, got %d", p.Budgets.MaxParallel)
	}
	if got := p.Budgets.PerRoleParallelCap["domain_writer"]; got != 3 {
		t.Fatalf("expected per_role_parallel_cap.domain_writer=3, got %d", got)
	}
	if got := p.AuditPolicy.ApprovalPolicy; got != "gate" {
		t.Fatalf("expected approval_policy=gate, got %q", got)
	}
}

func TestDeliveryPlan_Validate_Errors(t *testing.T) {
	t.Run("dup team_id", func(t *testing.T) {
		p := DeliveryPlan{
			Teams: []Team{{TeamID: "team_a"}, {TeamID: "team_a"}},
		}
		if err := p.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("bad role count", func(t *testing.T) {
		p := DeliveryPlan{
			Roles: []Role{{Role: "writer", Count: 0}},
		}
		if err := p.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("bad budgets", func(t *testing.T) {
		p := DeliveryPlan{
			Budgets: Budgets{PerRoleParallelCap: map[string]int{"writer": 0}},
		}
		if err := p.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("bad approval policy", func(t *testing.T) {
		p := DeliveryPlan{
			AuditPolicy: AuditPolicy{ApprovalPolicy: "sometimes"},
		}
		if err := p.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
}
