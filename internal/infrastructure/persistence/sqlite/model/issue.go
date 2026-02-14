package model

type Issue struct {
	IssueID   uint64  `gorm:"column:issue_id;primaryKey;autoIncrement"`
	Title     string  `gorm:"column:title;type:text;not null"`
	Body      string  `gorm:"column:body;type:text;not null"`
	Assignee  *string `gorm:"column:assignee;type:text"`
	IsClosed  bool    `gorm:"column:is_closed;not null;default:0"`
	CreatedAt string  `gorm:"column:created_at;type:text;not null"`
	UpdatedAt string  `gorm:"column:updated_at;type:text;not null"`
	ClosedAt  *string `gorm:"column:closed_at;type:text"`
}

func (Issue) TableName() string {
	return "issues"
}
