package sqlite

import (
	"context"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

// -- GORM models --

type InspectionReportModel struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID   *int64    `gorm:"column:project_id"`
	Status      string    `gorm:"column:status;not null"`
	Trigger     string    `gorm:"column:trigger_source;not null"`
	PeriodStart time.Time `gorm:"column:period_start;not null"`
	PeriodEnd   time.Time `gorm:"column:period_end;not null"`
	Snapshot    JSONField[*core.InspectionSnapshot] `gorm:"column:snapshot;type:text"`
	Summary     string    `gorm:"column:summary;not null;default:''"`
	SuggestedSkills JSONField[[]core.SuggestedSkill] `gorm:"column:suggested_skills;type:text"`
	ErrorMessage string   `gorm:"column:error_message;not null;default:''"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	FinishedAt  *time.Time `gorm:"column:finished_at"`
}

func (InspectionReportModel) TableName() string { return "inspection_reports" }

type InspectionFindingModel struct {
	ID              int64  `gorm:"column:id;primaryKey;autoIncrement"`
	InspectionID    int64  `gorm:"column:inspection_id;not null"`
	Category        string `gorm:"column:category;not null"`
	Severity        string `gorm:"column:severity;not null"`
	Title           string `gorm:"column:title;not null"`
	Description     string `gorm:"column:description;not null;default:''"`
	Evidence        string `gorm:"column:evidence;not null;default:''"`
	WorkItemID      *int64 `gorm:"column:work_item_id"`
	ActionID        *int64 `gorm:"column:action_id"`
	RunID           *int64 `gorm:"column:run_id"`
	ProjectID       *int64 `gorm:"column:project_id"`
	Recommendation  string `gorm:"column:recommendation;not null;default:''"`
	Recurring       bool   `gorm:"column:recurring;not null;default:false"`
	OccurrenceCount int    `gorm:"column:occurrence_count;not null;default:1"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (InspectionFindingModel) TableName() string { return "inspection_findings" }

type InspectionInsightModel struct {
	ID           int64  `gorm:"column:id;primaryKey;autoIncrement"`
	InspectionID int64  `gorm:"column:inspection_id;not null"`
	Type         string `gorm:"column:type;not null"`
	Title        string `gorm:"column:title;not null"`
	Description  string `gorm:"column:description;not null;default:''"`
	Trend        string `gorm:"column:trend;not null;default:''"`
	ActionItems  JSONField[[]string] `gorm:"column:action_items;type:text"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (InspectionInsightModel) TableName() string { return "inspection_insights" }

// -- Conversions --

func toInspectionReport(m *InspectionReportModel) *core.InspectionReport {
	return &core.InspectionReport{
		ID:              m.ID,
		ProjectID:       m.ProjectID,
		Status:          core.InspectionStatus(m.Status),
		Trigger:         core.InspectionTrigger(m.Trigger),
		PeriodStart:     m.PeriodStart,
		PeriodEnd:       m.PeriodEnd,
		Snapshot:        m.Snapshot.Data,
		Summary:         m.Summary,
		SuggestedSkills: m.SuggestedSkills.Data,
		ErrorMessage:    m.ErrorMessage,
		CreatedAt:       m.CreatedAt,
		FinishedAt:      m.FinishedAt,
	}
}

func fromInspectionReport(r *core.InspectionReport) *InspectionReportModel {
	return &InspectionReportModel{
		ID:              r.ID,
		ProjectID:       r.ProjectID,
		Status:          string(r.Status),
		Trigger:         string(r.Trigger),
		PeriodStart:     r.PeriodStart,
		PeriodEnd:       r.PeriodEnd,
		Snapshot:        JSONField[*core.InspectionSnapshot]{Data: r.Snapshot},
		Summary:         r.Summary,
		SuggestedSkills: JSONField[[]core.SuggestedSkill]{Data: r.SuggestedSkills},
		ErrorMessage:    r.ErrorMessage,
		CreatedAt:       r.CreatedAt,
		FinishedAt:      r.FinishedAt,
	}
}

func toInspectionFinding(m *InspectionFindingModel) *core.InspectionFinding {
	return &core.InspectionFinding{
		ID:              m.ID,
		InspectionID:    m.InspectionID,
		Category:        core.FindingCategory(m.Category),
		Severity:        core.FindingSeverity(m.Severity),
		Title:           m.Title,
		Description:     m.Description,
		Evidence:        m.Evidence,
		WorkItemID:      m.WorkItemID,
		ActionID:        m.ActionID,
		RunID:           m.RunID,
		ProjectID:       m.ProjectID,
		Recommendation:  m.Recommendation,
		Recurring:       m.Recurring,
		OccurrenceCount: m.OccurrenceCount,
		CreatedAt:       m.CreatedAt,
	}
}

func fromInspectionFinding(f *core.InspectionFinding) *InspectionFindingModel {
	return &InspectionFindingModel{
		ID:              f.ID,
		InspectionID:    f.InspectionID,
		Category:        string(f.Category),
		Severity:        string(f.Severity),
		Title:           f.Title,
		Description:     f.Description,
		Evidence:        f.Evidence,
		WorkItemID:      f.WorkItemID,
		ActionID:        f.ActionID,
		RunID:           f.RunID,
		ProjectID:       f.ProjectID,
		Recommendation:  f.Recommendation,
		Recurring:       f.Recurring,
		OccurrenceCount: f.OccurrenceCount,
		CreatedAt:       f.CreatedAt,
	}
}

func toInspectionInsight(m *InspectionInsightModel) *core.InspectionInsight {
	return &core.InspectionInsight{
		ID:           m.ID,
		InspectionID: m.InspectionID,
		Type:         m.Type,
		Title:        m.Title,
		Description:  m.Description,
		Trend:        m.Trend,
		ActionItems:  m.ActionItems.Data,
		CreatedAt:    m.CreatedAt,
	}
}

func fromInspectionInsight(i *core.InspectionInsight) *InspectionInsightModel {
	return &InspectionInsightModel{
		ID:           i.ID,
		InspectionID: i.InspectionID,
		Type:         i.Type,
		Title:        i.Title,
		Description:  i.Description,
		Trend:        i.Trend,
		ActionItems:  JSONField[[]string]{Data: i.ActionItems},
		CreatedAt:    i.CreatedAt,
	}
}

// -- Store methods --

func (s *Store) CreateInspection(ctx context.Context, report *core.InspectionReport) (int64, error) {
	m := fromInspectionReport(report)
	if err := s.orm.WithContext(ctx).Create(m).Error; err != nil {
		return 0, err
	}
	report.ID = m.ID
	return m.ID, nil
}

func (s *Store) GetInspection(ctx context.Context, id int64) (*core.InspectionReport, error) {
	var m InspectionReportModel
	if err := s.orm.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	report := toInspectionReport(&m)

	// Load findings and insights.
	findings, err := s.ListFindingsByInspection(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, f := range findings {
		report.Findings = append(report.Findings, *f)
	}

	insights, err := s.ListInsightsByInspection(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, i := range insights {
		report.Insights = append(report.Insights, *i)
	}

	return report, nil
}

func (s *Store) ListInspections(ctx context.Context, filter core.InspectionFilter) ([]*core.InspectionReport, error) {
	q := s.orm.WithContext(ctx).Model(&InspectionReportModel{})
	if filter.ProjectID != nil {
		q = q.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.Status != nil {
		q = q.Where("status = ?", string(*filter.Status))
	}
	if filter.Since != nil {
		q = q.Where("created_at >= ?", *filter.Since)
	}
	if filter.Until != nil {
		q = q.Where("created_at <= ?", *filter.Until)
	}
	q = q.Order("id DESC")
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	} else {
		q = q.Limit(50)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}

	var models []InspectionReportModel
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	reports := make([]*core.InspectionReport, 0, len(models))
	for i := range models {
		reports = append(reports, toInspectionReport(&models[i]))
	}
	return reports, nil
}

func (s *Store) UpdateInspection(ctx context.Context, report *core.InspectionReport) error {
	m := fromInspectionReport(report)
	return s.orm.WithContext(ctx).Save(m).Error
}

func (s *Store) CreateFinding(ctx context.Context, finding *core.InspectionFinding) (int64, error) {
	m := fromInspectionFinding(finding)
	if err := s.orm.WithContext(ctx).Create(m).Error; err != nil {
		return 0, err
	}
	finding.ID = m.ID
	return m.ID, nil
}

func (s *Store) ListFindingsByInspection(ctx context.Context, inspectionID int64) ([]*core.InspectionFinding, error) {
	var models []InspectionFindingModel
	if err := s.orm.WithContext(ctx).Where("inspection_id = ?", inspectionID).Order("severity ASC, id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	findings := make([]*core.InspectionFinding, 0, len(models))
	for i := range models {
		findings = append(findings, toInspectionFinding(&models[i]))
	}
	return findings, nil
}

func (s *Store) ListRecentFindings(ctx context.Context, category core.FindingCategory, limit int) ([]*core.InspectionFinding, error) {
	if limit <= 0 {
		limit = 20
	}
	var models []InspectionFindingModel
	q := s.orm.WithContext(ctx).Where("category = ?", string(category)).Order("id DESC").Limit(limit)
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	findings := make([]*core.InspectionFinding, 0, len(models))
	for i := range models {
		findings = append(findings, toInspectionFinding(&models[i]))
	}
	return findings, nil
}

func (s *Store) CreateInsight(ctx context.Context, insight *core.InspectionInsight) (int64, error) {
	m := fromInspectionInsight(insight)
	if err := s.orm.WithContext(ctx).Create(m).Error; err != nil {
		return 0, err
	}
	insight.ID = m.ID
	return m.ID, nil
}

func (s *Store) ListInsightsByInspection(ctx context.Context, inspectionID int64) ([]*core.InspectionInsight, error) {
	var models []InspectionInsightModel
	if err := s.orm.WithContext(ctx).Where("inspection_id = ?", inspectionID).Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	insights := make([]*core.InspectionInsight, 0, len(models))
	for i := range models {
		insights = append(insights, toInspectionInsight(&models[i]))
	}
	return insights, nil
}

func (s *Store) GetFindingRecurrenceCount(ctx context.Context, category core.FindingCategory, workItemID, actionID *int64) (int, error) {
	q := s.orm.WithContext(ctx).Model(&InspectionFindingModel{}).Where("category = ?", string(category))
	if workItemID != nil {
		q = q.Where("work_item_id = ?", *workItemID)
	}
	if actionID != nil {
		q = q.Where("action_id = ?", *actionID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}
