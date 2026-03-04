package engine

import "github.com/yoke233/ai-workflow/internal/core"

var Templates = map[string][]core.StageID{
	"full": {
		core.StageSetup, core.StageRequirements, core.StageImplement,
		core.StageReview, core.StageFixup, core.StageTest,
		core.StageMerge, core.StageCleanup,
	},
	"standard": {
		core.StageSetup, core.StageRequirements, core.StageImplement,
		core.StageReview, core.StageFixup, core.StageMerge, core.StageCleanup,
	},
	"quick": {
		core.StageSetup, core.StageRequirements, core.StageImplement,
		core.StageReview, core.StageMerge, core.StageCleanup,
	},
	"hotfix": {
		core.StageSetup, core.StageImplement, core.StageMerge, core.StageCleanup,
	},
}
