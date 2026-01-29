package taskdir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/yoke233/zhanggui/internal/uuidv7"
)

type SandboxSpec struct {
	Mode           string `json:"mode"`
	Image          string `json:"image,omitempty"`
	Network        string `json:"network,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type WorkspaceSpec struct {
	InputROPaths []string `json:"input_ro_paths,omitempty"`
	OutputRWPath string   `json:"output_rw_path,omitempty"`
}

type ParamsSpec struct {
	Entrypoint []string `json:"entrypoint,omitempty"`
}

type Task struct {
	SchemaVersion int           `json:"schema_version"`
	TaskID        string        `json:"task_id"`
	RunID         string        `json:"run_id"`
	CreatedAt     string        `json:"created_at"`
	ToolVersion   string        `json:"tool_version"`
	Sandbox       SandboxSpec   `json:"sandbox"`
	Workspace     WorkspaceSpec `json:"workspace"`
	Params        ParamsSpec    `json:"params,omitempty"`
}

type TaskDir struct {
	baseDir string
	taskID  string
	root    string
}

func CreateNew(baseDir, taskID string) (*TaskDir, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir 不能为空")
	}
	if taskID == "" {
		return nil, fmt.Errorf("taskID 不能为空")
	}

	root := filepath.Join(baseDir, taskID)
	if _, err := os.Stat(root); err == nil {
		return nil, fmt.Errorf("任务目录已存在: %s", root)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	td := &TaskDir{baseDir: baseDir, taskID: taskID, root: root}
	for _, d := range []string{td.LogsDir(), td.RevsDir(), td.PacksDir(), td.PackDir(), td.VerifyDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return td, nil
}

func Open(taskDir string) (*TaskDir, error) {
	if taskDir == "" {
		return nil, fmt.Errorf("taskDir 不能为空")
	}
	abs, err := filepath.Abs(taskDir)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, err
	}
	taskID := filepath.Base(abs)
	baseDir := filepath.Dir(abs)
	return &TaskDir{baseDir: baseDir, taskID: taskID, root: abs}, nil
}

func (t *TaskDir) Root() string    { return t.root }
func (t *TaskDir) TaskID() string  { return t.taskID }
func (t *TaskDir) BaseDir() string { return t.baseDir }
func (t *TaskDir) LogsDir() string { return filepath.Join(t.root, "logs") }
func (t *TaskDir) RevsDir() string { return filepath.Join(t.root, "revs") }
func (t *TaskDir) PacksDir() string {
	return filepath.Join(t.root, "packs")
}
func (t *TaskDir) PackDir() string { return filepath.Join(t.root, "pack") }
func (t *TaskDir) VerifyDir() string {
	return filepath.Join(t.root, "verify")
}
func (t *TaskDir) RevDir(rev string) string {
	return filepath.Join(t.root, "revs", rev)
}

var revRe = regexp.MustCompile(`^r(\d+)$`)

func (t *TaskDir) LatestRev() (string, error) {
	entries, err := os.ReadDir(t.RevsDir())
	if err != nil {
		return "", err
	}
	max := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := revRe.FindStringSubmatch(e.Name())
		if len(m) != 2 {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	if max == 0 {
		return "", fmt.Errorf("未找到 rev 目录: %s", t.RevsDir())
	}
	return fmt.Sprintf("r%d", max), nil
}

func NewTaskID(now time.Time) (string, error) {
	return uuidv7.NewAt(now), nil
}

func NewRunID(now time.Time) (string, error) {
	return uuidv7.NewAt(now), nil
}

func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, b, 0o644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp := filepath.Join(dir, "."+base+".tmp")
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic rename failed: %w", err)
	}
	return nil
}

func SortedFilePaths(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
