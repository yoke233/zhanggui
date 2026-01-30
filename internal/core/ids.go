package core

import (
	"time"

	"github.com/yoke233/zhanggui/internal/uuidv7"
)

func NewTaskID(now time.Time) string {
	return uuidv7.NewAt(now)
}

func NewContextID(now time.Time) string {
	return uuidv7.NewAt(now)
}

func NewMessageID(now time.Time) string {
	return uuidv7.NewAt(now)
}

func NewArtifactID(now time.Time) string {
	return uuidv7.NewAt(now)
}
