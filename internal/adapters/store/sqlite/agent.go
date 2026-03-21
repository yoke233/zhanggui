package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
	skillset "github.com/yoke233/zhanggui/internal/skills"
)

// Ensure Store implements AgentRegistry.
var _ core.AgentRegistry = (*Store)(nil)

// ---------- Profile CRUD ----------

func (s *Store) GetProfile(ctx context.Context, id string) (*core.AgentProfile, error) {
	return s.scanProfile(s.db.QueryRowContext(ctx,
		`SELECT id, name, driver_id, llm_config_id, driver_config, role, capabilities, actions_allowed,
		        prompt_template, session_reuse, session_max_turns, session_idle_ttl_ms,
		        mcp_enabled, mcp_tools, skills
		 FROM agent_profiles WHERE id = ?`, id))
}

func (s *Store) ListProfiles(ctx context.Context) ([]*core.AgentProfile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, driver_id, llm_config_id, driver_config, role, capabilities, actions_allowed,
		        prompt_template, session_reuse, session_max_turns, session_idle_ttl_ms,
		        mcp_enabled, mcp_tools, skills
		 FROM agent_profiles ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	defer rows.Close()

	var out []*core.AgentProfile
	for rows.Next() {
		p, err := s.scanProfileRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreateProfile(ctx context.Context, p *core.AgentProfile) error {
	// Check duplicate.
	var exists int
	_ = s.db.QueryRowContext(ctx, `SELECT 1 FROM agent_profiles WHERE id = ?`, p.ID).Scan(&exists)
	if exists == 1 {
		return fmt.Errorf("%w: %q", core.ErrDuplicateProfile, p.ID)
	}

	// Validate capabilities don't overflow.
	if err := s.validateProfile(ctx, p); err != nil {
		return err
	}

	return s.insertProfile(ctx, p)
}

func (s *Store) UpdateProfile(ctx context.Context, p *core.AgentProfile) error {
	// Check exists.
	var exists int
	_ = s.db.QueryRowContext(ctx, `SELECT 1 FROM agent_profiles WHERE id = ?`, p.ID).Scan(&exists)
	if exists == 0 {
		return fmt.Errorf("%w: %q", core.ErrProfileNotFound, p.ID)
	}

	// Validate capabilities don't overflow.
	if err := s.validateProfile(ctx, p); err != nil {
		return err
	}

	driverCfg, _ := marshalJSON(p.Driver)
	caps, _ := marshalJSON(p.Capabilities)
	actions, _ := marshalJSON(p.ActionsAllowed)
	mcpTools, _ := marshalJSON(p.MCP.Tools)
	skills, _ := marshalJSON(p.Skills)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_profiles SET name = ?, driver_id = ?, llm_config_id = ?, driver_config = ?, role = ?,
		        capabilities = ?, actions_allowed = ?, prompt_template = ?,
		        skills = ?,
		        session_reuse = ?, session_max_turns = ?, session_idle_ttl_ms = ?,
		        mcp_enabled = ?, mcp_tools = ?, updated_at = ?
		 WHERE id = ?`,
		p.Name, p.DriverID, p.LLMConfigID, driverCfg, string(p.Role),
		caps, actions, p.PromptTemplate,
		skills,
		p.Session.Reuse, p.Session.MaxTurns, p.Session.IdleTTL.Milliseconds(),
		p.MCP.Enabled, mcpTools, now,
		p.ID)
	return err
}

func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agent_profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %q", core.ErrProfileNotFound, id)
	}
	return nil
}

// ---------- Resolution ----------

func (s *Store) ResolveForAction(ctx context.Context, action *core.Action) (*core.AgentProfile, error) {
	profiles, err := s.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}

	role := core.AgentRole(action.AgentRole)
	for _, p := range profiles {
		if role != "" && p.Role != role {
			continue
		}
		if !p.MatchesRequirements(action.RequiredCapabilities) {
			continue
		}
		return p, nil
	}
	return nil, core.ErrNoMatchingAgent
}

func (s *Store) ResolveByID(ctx context.Context, profileID string) (*core.AgentProfile, error) {
	p, err := s.GetProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ---------- helpers ----------

func (s *Store) validateProfile(_ context.Context, p *core.AgentProfile) error {
	profileCaps := p.EffectiveCapabilities()
	if !p.Driver.CapabilitiesMax.Covers(profileCaps) {
		return fmt.Errorf("%w: profile %q exceeds driver capabilities_max", core.ErrCapabilityOverflow, p.ID)
	}
	if err := skillset.ValidateProfileSkills(p.Skills); err != nil {
		return err
	}
	return nil
}

func (s *Store) insertProfile(ctx context.Context, p *core.AgentProfile) error {
	driverCfg, _ := marshalJSON(p.Driver)
	caps, _ := marshalJSON(p.Capabilities)
	actions, _ := marshalJSON(p.ActionsAllowed)
	mcpTools, _ := marshalJSON(p.MCP.Tools)
	skills, _ := marshalJSON(p.Skills)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_profiles (id, name, driver_id, llm_config_id, driver_config, role, capabilities, actions_allowed,
		        prompt_template, skills, session_reuse, session_max_turns, session_idle_ttl_ms,
		        mcp_enabled, mcp_tools, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.DriverID, p.LLMConfigID, driverCfg, string(p.Role),
		caps, actions, p.PromptTemplate, skills,
		p.Session.Reuse, p.Session.MaxTurns, p.Session.IdleTTL.Milliseconds(),
		p.MCP.Enabled, mcpTools, now, now)
	if err != nil {
		return fmt.Errorf("insert profile: %w", err)
	}
	return nil
}

// scanProfile scans a single profile row from QueryRow.
func (s *Store) scanProfile(row *sql.Row) (*core.AgentProfile, error) {
	p := &core.AgentProfile{}
	var driverCfg, caps, actions, mcpTools, skills sql.NullString
	var role string
	var idleTTLMs int64
	err := row.Scan(&p.ID, &p.Name, &p.DriverID, &p.LLMConfigID, &driverCfg, &role,
		&caps, &actions, &p.PromptTemplate,
		&p.Session.Reuse, &p.Session.MaxTurns, &idleTTLMs,
		&p.MCP.Enabled, &mcpTools, &skills)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: scanned", core.ErrProfileNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("scan profile: %w", err)
	}
	p.Role = core.AgentRole(role)
	p.Session.IdleTTL = time.Duration(idleTTLMs) * time.Millisecond
	if driverCfg.Valid {
		_ = json.Unmarshal([]byte(driverCfg.String), &p.Driver)
	}
	unmarshalNullJSON(caps, &p.Capabilities)
	unmarshalNullJSON(actions, &p.ActionsAllowed)
	unmarshalNullJSON(mcpTools, &p.MCP.Tools)
	unmarshalNullJSON(skills, &p.Skills)
	return p, nil
}

// scanProfileRow scans a profile from Rows (used in ListProfiles).
func (s *Store) scanProfileRow(rows *sql.Rows) (*core.AgentProfile, error) {
	p := &core.AgentProfile{}
	var driverCfg, caps, actions, mcpTools, skills sql.NullString
	var role string
	var idleTTLMs int64
	if err := rows.Scan(&p.ID, &p.Name, &p.DriverID, &p.LLMConfigID, &driverCfg, &role,
		&caps, &actions, &p.PromptTemplate,
		&p.Session.Reuse, &p.Session.MaxTurns, &idleTTLMs,
		&p.MCP.Enabled, &mcpTools, &skills); err != nil {
		return nil, fmt.Errorf("scan profile row: %w", err)
	}
	p.Role = core.AgentRole(role)
	p.Session.IdleTTL = time.Duration(idleTTLMs) * time.Millisecond
	if driverCfg.Valid {
		_ = json.Unmarshal([]byte(driverCfg.String), &p.Driver)
	}
	unmarshalNullJSON(caps, &p.Capabilities)
	unmarshalNullJSON(actions, &p.ActionsAllowed)
	unmarshalNullJSON(mcpTools, &p.MCP.Tools)
	unmarshalNullJSON(skills, &p.Skills)
	return p, nil
}

// UpsertProfile inserts or replaces a profile (used for seeding from config).
func (s *Store) UpsertProfile(ctx context.Context, p *core.AgentProfile) error {
	if err := s.validateProfile(ctx, p); err != nil {
		return err
	}
	driverCfg, _ := marshalJSON(p.Driver)
	caps, _ := marshalJSON(p.Capabilities)
	actions, _ := marshalJSON(p.ActionsAllowed)
	mcpTools, _ := marshalJSON(p.MCP.Tools)
	skills, _ := marshalJSON(p.Skills)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_profiles (id, name, driver_id, llm_config_id, driver_config, role, capabilities, actions_allowed,
		        prompt_template, skills, session_reuse, session_max_turns, session_idle_ttl_ms,
		        mcp_enabled, mcp_tools, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		    name = excluded.name,
		    driver_id = excluded.driver_id,
		    llm_config_id = excluded.llm_config_id,
		    driver_config = excluded.driver_config,
		    role = excluded.role,
		    capabilities = excluded.capabilities,
		    actions_allowed = excluded.actions_allowed,
		    prompt_template = excluded.prompt_template,
		    skills = excluded.skills,
		    session_reuse = excluded.session_reuse,
		    session_max_turns = excluded.session_max_turns,
		    session_idle_ttl_ms = excluded.session_idle_ttl_ms,
		    mcp_enabled = excluded.mcp_enabled,
		    mcp_tools = excluded.mcp_tools,
		    updated_at = excluded.updated_at`,
		p.ID, p.Name, p.DriverID, p.LLMConfigID, driverCfg, string(p.Role),
		caps, actions, p.PromptTemplate, skills,
		p.Session.Reuse, p.Session.MaxTurns, p.Session.IdleTTL.Milliseconds(),
		p.MCP.Enabled, mcpTools, now, now)
	return err
}
