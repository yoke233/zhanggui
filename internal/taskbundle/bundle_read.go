package taskbundle

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ReadBundleCorrelation(taskRootAbs string, packID string) (Correlation, error) {
	if taskRootAbs == "" {
		return Correlation{}, fmt.Errorf("taskRootAbs 不能为空")
	}
	if strings.TrimSpace(packID) == "" {
		return Correlation{}, fmt.Errorf("packID 不能为空")
	}
	ledgerAbs := filepath.Join(taskRootAbs, "packs", packID, "ledger", "events.jsonl")
	f, err := os.Open(ledgerAbs)
	if err != nil {
		return Correlation{}, err
	}
	defer func() { _ = f.Close() }()

	type line struct {
		Correlation Correlation `json:"correlation"`
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var ln line
		if err := json.Unmarshal([]byte(raw), &ln); err != nil {
			return Correlation{}, err
		}
		if ln.Correlation.BundleID == "" {
			return Correlation{}, fmt.Errorf("ledger correlation.bundle_id 为空")
		}
		return ln.Correlation, nil
	}
	if err := sc.Err(); err != nil {
		return Correlation{}, err
	}
	return Correlation{}, fmt.Errorf("ledger 为空: %s", ledgerAbs)
}
