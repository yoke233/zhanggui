package sqlite

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

func autoMigrate(ctx context.Context, orm *gorm.DB) error {
	if orm == nil {
		return fmt.Errorf("nil orm")
	}
	if err := orm.WithContext(ctx).AutoMigrate(
		&ProjectModel{},
		&ResourceSpaceModel{},
		&ResourceModel{},
		&WorkItemModel{},
		&ActionModel{},
		&RunModel{},
		&DeliverableModel{},
		&AgentContextModel{},
		&EventModel{},
		&AgentProfileModel{},
		&DAGTemplateModel{},
		&UsageRecordModel{},
		&ThreadModel{},
		&ThreadMessageModel{},
		&ThreadMemberModel{},
		&ThreadWorkItemLinkModel{},
		&InitiativeModel{},
		&InitiativeItemModel{},
		&ThreadInitiativeLinkModel{},
		&ThreadProposalModel{},
		&ThreadContextRefModel{},
		&ThreadAttachmentModel{},
		&FeatureEntryModel{},
		&ActionSignalModel{},
		&ActionIODeclModel{},
		&InspectionReportModel{},
		&InspectionFindingModel{},
		&InspectionInsightModel{},
		&NotificationModel{},
		&JournalModel{},
	); err != nil {
		return err
	}

	// Migrate legacy column data (idempotent, safe for fresh DBs).
	var colCount int64
	orm.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM pragma_table_info('work_items') WHERE name = 'resource_binding_id'`,
	).Scan(&colCount)
	if colCount > 0 {
		orm.WithContext(ctx).Exec(
			`UPDATE work_items SET resource_space_id = resource_binding_id WHERE resource_space_id IS NULL AND resource_binding_id IS NOT NULL`,
		)
	}

	// Create partial indexes for activity_journal (GORM AutoMigrate does not support SQLite partial indexes).
	for _, ddl := range []string{
		`CREATE INDEX IF NOT EXISTS idx_actions_work_item_position_id ON actions(work_item_id, position, id)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_action_attempt ON runs(action_id, attempt)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_status_id ON runs(status, id)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_action_result ON runs(action_id, id DESC) WHERE result_markdown IS NOT NULL AND result_markdown != ''`,
		`CREATE INDEX IF NOT EXISTS idx_deliverables_work_item_created_at ON deliverables(work_item_id, created_at DESC) WHERE work_item_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_deliverables_thread_created_at ON deliverables(thread_id, created_at DESC) WHERE thread_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_deliverables_producer ON deliverables(producer_type, producer_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_action_signals_action_id ON action_signals(action_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_thread_messages_thread_id ON thread_messages(thread_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_thread_members_thread_profile_status ON thread_members(thread_id, agent_profile_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_run_id ON usage_records(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_created_at ON usage_records(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_project_created_at ON usage_records(project_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_journal_run ON activity_journal(run_id, created_at) WHERE run_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_journal_action ON activity_journal(action_id, created_at) WHERE action_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_journal_work_item ON activity_journal(work_item_id, created_at) WHERE work_item_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_journal_kind ON activity_journal(kind, created_at)`,
	} {
		if err := orm.WithContext(ctx).Exec(ddl).Error; err != nil {
			return fmt.Errorf("create sqlite index: %w", err)
		}
	}
	return nil
}
