package execution

import (
	"testing"

	"github.com/yoke233/zhanggui/internal/planning"
)

func TestCapsFromPlan_Defaults(t *testing.T) {
	p := planning.DeliveryPlan{Budgets: planning.Budgets{MaxParallel: 0}}
	c, err := CapsFromPlan(p)
	if err != nil {
		t.Fatalf("CapsFromPlan: %v", err)
	}
	if c.GlobalMax != 4 {
		t.Fatalf("expected GlobalMax=4, got %d", c.GlobalMax)
	}
}

func TestCapsFromPlan_MapsCaps(t *testing.T) {
	p := planning.DeliveryPlan{
		Budgets: planning.Budgets{
			MaxParallel: 3,
			PerRoleParallelCap: map[string]int{
				"writer": 2,
			},
			PerTeamParallelCap: map[string]int{
				"team_a": 1,
			},
		},
	}
	c, err := CapsFromPlan(p)
	if err != nil {
		t.Fatalf("CapsFromPlan: %v", err)
	}
	if c.GlobalMax != 3 {
		t.Fatalf("expected GlobalMax=3, got %d", c.GlobalMax)
	}
	if got := c.PerRole["writer"]; got != 2 {
		t.Fatalf("expected PerRole[writer]=2, got %d", got)
	}
	if got := c.PerTeam["team_a"]; got != 1 {
		t.Fatalf("expected PerTeam[team_a]=1, got %d", got)
	}
}

func TestCapsFromPlan_Invalid(t *testing.T) {
	p := planning.DeliveryPlan{Budgets: planning.Budgets{MaxParallel: -1}}
	if _, err := CapsFromPlan(p); err == nil {
		t.Fatalf("expected error")
	}
}
