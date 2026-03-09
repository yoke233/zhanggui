package core

import (
	"fmt"
	"strings"
	"time"
)

// DecomposeProposal is a draft DAG of issues produced by Team Leader,
// pending user review before actual Issue creation.
type DecomposeProposal struct {
	ID        string         `json:"proposal_id"`
	ProjectID string         `json:"project_id"`
	Prompt    string         `json:"prompt"`
	Summary   string         `json:"summary"`
	Items     []ProposalItem `json:"issues"`
	CreatedAt time.Time      `json:"created_at"`
}

// ProposalItem is one node in the proposed DAG.
type ProposalItem struct {
	TempID       string       `json:"temp_id"`
	Title        string       `json:"title"`
	Body         string       `json:"body"`
	Labels       []string     `json:"labels"`
	DependsOn    []string     `json:"depends_on"`
	ChildrenMode ChildrenMode `json:"children_mode,omitempty"`
	Template     string       `json:"template,omitempty"`
	AutoMerge    *bool        `json:"auto_merge,omitempty"`
}

func NewProposalID() string {
	return fmt.Sprintf("prop-%s-%s", time.Now().UTC().Format("20060102-150405"), randomHex(4))
}

func (p DecomposeProposal) Validate() error {
	if len(p.Items) == 0 {
		return fmt.Errorf("proposal must have at least one item")
	}

	ids := make(map[string]struct{}, len(p.Items))
	graph := make(map[string][]string, len(p.Items))
	for _, item := range p.Items {
		id := strings.TrimSpace(item.TempID)
		if id == "" {
			return fmt.Errorf("proposal item missing temp_id")
		}
		if strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("proposal item %q missing title", id)
		}
		if item.ChildrenMode != "" {
			if err := item.ChildrenMode.Validate(); err != nil {
				return fmt.Errorf("proposal item %q %w", id, err)
			}
		}
		if _, exists := ids[id]; exists {
			return fmt.Errorf("proposal item %q duplicated", id)
		}
		ids[id] = struct{}{}
		deps := make([]string, 0, len(item.DependsOn))
		seenDeps := make(map[string]struct{}, len(item.DependsOn))
		for _, dep := range item.DependsOn {
			depID := strings.TrimSpace(dep)
			if depID == "" {
				continue
			}
			if _, exists := seenDeps[depID]; exists {
				continue
			}
			seenDeps[depID] = struct{}{}
			deps = append(deps, depID)
		}
		graph[id] = deps
	}

	for id, deps := range graph {
		for _, dep := range deps {
			if _, exists := ids[dep]; !exists {
				return fmt.Errorf("proposal item %q depends on unknown %q", id, dep)
			}
		}
	}

	const (
		visitNew = iota
		visitActive
		visitDone
	)
	state := make(map[string]int, len(graph))
	var dfs func(string) error
	dfs = func(id string) error {
		switch state[id] {
		case visitActive:
			return fmt.Errorf("proposal contains cyclic dependency at %q", id)
		case visitDone:
			return nil
		}
		state[id] = visitActive
		for _, dep := range graph[id] {
			if err := dfs(dep); err != nil {
				return err
			}
		}
		state[id] = visitDone
		return nil
	}
	for id := range graph {
		if err := dfs(id); err != nil {
			return err
		}
	}
	return nil
}
