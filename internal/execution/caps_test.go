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
	if c.GlobalMax <= 0 {
		t.Fatalf("expected GlobalMax > 0")
	}
}
