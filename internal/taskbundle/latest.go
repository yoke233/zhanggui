package taskbundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func ReadLatestPointer(taskRootAbs string) (LatestPointer, error) {
	if taskRootAbs == "" {
		return LatestPointer{}, fmt.Errorf("taskRootAbs 不能为空")
	}
	b, err := os.ReadFile(filepath.Join(taskRootAbs, "pack", "latest.json"))
	if err != nil {
		return LatestPointer{}, err
	}
	var p LatestPointer
	if err := json.Unmarshal(b, &p); err != nil {
		return LatestPointer{}, err
	}
	if p.SchemaVersion != 1 {
		return LatestPointer{}, fmt.Errorf("pack/latest.json schema_version 不支持: %d", p.SchemaVersion)
	}
	if p.TaskID == "" || p.PackID == "" {
		return LatestPointer{}, fmt.Errorf("pack/latest.json 缺少 task_id/pack_id")
	}
	return p, nil
}
