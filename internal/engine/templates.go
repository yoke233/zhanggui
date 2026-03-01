package engine

import "github.com/user/ai-workflow/internal/core"

var Templates = map[string][]core.StageID{
	"full": {
		core.StageWorktreeSetup, core.StageRequirements, core.StageImplement,
		core.StageCodeReview, core.StageFixup, core.StageE2ETest,
		core.StageMerge, core.StageCleanup,
	},
	"standard": {
		core.StageWorktreeSetup, core.StageRequirements, core.StageImplement,
		core.StageCodeReview, core.StageFixup, core.StageMerge, core.StageCleanup,
	},
	"quick": {
		core.StageWorktreeSetup, core.StageRequirements, core.StageImplement,
		core.StageCodeReview, core.StageMerge, core.StageCleanup,
	},
	"hotfix": {
		core.StageWorktreeSetup, core.StageImplement, core.StageMerge, core.StageCleanup,
	},
}
