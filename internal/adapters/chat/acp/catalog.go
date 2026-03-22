package acp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	chatapp "github.com/yoke233/zhanggui/internal/application/chat"
)

const leadSessionCatalogFileName = "lead-chat-sessions.json"

type persistedLeadCatalog struct {
	Sessions []*persistedLeadSession `json:"sessions"`
}

type persistedLeadSession struct {
	SessionID         string                     `json:"session_id"`
	Scope             string                     `json:"scope"`
	WorkDir           string                     `json:"work_dir,omitempty"`
	Branch            string                     `json:"branch,omitempty"`
	Isolation         string                     `json:"isolation,omitempty"`
	RepoPath          string                     `json:"repo_path,omitempty"`
	Title             string                     `json:"title,omitempty"`
	ProjectID         int64                      `json:"project_id,omitempty"`
	ProjectName       string                     `json:"project_name,omitempty"`
	ProfileID         string                     `json:"profile_id,omitempty"`
	ProfileName       string                     `json:"profile_name,omitempty"`
	DriverID          string                     `json:"driver_id,omitempty"`
	LLMConfigID       string                     `json:"llm_config_id,omitempty"`
	AvailableCommands []chatapp.AvailableCommand `json:"available_commands,omitempty"`
	ConfigOptions     []chatapp.ConfigOption     `json:"config_options,omitempty"`
	Modes             *chatapp.SessionModeState  `json:"modes,omitempty"`
	PrURL             string                     `json:"pr_url,omitempty"`
	PrNumber          int                        `json:"pr_number,omitempty"`
	PrState           string                     `json:"pr_state,omitempty"`
	Archived          bool                       `json:"archived,omitempty"`
	CreatedAt         time.Time                  `json:"created_at"`
	UpdatedAt         time.Time                  `json:"updated_at"`
	Messages          []chatapp.Message          `json:"messages,omitempty"`
}

func loadLeadCatalog(path string) (map[string]*persistedLeadSession, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]*persistedLeadSession{}, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]*persistedLeadSession{}, nil
		}
		return nil, fmt.Errorf("read lead session catalog: %w", err)
	}

	var payload persistedLeadCatalog
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode lead session catalog: %w", err)
	}

	out := make(map[string]*persistedLeadSession, len(payload.Sessions))
	for _, item := range payload.Sessions {
		if item == nil {
			continue
		}
		id := strings.TrimSpace(item.SessionID)
		if id == "" {
			continue
		}
		cloned := *item
		cloned.Messages = append([]chatapp.Message(nil), item.Messages...)
		cloned.AvailableCommands = cloneAvailableCommands(item.AvailableCommands)
		cloned.ConfigOptions = cloneConfigOptions(item.ConfigOptions)
		cloned.Modes = cloneModeState(item.Modes)
		out[id] = &cloned
	}
	return out, nil
}

func saveLeadCatalog(path string, sessions map[string]*persistedLeadSession) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create lead catalog dir: %w", err)
	}

	items := make([]*persistedLeadSession, 0, len(sessions))
	for _, item := range sessions {
		if item == nil || strings.TrimSpace(item.SessionID) == "" {
			continue
		}
		cloned := *item
		cloned.Messages = append([]chatapp.Message(nil), item.Messages...)
		cloned.AvailableCommands = cloneAvailableCommands(item.AvailableCommands)
		cloned.ConfigOptions = cloneConfigOptions(item.ConfigOptions)
		cloned.Modes = cloneModeState(item.Modes)
		items = append(items, &cloned)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].SessionID < items[j].SessionID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	payload := persistedLeadCatalog{Sessions: items}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lead session catalog: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write lead session catalog temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace lead session catalog: %w", err)
	}
	return nil
}

func cloneAvailableCommands(items []chatapp.AvailableCommand) []chatapp.AvailableCommand {
	if items == nil {
		return nil
	}
	cloned := make([]chatapp.AvailableCommand, len(items))
	copy(cloned, items)
	for i := range cloned {
		if items[i].Input == nil {
			continue
		}
		input := *items[i].Input
		cloned[i].Input = &input
	}
	return cloned
}

func cloneModeState(state *chatapp.SessionModeState) *chatapp.SessionModeState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.AvailableModes = append([]chatapp.SessionMode(nil), state.AvailableModes...)
	return &cloned
}

func cloneConfigOptions(items []chatapp.ConfigOption) []chatapp.ConfigOption {
	if items == nil {
		return nil
	}
	cloned := make([]chatapp.ConfigOption, len(items))
	copy(cloned, items)
	for i := range cloned {
		cloned[i].Options = append([]chatapp.ConfigOptionValue(nil), items[i].Options...)
	}
	return cloned
}
