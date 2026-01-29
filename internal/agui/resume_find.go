package agui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func findInterruptedRun(runsDir string, interruptID string) (string, RunState, error) {
	interruptID = strings.TrimSpace(interruptID)
	if interruptID == "" {
		return "", RunState{}, fmt.Errorf("interrupt_id 不能为空")
	}

	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return "", RunState{}, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") || strings.EqualFold(e.Name(), "_server") {
			continue
		}
		runRoot := filepath.Join(runsDir, e.Name())
		st, err := readRunState(runRoot)
		if err != nil {
			continue
		}
		if st.PendingInt == nil || strings.TrimSpace(st.PendingInt.ID) == "" {
			continue
		}
		if st.PendingInt.ID == interruptID {
			return e.Name(), st, nil
		}
	}

	return "", RunState{}, fmt.Errorf("未找到 interrupt_id 对应的 run: %s", interruptID)
}
