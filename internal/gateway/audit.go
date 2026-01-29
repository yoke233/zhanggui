package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Auditor struct {
	path string
	mu   sync.Mutex
	f    *os.File
}

func NewAuditor(path string) (*Auditor, error) {
	if path == "" {
		return nil, fmt.Errorf("audit path 不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Auditor{path: path, f: f}, nil
}

func (a *Auditor) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.f == nil {
		return nil
	}
	err := a.f.Close()
	a.f = nil
	return err
}

func (a *Auditor) Write(rec AuditRecord) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.f == nil {
		return fmt.Errorf("audit file closed: %s", a.path)
	}
	_, err = a.f.Write(b)
	return err
}
