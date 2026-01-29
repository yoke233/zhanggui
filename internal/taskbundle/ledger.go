package taskbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yoke233/zhanggui/internal/gateway"
	"github.com/yoke233/zhanggui/internal/uuidv7"
)

type LedgerWriter struct {
	gw          *gateway.Gateway
	relPath     string
	nextSeq     uint64
	actor       LedgerActor
	correlation Correlation
}

func NewLedgerWriter(gw *gateway.Gateway, relPath string, actor LedgerActor, correlation Correlation) (*LedgerWriter, error) {
	if gw == nil {
		return nil, fmt.Errorf("gw 不能为空")
	}
	if relPath == "" {
		return nil, fmt.Errorf("relPath 不能为空")
	}
	if correlation.BundleID == "" {
		return nil, fmt.Errorf("correlation.bundle_id 不能为空")
	}

	nextSeq := uint64(1)
	abs := filepath.Join(gw.Root(), filepath.FromSlash(relPath))
	if s, err := nextSeqFromExistingLedger(abs); err != nil {
		return nil, err
	} else if s > 0 {
		nextSeq = s
	}
	return &LedgerWriter{
		gw:          gw,
		relPath:     relPath,
		nextSeq:     nextSeq,
		actor:       actor,
		correlation: correlation,
	}, nil
}

func (w *LedgerWriter) Append(eventType string, refs []Ref, payload any) error {
	if w == nil {
		return fmt.Errorf("LedgerWriter 为空")
	}
	if eventType == "" {
		return fmt.Errorf("eventType 不能为空")
	}
	if refs == nil {
		refs = []Ref{}
	}
	if payload == nil {
		payload = map[string]any{}
	}

	ev := LedgerEvent{
		SchemaVersion: 1,
		TS:            time.Now().UTC().Format(time.RFC3339Nano),
		Seq:           w.nextSeq,
		EventID:       uuidv7.New(),
		EventType:     eventType,
		Actor:         w.actor,
		Correlation:   w.correlation,
		Refs:          refs,
		Payload:       payload,
	}
	w.nextSeq++

	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return w.gw.AppendFile(w.relPath, b, 0o644, "ledger: "+eventType)
}

func nextSeqFromExistingLedger(ledgerAbs string) (uint64, error) {
	if ledgerAbs == "" {
		return 1, fmt.Errorf("ledgerAbs 不能为空")
	}
	b, err := os.ReadFile(ledgerAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, err
	}
	lines := bytes.Split(b, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var probe struct {
			Seq uint64 `json:"seq"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			return 0, fmt.Errorf("解析 ledger 最后一行失败: %w", err)
		}
		return probe.Seq + 1, nil
	}
	return 1, nil
}
