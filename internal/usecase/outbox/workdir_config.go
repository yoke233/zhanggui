package outbox

import "strings"

func shouldUseWorkdir(cfg workflowWorkdirConfig, role string) bool {
	if !cfg.Enabled {
		return false
	}
	role = strings.TrimSpace(role)
	if role == "" {
		return false
	}
	for _, item := range cfg.Roles {
		if strings.TrimSpace(item) == role {
			return true
		}
	}
	return false
}
