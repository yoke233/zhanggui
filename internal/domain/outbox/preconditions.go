package outbox

import "strings"

type WorkPreconditions struct {
	TargetState            string
	Assignee               string
	HasNeedsHuman          bool
	UnresolvedDependencies []string
}

func EvaluateWorkPreconditions(in WorkPreconditions) ([]string, error) {
	if !RequiresWorkStartValidation(in.TargetState) {
		return nil, nil
	}

	if strings.TrimSpace(in.Assignee) == "" {
		return nil, ErrIssueNotClaimed
	}

	if in.HasNeedsHuman {
		return []string{"needs-human"}, ErrNeedsHuman
	}

	if len(in.UnresolvedDependencies) > 0 {
		deps := make([]string, 0, len(in.UnresolvedDependencies))
		for _, dep := range in.UnresolvedDependencies {
			trimmed := strings.TrimSpace(dep)
			if trimmed != "" {
				deps = append(deps, trimmed)
			}
		}
		if len(deps) > 0 {
			return deps, ErrDependsUnresolved
		}
	}

	return nil, nil
}
