package agui

import (
	"time"

	"github.com/yoke233/zhanggui/internal/uuidv7"
)

func newRunID(now time.Time) string {
	return uuidv7.NewAt(now)
}

func newThreadID(now time.Time) string {
	return uuidv7.NewAt(now)
}

func newToolCallID(now time.Time) string {
	return uuidv7.NewAt(now)
}

func newMessageID(now time.Time) string {
	return uuidv7.NewAt(now)
}

func newInterruptID(now time.Time) string {
	return uuidv7.NewAt(now)
}
