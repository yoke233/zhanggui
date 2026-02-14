package model

type OutboxKV struct {
	Key       string `gorm:"column:key;type:text;primaryKey"`
	Value     string `gorm:"column:value;type:text;not null"`
	UpdatedAt string `gorm:"column:updated_at;type:text;not null"`
}

func (OutboxKV) TableName() string {
	return "outbox_kv"
}
