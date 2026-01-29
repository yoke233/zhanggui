package execution

import (
	"github.com/yoke233/zhanggui/internal/planning"
	"github.com/yoke233/zhanggui/internal/scheduler"
)

func CapsFromPlan(p planning.DeliveryPlan) (scheduler.Caps, error) {
	globalMax := p.Budgets.MaxParallel
	if globalMax == 0 {
		globalMax = 4
	}
	caps := scheduler.Caps{
		GlobalMax: globalMax,
		PerTeam:   p.Budgets.PerTeamParallelCap,
		PerRole:   p.Budgets.PerRoleParallelCap,
	}
	if err := caps.Validate(); err != nil {
		return scheduler.Caps{}, err
	}
	return caps, nil
}
