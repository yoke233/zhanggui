package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const schemaV1 = `
CREATE TABLE IF NOT EXISTS projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'general',
    description TEXT NOT NULL DEFAULT '',
    metadata TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS resource_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id),
    issue_id INTEGER,
    kind TEXT NOT NULL,
    uri TEXT NOT NULL,
    config TEXT,
    label TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_rb_project ON resource_bindings(project_id);

CREATE TABLE IF NOT EXISTS issues (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id          INTEGER,
    resource_binding_id INTEGER,
    title               TEXT    NOT NULL,
    body                TEXT    NOT NULL DEFAULT '',
    status              TEXT    NOT NULL DEFAULT 'open',
    priority            TEXT    NOT NULL DEFAULT 'medium',
    labels              TEXT,
    depends_on          TEXT,
    metadata            TEXT,
    archived_at         DATETIME,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id INTEGER NOT NULL REFERENCES issues(id),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    position INTEGER NOT NULL DEFAULT 0,
    agent_role TEXT,
    required_capabilities TEXT,
    acceptance_criteria TEXT,
    timeout_ms INTEGER DEFAULT 0,
    config TEXT,
    max_retries INTEGER DEFAULT 0,
    retry_count INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    step_id INTEGER NOT NULL REFERENCES steps(id),
    issue_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'created',
    agent_id TEXT,
    agent_context_id INTEGER,
    briefing_snapshot TEXT,
    input TEXT,
    output TEXT,
    error_message TEXT,
    error_kind TEXT,
    attempt INTEGER DEFAULT 1,
    started_at DATETIME,
    finished_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    result_markdown TEXT NOT NULL DEFAULT '',
    result_metadata TEXT,
    result_assets TEXT
);

CREATE TABLE IF NOT EXISTS agent_contexts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT NOT NULL,
    issue_id INTEGER NOT NULL,
    system_prompt TEXT,
    session_id TEXT,
    summary TEXT,
    turn_count INTEGER DEFAULT 0,
    worker_id TEXT NOT NULL DEFAULT '',
    worker_last_seen_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS event_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'domain',
    issue_id INTEGER,
    step_id INTEGER,
    exec_id INTEGER,
    data TEXT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_drivers (
    id TEXT PRIMARY KEY,
    launch_command TEXT NOT NULL,
    launch_args TEXT,
    env TEXT,
    cap_fs_read INTEGER NOT NULL DEFAULT 0,
    cap_fs_write INTEGER NOT NULL DEFAULT 0,
    cap_terminal INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

 CREATE TABLE IF NOT EXISTS agent_profiles (
     id TEXT PRIMARY KEY,
     name TEXT NOT NULL DEFAULT '',
     driver_id TEXT NOT NULL REFERENCES agent_drivers(id),
     role TEXT NOT NULL,
     capabilities TEXT,
     actions_allowed TEXT,
     prompt_template TEXT NOT NULL DEFAULT '',
     skills TEXT,
     session_reuse INTEGER NOT NULL DEFAULT 0,
     session_max_turns INTEGER NOT NULL DEFAULT 0,
     session_idle_ttl_ms INTEGER NOT NULL DEFAULT 0,
     mcp_enabled INTEGER NOT NULL DEFAULT 0,
     mcp_tools TEXT,
     created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
     updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_issues_project ON issues(project_id);
CREATE INDEX IF NOT EXISTS idx_issues_status  ON issues(status);
CREATE INDEX IF NOT EXISTS idx_steps_issue ON steps(issue_id);
CREATE INDEX IF NOT EXISTS idx_executions_step ON executions(step_id);
CREATE INDEX IF NOT EXISTS idx_event_log_issue ON event_log(issue_id);
CREATE INDEX IF NOT EXISTS idx_event_log_category ON event_log(category);
CREATE INDEX IF NOT EXISTS idx_event_log_exec_category ON event_log(exec_id, category);
CREATE INDEX IF NOT EXISTS idx_agent_contexts_lookup ON agent_contexts(agent_id, issue_id);
`

func runMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("nil db")
	}

	if _, err := db.ExecContext(ctx, schemaV1); err != nil {
		return fmt.Errorf("run base schema: %w", err)
	}

	// Backward-compatible upgrades for existing DBs that were created before columns/tables existed.
	// SQLite doesn't support ADD COLUMN IF NOT EXISTS in older versions; ignore duplicate-column errors.
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS projects (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            kind TEXT NOT NULL DEFAULT 'general',
            description TEXT NOT NULL DEFAULT '',
            metadata TEXT,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		// Upgrade old projects table: add new columns (ignore if already present).
		`ALTER TABLE projects ADD COLUMN kind TEXT NOT NULL DEFAULT 'general'`,
		`ALTER TABLE projects ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE projects ADD COLUMN metadata TEXT`,
		// resource_bindings table is handled by CREATE TABLE IF NOT EXISTS in schemaV1.
		// Add description column to steps (runtime schema upgrade).
		`ALTER TABLE steps ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		// Add skills column to agent_profiles (runtime profile skills).
		`ALTER TABLE agent_profiles ADD COLUMN skills TEXT`,
		`ALTER TABLE agent_contexts ADD COLUMN worker_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agent_contexts ADD COLUMN worker_last_seen_at DATETIME`,
		// Issue-centric migration: add new Issue columns.
		`ALTER TABLE issues ADD COLUMN resource_binding_id INTEGER`,
		`ALTER TABLE issues ADD COLUMN depends_on TEXT`,
		`ALTER TABLE issues ADD COLUMN archived_at DATETIME`,
		// Step migration: add position column.
		`ALTER TABLE steps ADD COLUMN position INTEGER NOT NULL DEFAULT 0`,
		// Step migration: add depends_on column (JSON array of predecessor step IDs).
		`ALTER TABLE steps ADD COLUMN depends_on TEXT`,
		// Step migration: add input column (assembled run input text).
		`ALTER TABLE steps ADD COLUMN input TEXT`,
		// Column renames (SQLite 3.25+): flow_id → issue_id across tables.
		`ALTER TABLE steps RENAME COLUMN flow_id TO issue_id`,
		`ALTER TABLE executions RENAME COLUMN flow_id TO issue_id`,
		`ALTER TABLE artifacts RENAME COLUMN flow_id TO issue_id`,
		`ALTER TABLE agent_contexts RENAME COLUMN flow_id TO issue_id`,
		`ALTER TABLE events RENAME COLUMN flow_id TO issue_id`,
		`ALTER TABLE event_log RENAME COLUMN flow_id TO issue_id`,
		// Updated indexes for the renamed columns.
		`CREATE INDEX IF NOT EXISTS idx_steps_issue ON steps(issue_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_log_issue ON event_log(issue_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_contexts_lookup ON agent_contexts(agent_id, issue_id)`,
		// dag_templates table (reusable DAG template storage).
		`CREATE TABLE IF NOT EXISTS dag_templates (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            project_id INTEGER,
            tags TEXT,
            metadata TEXT,
            steps TEXT NOT NULL DEFAULT '[]',
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_dag_templates_project ON dag_templates(project_id)`,
		// usage_records table (token usage tracking per execution).
		`CREATE TABLE IF NOT EXISTS usage_records (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            execution_id INTEGER NOT NULL REFERENCES executions(id),
            issue_id INTEGER NOT NULL,
            step_id INTEGER NOT NULL,
            project_id INTEGER,
            agent_id TEXT NOT NULL DEFAULT '',
            profile_id TEXT NOT NULL DEFAULT '',
            model_id TEXT NOT NULL DEFAULT '',
            input_tokens INTEGER NOT NULL DEFAULT 0,
            output_tokens INTEGER NOT NULL DEFAULT 0,
            cache_read_tokens INTEGER NOT NULL DEFAULT 0,
            cache_write_tokens INTEGER NOT NULL DEFAULT 0,
            reasoning_tokens INTEGER NOT NULL DEFAULT 0,
            total_tokens INTEGER NOT NULL DEFAULT 0,
            duration_ms INTEGER NOT NULL DEFAULT 0,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_records_execution ON usage_records(execution_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_project ON usage_records(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_agent ON usage_records(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_profile ON usage_records(profile_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_created ON usage_records(created_at)`,
		`ALTER TABLE usage_records RENAME COLUMN flow_id TO issue_id`,
		// events → event_log rename and category column.
		`ALTER TABLE events RENAME TO event_log`,
		`ALTER TABLE event_log ADD COLUMN category TEXT NOT NULL DEFAULT 'domain'`,
		`CREATE INDEX IF NOT EXISTS idx_event_log_category ON event_log(category)`,
		`CREATE INDEX IF NOT EXISTS idx_event_log_exec_category ON event_log(exec_id, category)`,
		// threads table (independent multi-participant discussion container).
		`CREATE TABLE IF NOT EXISTS threads (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT NOT NULL,
            status TEXT NOT NULL DEFAULT 'active',
            owner_id TEXT NOT NULL DEFAULT '',
            summary TEXT NOT NULL DEFAULT '',
            metadata TEXT,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_threads_status ON threads(status)`,
		// thread_messages table.
		`CREATE TABLE IF NOT EXISTS thread_messages (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            thread_id INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
            sender_id TEXT NOT NULL DEFAULT '',
            role TEXT NOT NULL DEFAULT 'human',
            content TEXT NOT NULL DEFAULT '',
            reply_to_msg_id INTEGER,
            metadata TEXT,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_thread_messages_thread ON thread_messages(thread_id, id)`,
		`ALTER TABLE thread_messages ADD COLUMN reply_to_msg_id INTEGER`,
		// thread_participants table.
		`CREATE TABLE IF NOT EXISTS thread_participants (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            thread_id INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
            user_id TEXT NOT NULL,
            role TEXT NOT NULL DEFAULT 'member',
            joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(thread_id, user_id)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_thread_participants_thread ON thread_participants(thread_id)`,
		// thread_work_item_links table.
		`CREATE TABLE IF NOT EXISTS thread_work_item_links (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            thread_id INTEGER NOT NULL REFERENCES threads(id),
            work_item_id INTEGER NOT NULL REFERENCES issues(id),
            relation_type TEXT NOT NULL DEFAULT 'related',
            is_primary INTEGER NOT NULL DEFAULT 0,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(thread_id, work_item_id)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_twil_thread ON thread_work_item_links(thread_id)`,
		`CREATE INDEX IF NOT EXISTS idx_twil_work_item ON thread_work_item_links(work_item_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_twil_primary_thread ON thread_work_item_links(thread_id) WHERE is_primary = 1`,
		// thread_agent_sessions table.
		`CREATE TABLE IF NOT EXISTS thread_agent_sessions (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            thread_id INTEGER NOT NULL REFERENCES threads(id),
            agent_profile_id TEXT NOT NULL,
            acp_session_id TEXT NOT NULL DEFAULT '',
            status TEXT NOT NULL DEFAULT 'joining',
            joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            last_active_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(thread_id, agent_profile_id)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_tas_thread ON thread_agent_sessions(thread_id)`,
		// Thread agent session runtime fields.
		`ALTER TABLE thread_agent_sessions ADD COLUMN turn_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE thread_agent_sessions ADD COLUMN total_input_tokens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE thread_agent_sessions ADD COLUMN total_output_tokens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE thread_agent_sessions ADD COLUMN progress_summary TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE thread_agent_sessions ADD COLUMN metadata TEXT`,
		// feature_manifests table (project-level feature checklist).
		`CREATE TABLE IF NOT EXISTS feature_manifests (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            project_id INTEGER NOT NULL REFERENCES projects(id),
            version INTEGER NOT NULL DEFAULT 1,
            summary TEXT NOT NULL DEFAULT '',
            metadata TEXT,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_feature_manifests_project ON feature_manifests(project_id)`,
		// feature_entries table (individual feature/scenario entries).
		`CREATE TABLE IF NOT EXISTS feature_entries (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            manifest_id INTEGER NOT NULL REFERENCES feature_manifests(id) ON DELETE CASCADE,
            key TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            status TEXT NOT NULL DEFAULT 'pending',
            issue_id INTEGER,
            step_id INTEGER,
            tags TEXT,
            metadata TEXT,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_feature_entries_key ON feature_entries(manifest_id, key)`,
		`CREATE INDEX IF NOT EXISTS idx_feature_entries_manifest ON feature_entries(manifest_id)`,
		`CREATE INDEX IF NOT EXISTS idx_feature_entries_status ON feature_entries(manifest_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_feature_entries_issue ON feature_entries(issue_id)`,
		// action_signals table (explicit agent/human declarations about step outcomes).
		`CREATE TABLE IF NOT EXISTS action_signals (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            step_id INTEGER NOT NULL REFERENCES steps(id),
            issue_id INTEGER NOT NULL,
            exec_id INTEGER,
            type TEXT NOT NULL,
            source TEXT NOT NULL,
            payload TEXT,
            actor TEXT NOT NULL DEFAULT '',
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_action_signals_step ON action_signals(step_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_action_signals_exec ON action_signals(exec_id)`,
		// Step signals: new columns for unified interaction records.
		`ALTER TABLE action_signals ADD COLUMN summary TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE action_signals ADD COLUMN content TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE action_signals ADD COLUMN source_step_id INTEGER`,
		`CREATE INDEX IF NOT EXISTS idx_action_signals_type ON action_signals(step_id, type)`,
		// work_item_attachments table (file attachments for issues).
		`CREATE TABLE IF NOT EXISTS work_item_attachments (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
            file_name TEXT NOT NULL,
            file_path TEXT NOT NULL,
            mime_type TEXT NOT NULL DEFAULT '',
            size INTEGER NOT NULL DEFAULT 0,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_work_item_attachments_issue ON work_item_attachments(issue_id)`,
		// notifications table (user-facing notification center).
		`CREATE TABLE IF NOT EXISTS notifications (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            level TEXT NOT NULL DEFAULT 'info',
            title TEXT NOT NULL,
            body TEXT NOT NULL DEFAULT '',
            category TEXT NOT NULL DEFAULT '',
            action_url TEXT NOT NULL DEFAULT '',
            project_id INTEGER,
            issue_id INTEGER,
            exec_id INTEGER,
            channels TEXT,
            read INTEGER NOT NULL DEFAULT 0,
            read_at DATETIME,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(read, id)`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_category ON notifications(category)`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_issue ON notifications(issue_id)`,

		// Add updated_at to resource_bindings (unified resource model).
		`ALTER TABLE resource_bindings ADD COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP`,

		// Action resources — per-action input/output resource declarations.
		// References resource_bindings directly (no separate locators table).
		`CREATE TABLE IF NOT EXISTS action_resources (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            action_id INTEGER NOT NULL REFERENCES steps(id),
            resource_binding_id INTEGER NOT NULL REFERENCES resource_bindings(id),
            direction TEXT NOT NULL,
            path TEXT NOT NULL,
            media_type TEXT NOT NULL DEFAULT '',
            description TEXT NOT NULL DEFAULT '',
            required INTEGER NOT NULL DEFAULT 0,
            metadata TEXT,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_action_resources_action ON action_resources(action_id)`,
		`CREATE INDEX IF NOT EXISTS idx_action_resources_binding ON action_resources(resource_binding_id)`,

		// Migration: rename locator_id → resource_binding_id if old schema exists.
		`ALTER TABLE action_resources RENAME COLUMN locator_id TO resource_binding_id`,

		// Drop legacy resource_locators table if it exists.
		`DROP TABLE IF EXISTS resource_locators`,

		// Schema simplification: drop legacy briefings table (replaced by steps.input).
		`DROP TABLE IF EXISTS briefings`,
		// Schema simplification: merge work_item_attachments into resource_bindings.
		`ALTER TABLE resource_bindings ADD COLUMN issue_id INTEGER`,
		`CREATE INDEX IF NOT EXISTS idx_rb_issue ON resource_bindings(issue_id)`,
		// Schema simplification: merge feature_manifests into feature_entries.
		`ALTER TABLE feature_entries ADD COLUMN project_id INTEGER`,
		// Schema simplification: merge thread_participants + thread_agent_sessions into thread_members.
		`CREATE TABLE IF NOT EXISTS thread_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			thread_id INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
			kind TEXT NOT NULL DEFAULT 'human',
			user_id TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'member',
			status TEXT NOT NULL DEFAULT '',
			agent_data TEXT,
			joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_active_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(thread_id, user_id, kind)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_thread_members_thread ON thread_members(thread_id)`,
		// Schema simplification: merge artifacts into executions (inline result fields).
		`ALTER TABLE executions ADD COLUMN result_markdown TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE executions ADD COLUMN result_metadata TEXT`,
		`ALTER TABLE executions ADD COLUMN result_assets TEXT`,
		// Schema simplification: merge agent_drivers into agent_profiles (embed driver config).
		`ALTER TABLE agent_profiles ADD COLUMN driver_config TEXT`,
		// inspection_reports table (self-evolving inspection system).
		`CREATE TABLE IF NOT EXISTS inspection_reports (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            project_id INTEGER,
            status TEXT NOT NULL DEFAULT 'pending',
            trigger_source TEXT NOT NULL DEFAULT 'manual',
            period_start DATETIME NOT NULL,
            period_end DATETIME NOT NULL,
            snapshot TEXT,
            summary TEXT NOT NULL DEFAULT '',
            suggested_skills TEXT,
            error_message TEXT NOT NULL DEFAULT '',
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            finished_at DATETIME
        )`,
		`CREATE INDEX IF NOT EXISTS idx_inspection_reports_project ON inspection_reports(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_inspection_reports_status ON inspection_reports(status)`,
		`CREATE INDEX IF NOT EXISTS idx_inspection_reports_created ON inspection_reports(created_at)`,
		// inspection_findings table.
		`CREATE TABLE IF NOT EXISTS inspection_findings (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            inspection_id INTEGER NOT NULL REFERENCES inspection_reports(id) ON DELETE CASCADE,
            category TEXT NOT NULL,
            severity TEXT NOT NULL,
            title TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            evidence TEXT NOT NULL DEFAULT '',
            work_item_id INTEGER,
            action_id INTEGER,
            run_id INTEGER,
            project_id INTEGER,
            recommendation TEXT NOT NULL DEFAULT '',
            recurring INTEGER NOT NULL DEFAULT 0,
            occurrence_count INTEGER NOT NULL DEFAULT 1,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_inspection_findings_inspection ON inspection_findings(inspection_id)`,
		`CREATE INDEX IF NOT EXISTS idx_inspection_findings_category ON inspection_findings(category)`,
		`CREATE INDEX IF NOT EXISTS idx_inspection_findings_severity ON inspection_findings(severity)`,
		// inspection_insights table.
		`CREATE TABLE IF NOT EXISTS inspection_insights (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            inspection_id INTEGER NOT NULL REFERENCES inspection_reports(id) ON DELETE CASCADE,
            type TEXT NOT NULL,
            title TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            trend TEXT NOT NULL DEFAULT '',
            action_items TEXT,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX IF NOT EXISTS idx_inspection_insights_inspection ON inspection_insights(inspection_id)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			// Some SQLite builds return a generic error string for duplicate columns or tables.
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			if strings.Contains(err.Error(), "no such table") && (strings.Contains(stmt, "RENAME COLUMN") || strings.Contains(stmt, "RENAME TO")) {
				continue
			}
			// RENAME COLUMN fails if column already has the new name.
			if strings.Contains(err.Error(), "no such column") && strings.Contains(stmt, "RENAME COLUMN") {
				continue
			}
			return fmt.Errorf("migration failed: %s: %w", stmt, err)
		}
	}

	// Migrate work_item_attachments → resource_bindings (idempotent).
	migrateAttachmentsToBindings(ctx, db)

	// Migrate feature_manifests → feature_entries (idempotent).
	migrateFeatureManifestsToEntries(ctx, db)

	// Merge thread_participants + thread_agent_sessions → thread_members (idempotent).
	migrateThreadMembersFromOldTables(ctx, db)

	// Merge artifacts → executions (inline result fields, idempotent).
	migrateArtifactsToExecutions(ctx, db)

	// Merge agent_drivers into agent_profiles (embed driver_config JSON, idempotent).
	migrateDriversToProfiles(ctx, db)

	// One-time data migration: convert legacy Config rework_history / blocked_type
	// into ActionSignal records. Idempotent — skips steps that already have signals.
	migrateConfigToSignals(ctx, db)

	// Merge execution_probes → action_signals (probe_request/probe_response signals, idempotent).
	migrateExecutionProbesToSignals(ctx, db)

	// Merge tool_call_audits → event_log (category='tool_audit', idempotent).
	migrateToolCallAuditsToEventLog(ctx, db)

	return nil
}

// migrateConfigToSignals scans steps with rework_history or blocked_type in Config
// and creates corresponding ActionSignal records. Idempotent: skips if signals exist.
func migrateConfigToSignals(ctx context.Context, db *sql.DB) {
	rows, err := db.QueryContext(ctx, `SELECT id, issue_id, config FROM steps WHERE config IS NOT NULL AND config != '' AND config != '{}'`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var stepID, issueID int64
		var configJSON string
		if err := rows.Scan(&stepID, &issueID, &configJSON); err != nil {
			continue
		}

		var config map[string]any
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			continue
		}

		// Check if this step already has feedback/context signals (idempotent guard).
		var existingCount int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM action_signals WHERE step_id = ? AND type IN ('feedback','context')`, stepID).Scan(&existingCount); err == nil && existingCount > 0 {
			continue
		}

		// Migrate rework_history entries → SignalFeedback.
		if history, ok := config["rework_history"].([]any); ok {
			for _, entry := range history {
				m, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				reason, _ := m["reason"].(string)
				if reason == "" {
					reason = "gate rejected"
				}
				gateStepID := int64(0)
				if v, ok := m["gate_step_id"].(float64); ok {
					gateStepID = int64(v)
				}
				createdAt := ""
				if v, ok := m["at"].(string); ok {
					createdAt = v
				}
				if createdAt == "" {
					createdAt = "2024-01-01T00:00:00Z"
				}

				var sourceStepID any = nil
				if gateStepID > 0 {
					sourceStepID = gateStepID
				}

				// Serialize only the entry itself as payload, not the full config.
				entryJSON, _ := json.Marshal(m)

				_, _ = db.ExecContext(ctx,
					`INSERT INTO action_signals (step_id, issue_id, type, source, summary, content, source_step_id, payload, actor, created_at) VALUES (?, ?, 'feedback', 'system', ?, ?, ?, ?, 'gate', ?)`,
					stepID, issueID, reason, reason, sourceStepID, string(entryJSON), createdAt,
				)
			}
		}

		// Migrate blocked_type=merge_conflict → SignalContext.
		if bt, ok := config["blocked_type"].(string); ok && bt == "merge_conflict" {
			blockedReason, _ := config["blocked_reason"].(string)
			if blockedReason == "" {
				blockedReason = "merge_conflict"
			}
			blockedAt, _ := config["blocked_at"].(string)
			if blockedAt == "" {
				blockedAt = "2024-01-01T00:00:00Z"
			}
			// Extract only blocked-related fields as payload.
			blockedPayload := map[string]any{
				"blocked_type":   bt,
				"blocked_reason": blockedReason,
			}
			if v, ok := config["merge_error"]; ok {
				blockedPayload["merge_error"] = v
			}
			if v, ok := config["mergeable_state"]; ok {
				blockedPayload["mergeable_state"] = v
			}
			if v, ok := config["pr_number"]; ok {
				blockedPayload["pr_number"] = v
			}
			if v, ok := config["pr_url"]; ok {
				blockedPayload["pr_url"] = v
			}
			payloadJSON, _ := json.Marshal(blockedPayload)
			_, _ = db.ExecContext(ctx,
				`INSERT INTO action_signals (step_id, issue_id, type, source, summary, content, payload, actor, created_at) VALUES (?, ?, 'context', 'system', 'merge_conflict', ?, ?, 'system', ?)`,
				stepID, issueID, blockedReason, string(payloadJSON), blockedAt,
			)
		}
	}
}

// migrateAttachmentsToBindings moves rows from work_item_attachments into resource_bindings
// with kind='attachment', then drops the old table. Idempotent.
func migrateAttachmentsToBindings(ctx context.Context, db *sql.DB) {
	// Check if old table still exists.
	var tableName string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='work_item_attachments'`).Scan(&tableName)
	if err != nil {
		return // table already gone
	}

	_, _ = db.ExecContext(ctx, `
		INSERT INTO resource_bindings (issue_id, project_id, kind, uri, label, config, created_at)
		SELECT
			wa.issue_id,
			COALESCE(i.project_id, 0),
			'attachment',
			wa.file_path,
			wa.file_name,
			json_object('mime_type', wa.mime_type, 'size', wa.size),
			wa.created_at
		FROM work_item_attachments wa
		LEFT JOIN issues i ON i.id = wa.issue_id
		WHERE NOT EXISTS (
			SELECT 1 FROM resource_bindings rb
			WHERE rb.issue_id = wa.issue_id AND rb.kind = 'attachment' AND rb.uri = wa.file_path
		)
	`)

	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS work_item_attachments`)
}

// migrateFeatureManifestsToEntries copies project_id from feature_manifests into
// feature_entries, then drops the feature_manifests table. Idempotent.
func migrateFeatureManifestsToEntries(ctx context.Context, db *sql.DB) {
	// Check if old table still exists.
	var tableName string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='feature_manifests'`).Scan(&tableName)
	if err != nil {
		return // table already gone
	}

	// Backfill project_id on feature_entries from feature_manifests.
	_, _ = db.ExecContext(ctx, `
		UPDATE feature_entries
		SET project_id = (
			SELECT fm.project_id FROM feature_manifests fm WHERE fm.id = feature_entries.manifest_id
		)
		WHERE project_id IS NULL
	`)

	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS feature_manifests`)
}

// migrateThreadMembersFromOldTables copies data from thread_participants and
// thread_agent_sessions into thread_members, then drops both old tables.
// Idempotent: skips if thread_participants table is already gone.
func migrateThreadMembersFromOldTables(ctx context.Context, db *sql.DB) {
	var tableName string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='thread_participants'`).Scan(&tableName)
	if err != nil {
		return // table already gone — migration already applied
	}

	// Insert human members from thread_participants.
	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO thread_members (thread_id, kind, user_id, role, joined_at, last_active_at)
		SELECT thread_id, 'human', user_id, role, joined_at, joined_at
		FROM thread_participants
	`)

	// Insert agent members from thread_agent_sessions (if table exists).
	var agentTable string
	if db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='thread_agent_sessions'`).Scan(&agentTable) == nil {
		_, _ = db.ExecContext(ctx, `
			INSERT OR IGNORE INTO thread_members (thread_id, kind, user_id, agent_profile_id, role, status, agent_data, joined_at, last_active_at)
			SELECT
				thread_id,
				'agent',
				agent_profile_id,
				agent_profile_id,
				'agent',
				status,
				json_object(
					'acp_session_id', COALESCE(acp_session_id, ''),
					'turn_count', COALESCE(turn_count, 0),
					'total_input_tokens', COALESCE(total_input_tokens, 0),
					'total_output_tokens', COALESCE(total_output_tokens, 0),
					'progress_summary', COALESCE(progress_summary, ''),
					'metadata', COALESCE(metadata, '{}')
				),
				joined_at,
				last_active_at
			FROM thread_agent_sessions
		`)
		_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS thread_agent_sessions`)
	}

	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS thread_participants`)
}

// migrateArtifactsToExecutions copies result data from artifacts into executions,
// then drops the artifacts table. Idempotent.
func migrateArtifactsToExecutions(ctx context.Context, db *sql.DB) {
	var tableName string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='artifacts'`).Scan(&tableName)
	if err != nil {
		return // table already gone
	}

	// Backfill result fields on executions from the latest artifact per execution.
	_, _ = db.ExecContext(ctx, `
		UPDATE executions
		SET result_markdown = (
			SELECT a.result_markdown FROM artifacts a WHERE a.execution_id = executions.id ORDER BY a.id DESC LIMIT 1
		),
		result_metadata = (
			SELECT a.metadata FROM artifacts a WHERE a.execution_id = executions.id ORDER BY a.id DESC LIMIT 1
		),
		result_assets = (
			SELECT a.assets FROM artifacts a WHERE a.execution_id = executions.id ORDER BY a.id DESC LIMIT 1
		)
		WHERE result_markdown = '' AND EXISTS (
			SELECT 1 FROM artifacts a WHERE a.execution_id = executions.id
		)
	`)

	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS artifacts`)
}

// migrateDriversToProfiles copies driver config from agent_drivers into agent_profiles.driver_config
// as a JSON blob, then drops the agent_drivers table. Idempotent.
func migrateDriversToProfiles(ctx context.Context, db *sql.DB) {
	var tableName string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='agent_drivers'`).Scan(&tableName)
	if err != nil {
		return // table already gone
	}

	// For each profile that has a driver_id, build the driver_config JSON from agent_drivers.
	_, _ = db.ExecContext(ctx, `
		UPDATE agent_profiles
		SET driver_config = (
			SELECT json_object(
				'launch_command', d.launch_command,
				'launch_args', CASE WHEN d.launch_args IS NOT NULL THEN json(d.launch_args) ELSE json('[]') END,
				'env', CASE WHEN d.env IS NOT NULL THEN json(d.env) ELSE json('{}') END,
				'capabilities_max', json_object(
					'fs_read', CASE WHEN d.cap_fs_read THEN json('true') ELSE json('false') END,
					'fs_write', CASE WHEN d.cap_fs_write THEN json('true') ELSE json('false') END,
					'terminal', CASE WHEN d.cap_terminal THEN json('true') ELSE json('false') END
				)
			)
			FROM agent_drivers d
			WHERE d.id = agent_profiles.driver_id
		)
		WHERE driver_config IS NULL AND driver_id IS NOT NULL AND driver_id != ''
	`)

	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS agent_drivers`)
}

// migrateExecutionProbesToSignals moves rows from execution_probes into action_signals
// as probe_request signals, then drops the old table. Idempotent.
func migrateExecutionProbesToSignals(ctx context.Context, db *sql.DB) {
	var tableName string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='execution_probes'`).Scan(&tableName)
	if err != nil {
		return // table already gone — migration already applied
	}

	// Insert each execution_probe as a probe_request or probe_response signal in action_signals.
	// The probe's full state is serialized into the payload JSON column.
	_, _ = db.ExecContext(ctx, `
		INSERT INTO action_signals (step_id, issue_id, exec_id, type, source, summary, content, payload, actor, created_at)
		SELECT
			ep.step_id,
			ep.issue_id,
			ep.execution_id,
			CASE WHEN ep.status IN ('answered', 'timeout', 'unreachable', 'failed') THEN 'probe_response' ELSE 'probe_request' END,
			'system',
			ep.question,
			COALESCE(ep.reply_text, ep.question),
			json_object(
				'trigger_source', ep.trigger_source,
				'question', ep.question,
				'status', ep.status,
				'verdict', ep.verdict,
				'session_id', ep.session_id,
				'owner_id', ep.owner_id,
				'reply_text', COALESCE(ep.reply_text, ''),
				'error', COALESCE(ep.error, ''),
				'agent_context_id', ep.agent_context_id,
				'sent_at', ep.sent_at,
				'answered_at', ep.answered_at
			),
			'system',
			ep.created_at
		FROM execution_probes ep
		WHERE NOT EXISTS (
			SELECT 1 FROM action_signals a
			WHERE a.exec_id = ep.execution_id
			  AND a.type IN ('probe_request', 'probe_response')
			  AND a.created_at = ep.created_at
		)
	`)

	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS execution_probes`)
}

// migrateToolCallAuditsToEventLog moves rows from tool_call_audits into event_log
// with category='tool_audit', then drops the old table. Idempotent.
func migrateToolCallAuditsToEventLog(ctx context.Context, db *sql.DB) {
	var tableName string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='tool_call_audits'`).Scan(&tableName)
	if err != nil {
		return // table already gone
	}

	_, _ = db.ExecContext(ctx, `
		INSERT INTO event_log (type, category, issue_id, step_id, exec_id, data, timestamp)
		SELECT
			'tool_call_audit',
			'tool_audit',
			issue_id,
			step_id,
			execution_id,
			json_object(
				'session_id', COALESCE(session_id, ''),
				'tool_call_id', COALESCE(tool_call_id, ''),
				'tool_name', COALESCE(tool_name, ''),
				'status', COALESCE(status, ''),
				'duration_ms', COALESCE(duration_ms, 0),
				'input_digest', COALESCE(input_digest, ''),
				'output_digest', COALESCE(output_digest, ''),
				'stdout_digest', COALESCE(stdout_digest, ''),
				'stderr_digest', COALESCE(stderr_digest, ''),
				'input_preview', COALESCE(input_preview, ''),
				'output_preview', COALESCE(output_preview, ''),
				'stdout_preview', COALESCE(stdout_preview, ''),
				'stderr_preview', COALESCE(stderr_preview, ''),
				'redaction_level', COALESCE(redaction_level, ''),
				'started_at', COALESCE(started_at, ''),
				'finished_at', COALESCE(finished_at, ''),
				'exit_code', exit_code
			),
			created_at
		FROM tool_call_audits
		WHERE NOT EXISTS (
			SELECT 1 FROM event_log el
			WHERE el.exec_id = tool_call_audits.execution_id
			  AND el.category = 'tool_audit'
			  AND el.timestamp = tool_call_audits.created_at
		)
	`)

	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS tool_call_audits`)
}
