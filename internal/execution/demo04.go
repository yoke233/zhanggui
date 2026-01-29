package execution

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/yoke233/zhanggui/internal/uuidv7"
	"github.com/yoke233/zhanggui/internal/verify"
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

	mpus := []MPU{
		{MPUID: uuidv7.New(), TeamID: "team_a", Role: "writer", Kind: "report_section", Title: "Report: Summary"},
		{MPUID: uuidv7.New(), TeamID: "team_a", Role: "designer", Kind: "ppt_slide", Title: "Slide: Title"},
	}

	for _, m := range mpus {
		summaryPath := filepath.ToSlash(filepath.Join("revs", rev, "mpus", m.MPUID, "summary.md"))
		summary := []byte(fmt.Sprintf("# mpu summary\n\nmpu_id: %s\nkind: %s\nteam_id: %s\nrole: %s\ntitle: %s\n", m.MPUID, m.Kind, m.TeamID, m.Role, m.Title))
		if err := ctx.GW.ReplaceFile(summaryPath, summary, 0o644, "demo04: write mpu summary"); err != nil {
			return Result{}, err
		}
	}

	issues := verify.IssuesFile{
		SchemaVersion: 1,
		TaskID:        ctx.TaskID,
		Rev:           rev,
		Issues:        []verify.Issue{},
	}
	issuesBody, err := json.MarshalIndent(issues, "", "  ")
	if err != nil {
		return Result{}, err
	}
	issuesBody = append(issuesBody, '\n')
	issuesPath := filepath.ToSlash(filepath.Join("revs", rev, "issues.json"))
	if err := ctx.GW.ReplaceFile(issuesPath, issuesBody, 0o644, "demo04: write issues.json"); err != nil {
		return Result{}, err
	}

	dst := filepath.ToSlash(filepath.Join("revs", rev, "deliver", "report.md"))
	body := []byte(fmt.Sprintf("# demo04 report\n\ngenerated_at: %s\n", time.Now().Format(time.RFC3339)))
	if err := ctx.GW.ReplaceFile(dst, body, 0o644, "demo04: write deliver/report.md"); err != nil {
		return Result{}, err
	}
	return Result{}, nil
}
