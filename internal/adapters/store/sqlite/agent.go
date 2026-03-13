package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	skillset "github.com/yoke233/ai-workflow/internal/skills"
)

// Ensure Store implements AgentRegistry.
var _ core.AgentRegistry = (*Store)(nil)

// ---------- Driver CRUD ----------

func (s *Store) GetDriver(ctx context.Context, id string) (*core.AgentDriver, error) {
	d := &core.AgentDriver{}
	var args, env sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, launch_command, launch_args, env,
		        cap_fs_read, cap_fs_write, cap_terminal
		 FROM agent_drivers WHERE id = ?`, id,
	).Scan(&d.ID, &d.LaunchCommand, &args, &env,
		&d.CapabilitiesMax.FSRead, &d.CapabilitiesMax.FSWrite, &d.CapabilitiesMax.Terminal)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %q", core.ErrDriverNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get driver %q: %w", id, err)
	}
	unmarshalNullJSON(args, &d.LaunchArgs)
	if env.Valid {
		d.Env = map[string]string{}
		_ = json.Unmarshal([]byte(env.String), &d.Env)
	}
	return d, nil
}

func (s *Store) ListDrivers(ctx context.Context) ([]*core.AgentDriver, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, launch_command, launch_args, env,
		        cap_fs_read, cap_fs_write, cap_terminal
		 FROM agent_drivers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list drivers: %w", err)
	}
	defer rows.Close()

	var out []*core.AgentDriver
	for rows.Next() {
		d := &core.AgentDriver{}
		var args, env sql.NullString
		if err := rows.Scan(&d.ID, &d.LaunchCommand, &args, &env,
			&d.CapabilitiesMax.FSRead, &d.CapabilitiesMax.FSWrite, &d.CapabilitiesMax.Terminal); err != nil {
			return nil, fmt.Errorf("scan driver: %w", err)
		}
		unmarshalNullJSON(args, &d.LaunchArgs)
		if env.Valid {
			d.Env = map[string]string{}
			_ = json.Unmarshal([]byte(env.String), &d.Env)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) CreateDriver(ctx context.Context, d *core.AgentDriver) error {
	// Check duplicate.
	var exists int
	_ = s.db.QueryRowContext(ctx, `SELECT 1 FROM agent_drivers WHERE id = ?`, d.ID).Scan(&exists)
	if exists == 1 {
		return fmt.Errorf("%w: %q", core.ErrDuplicateDriver, d.ID)
	}

	args, _ := marshalJSON(d.LaunchArgs)
	env, _ := marshalJSON(d.Env)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_drivers (id, launch_command, launch_args, env,
		        cap_fs_read, cap_fs_write, cap_terminal, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.LaunchCommand, args, env,
		d.CapabilitiesMax.FSRead, d.CapabilitiesMax.FSWrite, d.CapabilitiesMax.Terminal,
		now, now)
	if err != nil {
		return fmt.Errorf("insert driver: %w", err)
	}
	return nil
}

func (s *Store) UpdateDriver(ctx context.Context, d *core.AgentDriver) error {
	args, _ := marshalJSON(d.LaunchArgs)
	env, _ := marshalJSON(d.Env)
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE agent_drivers SET launch_command = ?, launch_args = ?, env = ?,
		        cap_fs_read = ?, cap_fs_write = ?, cap_terminal = ?, updated_at = ?
		 WHERE id = ?`,
		d.LaunchCommand, args, env,
		d.CapabilitiesMax.FSRead, d.CapabilitiesMax.FSWrite, d.CapabilitiesMax.Terminal,
		now, d.ID)
	if err != nil {
		return fmt.Errorf("update driver: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %q", core.ErrDriverNotFound, d.ID)
	}
	return nil
}

func (s *Store) DeleteDriver(ctx context.Context, id string) error {
	// Check if any profile references this driver.
	var refCount int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_profiles WHERE driver_id = ?`, id).Scan(&refCount)
	if refCount > 0 {
		return fmt.Errorf("%w: driver %q", core.ErrDriverInUse, id)
	}

	res, err := s.db.ExecContext(ctx, `DELETE FROM agent_drivers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete driver: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %q", core.ErrDriverNotFound, id)
	}
	return nil
}

// ---------- Profile CRUD ----------

func (s *Store) GetProfile(ctx context.Context, id string) (*core.AgentProfile, error) {
	return s.scanProfile(s.db.QueryRowContext(ctx,
		`SELECT id, name, driver_id, role, capabilities, actions_allowed,
		        prompt_template, session_reuse, session_max_turns, session_idle_ttl_ms,
		        mcp_enabled, mcp_tools, skills
		 FROM agent_profiles WHERE id = ?`, id))
}

func (s *Store) ListProfiles(ctx context.Context) ([]*core.AgentProfile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, driver_id, role, capabilities, actions_allowed,
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

	// Validate driver exists and capabilities don't overflow.
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

	// Validate driver exists and capabilities don't overflow.
	if err := s.validateProfile(ctx, p); err != nil {
		return err
	}

	caps, _ := marshalJSON(p.Capabilities)
	actions, _ := marshalJSON(p.ActionsAllowed)
	mcpTools, _ := marshalJSON(p.MCP.Tools)
	skills, _ := marshalJSON(p.Skills)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_profiles SET name = ?, driver_id = ?, role = ?,
		        capabilities = ?, actions_allowed = ?, prompt_template = ?,
		        skills = ?,
		        session_reuse = ?, session_max_turns = ?, session_idle_ttl_ms = ?,
		        mcp_enabled = ?, mcp_tools = ?, updated_at = ?
		 WHERE id = ?`,
		p.Name, p.DriverID, string(p.Role),
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

func (s *Store) ResolveForAction(ctx context.Context, step *core.Action) (*core.AgentProfile, *core.AgentDriver, error) {
	profiles, err := s.ListProfiles(ctx)
	if err != nil {
		return nil, nil, err
	}

	role := core.AgentRole(step.AgentRole)
	for _, p := range profiles {
		if role != "" && p.Role != role {
			continue
		}
		if !p.MatchesRequirements(step.RequiredCapabilities) {
			continue
		}
		d, err := s.GetDriver(ctx, p.DriverID)
		if err != nil {
			continue // skip profiles with missing drivers
		}
		return p, d, nil
	}
	return nil, nil, core.ErrNoMatchingAgent
}

func (s *Store) ResolveByID(ctx context.Context, profileID string) (*core.AgentProfile, *core.AgentDriver, error) {
	p, err := s.GetProfile(ctx, profileID)
	if err != nil {
		return nil, nil, err
	}
	d, err := s.GetDriver(ctx, p.DriverID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: profile %q references driver %q", core.ErrDriverNotFound, profileID, p.DriverID)
	}
	return p, d, nil
}

// ---------- helpers ----------

func (s *Store) validateProfile(ctx context.Context, p *core.AgentProfile) error {
	d, err := s.GetDriver(ctx, p.DriverID)
	if err != nil {
		return fmt.Errorf("%w: profile %q references driver %q", core.ErrDriverNotFound, p.ID, p.DriverID)
	}
	profileCaps := p.EffectiveCapabilities()
	if !d.CapabilitiesMax.Covers(profileCaps) {
		return fmt.Errorf("%w: profile %q exceeds driver %q", core.ErrCapabilityOverflow, p.ID, d.ID)
	}
	if err := skillset.ValidateProfileSkills(p.Skills); err != nil {
		return err
	}
	return nil
}

func (s *Store) insertProfile(ctx context.Context, p *core.AgentProfile) error {
	caps, _ := marshalJSON(p.Capabilities)
	actions, _ := marshalJSON(p.ActionsAllowed)
	mcpTools, _ := marshalJSON(p.MCP.Tools)
	skills, _ := marshalJSON(p.Skills)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_profiles (id, name, driver_id, role, capabilities, actions_allowed,
		        prompt_template, skills, session_reuse, session_max_turns, session_idle_ttl_ms,
		        mcp_enabled, mcp_tools, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.DriverID, string(p.Role),
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
	var caps, actions, mcpTools, skills sql.NullString
	var role string
	var idleTTLMs int64
	err := row.Scan(&p.ID, &p.Name, &p.DriverID, &role,
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
	unmarshalNullJSON(caps, &p.Capabilities)
	unmarshalNullJSON(actions, &p.ActionsAllowed)
	unmarshalNullJSON(mcpTools, &p.MCP.Tools)
	unmarshalNullJSON(skills, &p.Skills)
	return p, nil
}

// scanProfileRow scans a profile from Rows (used in ListProfiles).
func (s *Store) scanProfileRow(rows *sql.Rows) (*core.AgentProfile, error) {
	p := &core.AgentProfile{}
	var caps, actions, mcpTools, skills sql.NullString
	var role string
	var idleTTLMs int64
	if err := rows.Scan(&p.ID, &p.Name, &p.DriverID, &role,
		&caps, &actions, &p.PromptTemplate,
		&p.Session.Reuse, &p.Session.MaxTurns, &idleTTLMs,
		&p.MCP.Enabled, &mcpTools, &skills); err != nil {
		return nil, fmt.Errorf("scan profile row: %w", err)
	}
	p.Role = core.AgentRole(role)
	p.Session.IdleTTL = time.Duration(idleTTLMs) * time.Millisecond
	unmarshalNullJSON(caps, &p.Capabilities)
	unmarshalNullJSON(actions, &p.ActionsAllowed)
	unmarshalNullJSON(mcpTools, &p.MCP.Tools)
	unmarshalNullJSON(skills, &p.Skills)
	return p, nil
}

// UpsertDriver inserts or replaces a driver (used for seeding from config).
func (s *Store) UpsertDriver(ctx context.Context, d *core.AgentDriver) error {
	args, _ := marshalJSON(d.LaunchArgs)
	env, _ := marshalJSON(d.Env)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_drivers (id, launch_command, launch_args, env,
		        cap_fs_read, cap_fs_write, cap_terminal, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		    launch_command = excluded.launch_command,
		    launch_args = excluded.launch_args,
		    env = excluded.env,
		    cap_fs_read = excluded.cap_fs_read,
		    cap_fs_write = excluded.cap_fs_write,
		    cap_terminal = excluded.cap_terminal,
		    updated_at = excluded.updated_at`,
		d.ID, d.LaunchCommand, args, env,
		d.CapabilitiesMax.FSRead, d.CapabilitiesMax.FSWrite, d.CapabilitiesMax.Terminal,
		now, now)
	return err
}

// UpsertProfile inserts or replaces a profile (used for seeding from config).
func (s *Store) UpsertProfile(ctx context.Context, p *core.AgentProfile) error {
	if err := s.validateProfile(ctx, p); err != nil {
		return err
	}
	caps, _ := marshalJSON(p.Capabilities)
	actions, _ := marshalJSON(p.ActionsAllowed)
	mcpTools, _ := marshalJSON(p.MCP.Tools)
	skills, _ := marshalJSON(p.Skills)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_profiles (id, name, driver_id, role, capabilities, actions_allowed,
		        prompt_template, skills, session_reuse, session_max_turns, session_idle_ttl_ms,
		        mcp_enabled, mcp_tools, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		    name = excluded.name,
		    driver_id = excluded.driver_id,
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
		p.ID, p.Name, p.DriverID, string(p.Role),
		caps, actions, p.PromptTemplate, skills,
		p.Session.Reuse, p.Session.MaxTurns, p.Session.IdleTTL.Milliseconds(),
		p.MCP.Enabled, mcpTools, now, now)
	return err
}
