package model

type QualityEvent struct {
	QualityEventID  uint64 `gorm:"column:quality_event_id;primaryKey;autoIncrement"`
	IssueID         uint64 `gorm:"column:issue_id;not null;index"`
	IdempotencyKey  string `gorm:"column:idempotency_key;type:text;not null;uniqueIndex"`
	Source          string `gorm:"column:source;type:text;not null"`
	ExternalEventID string `gorm:"column:external_event_id;type:text;not null"`
	Category        string `gorm:"column:category;type:text;not null"`
	Result          string `gorm:"column:result;type:text;not null"`
	Actor           string `gorm:"column:actor;type:text;not null"`
	Summary         string `gorm:"column:summary;type:text;not null"`
	EvidenceJSON    string `gorm:"column:evidence_json;type:text;not null"`
	PayloadJSON     string `gorm:"column:payload_json;type:text;not null"`
	IngestedAt      string `gorm:"column:ingested_at;type:text;not null;index"`
}

func (QualityEvent) TableName() string {
	return "quality_events"
}
