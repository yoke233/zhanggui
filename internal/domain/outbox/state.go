package outbox

import (
	"fmt"
	"strings"
)

var allowedStates = map[string]struct{}{
	"state:todo":    {},
	"state:doing":   {},
	"state:blocked": {},
	"state:review":  {},
	"state:done":    {},
}

func NormalizeStateLabel(state string) (string, error) {
	trimmed := strings.TrimSpace(state)
	if trimmed == "" {
		return "", nil
	}

	if !strings.HasPrefix(trimmed, "state:") {
		trimmed = "state:" + trimmed
	}

	if _, ok := allowedStates[trimmed]; !ok {
		return "", fmt.Errorf("%w: %q", ErrInvalidStateLabel, state)
	}
	return trimmed, nil
}

func RequiresWorkStartValidation(state string) bool {
	return state == "state:doing" || state == "state:review" || state == "state:done"
}
