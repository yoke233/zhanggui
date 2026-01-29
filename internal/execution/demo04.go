package execution

import (
	"fmt"
	"path/filepath"
	"time"
)

type demo04Workflow struct{}

func NewDemo04Workflow() Workflow { return &demo04Workflow{} }

func (w *demo04Workflow) Name() string { return "demo04" }

func (w *demo04Workflow) Run(ctx Context) (Result, error) {
	if ctx.GW == nil {
		return Result{}, fmt.Errorf("gw missing")
	}
	rev := ctx.Rev
	if rev == "" {
		rev = "r1"
	}
	dst := filepath.ToSlash(filepath.Join("revs", rev, "deliver", "report.md"))
	body := []byte(fmt.Sprintf("# demo04 report\n\ngenerated_at: %s\n", time.Now().Format(time.RFC3339)))
	if err := ctx.GW.ReplaceFile(dst, body, 0o644, "demo04: write deliver/report.md"); err != nil {
		return Result{}, err
	}
	return Result{}, nil
}
