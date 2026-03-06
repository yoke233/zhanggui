package mcpserver

import (
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// resolveProjectID resolves a project ID from either an explicit ID or a name.
// Priority: id > name > single-project auto-infer.
func resolveProjectID(store core.Store, id, name string) (string, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)

	if id != "" {
		return id, nil
	}

	if name != "" {
		projects, err := store.ListProjects(core.ProjectFilter{NameContains: name})
		if err != nil {
			return "", fmt.Errorf("list projects: %w", err)
		}
		// Exact match takes priority — but must be unique.
		var exactMatches []core.Project
		for _, p := range projects {
			if strings.EqualFold(p.Name, name) {
				exactMatches = append(exactMatches, p)
			}
		}
		if len(exactMatches) == 1 {
			return exactMatches[0].ID, nil
		}
		if len(exactMatches) > 1 {
			names := make([]string, len(exactMatches))
			for i, p := range exactMatches {
				names[i] = fmt.Sprintf("%s (%s)", p.Name, p.ID)
			}
			return "", fmt.Errorf("multiple projects named %q: %s", name, strings.Join(names, ", "))
		}
		if len(projects) == 1 {
			return projects[0].ID, nil
		}
		if len(projects) == 0 {
			return "", fmt.Errorf("no project found matching name %q", name)
		}
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = fmt.Sprintf("%s (%s)", p.Name, p.ID)
		}
		return "", fmt.Errorf("ambiguous project name %q, candidates: %s", name, strings.Join(names, ", "))
	}

	// Both empty: auto-infer if exactly one project exists.
	projects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return "", fmt.Errorf("list projects: %w", err)
	}
	if len(projects) == 1 {
		return projects[0].ID, nil
	}
	if len(projects) == 0 {
		return "", fmt.Errorf("no projects exist; provide project_id or project_name")
	}
	return "", fmt.Errorf("multiple projects exist; provide project_id or project_name")
}
