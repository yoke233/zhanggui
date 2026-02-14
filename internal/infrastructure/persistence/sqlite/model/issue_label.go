package model

type IssueLabel struct {
	IssueID uint64 `gorm:"column:issue_id;not null;primaryKey"`
	Label   string `gorm:"column:label;type:text;not null;primaryKey"`
}

func (IssueLabel) TableName() string {
	return "issue_labels"
}
