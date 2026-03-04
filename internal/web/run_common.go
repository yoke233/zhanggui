package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
)

func parsePaginationParams(r *http.Request) (int, int, error) {
	limit := 20
	offset := 0

	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			return 0, 0, fmt.Errorf("limit must be a positive integer")
		}
		limit = parsed
	}

	if rawOffset := strings.TrimSpace(r.URL.Query().Get("offset")); rawOffset != "" {
		parsed, err := strconv.Atoi(rawOffset)
		if err != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = parsed
	}

	return limit, offset, nil
}

func normalizeStageRoleBindings(stageRoleBindings map[string]string) map[core.StageID]string {
	if len(stageRoleBindings) == 0 {
		return nil
	}

	normalized := make(map[core.StageID]string, len(stageRoleBindings))
	for rawStage, rawRole := range stageRoleBindings {
		stage := core.StageID(strings.TrimSpace(rawStage))
		role := strings.TrimSpace(rawRole)
		if stage == "" || role == "" {
			continue
		}
		normalized[stage] = role
	}
	return normalized
}

func buildRunstages(template string, stageRoles map[core.StageID]string) ([]core.StageConfig, error) {
	stageIDs, ok := engine.Templates[template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", template)
	}

	stages := make([]core.StageConfig, len(stageIDs))
	for i, stageID := range stageIDs {
		stages[i] = defaultRunStageConfig(stageID)
		if role, ok := stageRoles[stageID]; ok {
			stages[i].Role = role
		}
	}
	return stages, nil
}

func defaultRunStageConfig(id core.StageID) core.StageConfig {
	cfg := core.StageConfig{
		Name:           id,
		PromptTemplate: string(id),
		Timeout:        30 * time.Minute,
		MaxRetries:     1,
		OnFailure:      core.OnFailureHuman,
	}

	switch id {
	case core.StageRequirements, core.StageCodeReview:
		cfg.Agent = "claude"
	case core.StageImplement, core.StageFixup:
		cfg.Agent = "codex"
	case core.StageE2ETest:
		cfg.Agent = "codex"
		cfg.Timeout = 15 * time.Minute
	case core.StageWorktreeSetup, core.StageMerge, core.StageCleanup:
		cfg.Timeout = 2 * time.Minute
	}
	return cfg
}