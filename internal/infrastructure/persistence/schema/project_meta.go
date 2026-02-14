package schema

import "time"

type ProjectMeta struct {
	ID        uint      `gorm:"column:id;primaryKey;autoIncrement"`
	Key       string    `gorm:"column:key;type:text;uniqueIndex;not null"`
	Value     string    `gorm:"column:value;type:text;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (ProjectMeta) TableName() string {
	return "project_meta"
}
