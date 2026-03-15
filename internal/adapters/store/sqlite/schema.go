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
		&ResourceBindingModel{},
		&ResourceSpaceModel{},
		&ResourceModel{},
		&WorkItemModel{},
		&ActionModel{},
		&RunModel{},
		&AgentContextModel{},
		&EventModel{},
		&AgentProfileModel{},
		&DAGTemplateModel{},
		&UsageRecordModel{},
		&ThreadModel{},
		&ThreadMessageModel{},
		&ThreadMemberModel{},
		&ThreadWorkItemLinkModel{},
		&ThreadContextRefModel{},
		&WorkItemTrackModel{},
		&WorkItemTrackThreadModel{},
		&FeatureEntryModel{},
		&ActionSignalModel{},
		&ActionResourceModel{},
		&ActionIODeclModel{},
		&InspectionReportModel{},
		&InspectionFindingModel{},
		&InspectionInsightModel{},
		&NotificationModel{},
		&JournalModel{},
	); err != nil {
		return err
	}

	// Create partial indexes for activity_journal (GORM AutoMigrate does not support SQLite partial indexes).
	for _, ddl := range []string{
		`CREATE INDEX IF NOT EXISTS idx_journal_run ON activity_journal(run_id, created_at) WHERE run_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_journal_action ON activity_journal(action_id, created_at) WHERE action_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_journal_work_item ON activity_journal(work_item_id, created_at) WHERE work_item_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_journal_kind ON activity_journal(kind, created_at)`,
	} {
		if err := orm.WithContext(ctx).Exec(ddl).Error; err != nil {
			return fmt.Errorf("create journal index: %w", err)
		}
	}
	if err := migrateUnifiedResources(ctx, orm); err != nil {
		return fmt.Errorf("migrate unified resources: %w", err)
	}
	return nil
}
