package engine

import (
	"crypto/rand"
	"fmt"
	"time"
)

func NewRunID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%x", time.Now().Format("20060102"), b)
}
