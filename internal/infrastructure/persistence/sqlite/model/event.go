package model

type Event struct {
	EventID   uint64 `gorm:"column:event_id;primaryKey;autoIncrement"`
	IssueID   uint64 `gorm:"column:issue_id;not null;index"`
	Actor     string `gorm:"column:actor;type:text;not null"`
	Body      string `gorm:"column:body;type:text;not null"`
	CreatedAt string `gorm:"column:created_at;type:text;not null"`
}

func (Event) TableName() string {
	return "events"
}
