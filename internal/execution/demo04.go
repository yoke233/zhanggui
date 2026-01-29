package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yoke233/zhanggui/internal/planning"
	"github.com/yoke233/zhanggui/internal/scheduler"
	"github.com/yoke233/zhanggui/internal/uuidv7"
	"github.com/yoke233/zhanggui/internal/verify"
)

type demo04Workflow struct{}

func NewDemo04Workflow() Workflow { return &demo04Workflow{} }

func (w *demo04Workflow) Name() string { return "demo04" }

const demo04DeliveryPlanYAML = `
teams:
  - team_id: team_a
    intent: "demo04"
roles:
  - role: writer
    count: 1
  - role: designer
    count: 1
budgets:
  max_parallel: 2
  per_team_parallel_cap:
    team_a: 2
  per_role_parallel_cap:
    writer: 2
    designer: 1
audit_policy:
  approval_policy: always
`

func (w *demo04Workflow) Run(ctx Context) (Result, error) {
	if ctx.GW == nil {
		return Result{}, fmt.Errorf("gw missing")
	}
	rev := ctx.Rev
	if rev == "" {
		rev = "r1"
	}

	plan, err := planning.ParseDeliveryPlanYAML([]byte(demo04DeliveryPlanYAML))
	if err != nil {
		return Result{}, err
	}
	if err := plan.Validate(); err != nil {
		return Result{}, err
	}
	caps, err := CapsFromPlan(plan)
	if err != nil {
		return Result{}, err
	}
	limiter, err := scheduler.NewLimiter(caps)
	if err != nil {
		return Result{}, err
	}

	mpus := []MPU{
		{MPUID: uuidv7.New(), TeamID: "team_a", Role: "writer", Kind: "report_section", Title: "Report: Summary"},
		{MPUID: uuidv7.New(), TeamID: "team_a", Role: "designer", Kind: "ppt_slide", Title: "Slide: Title"},
	}

	runCtx := ctx.Ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	if err := RunMPUs(runCtx, limiter, mpus, func(c context.Context, m MPU) error {
		select {
		case <-c.Done():
			return c.Err()
		default:
		}

		summaryPath := filepath.ToSlash(filepath.Join("revs", rev, "mpus", m.MPUID, "summary.md"))
		summary := []byte(fmt.Sprintf("# mpu summary\n\nmpu_id: %s\nkind: %s\nteam_id: %s\nrole: %s\ntitle: %s\n", m.MPUID, m.Kind, m.TeamID, m.Role, m.Title))
		return ctx.GW.ReplaceFile(summaryPath, summary, 0o644, "demo04: write mpu summary")
	}); err != nil {
		return Result{}, err
	}

	var issues []verify.Issue
	if err := AssembleDemo04(ctx); err != nil {
		return Result{}, err
	}

	adIssues, err := AdaptPPTIRToRendererInput(ctx)
	if err != nil {
		return Result{}, err
	}
	issues = append(issues, adIssues...)

	hasBlocker := Result{Issues: issues}.HasBlocker()
	if !hasBlocker {
		rIssues, err := RenderSlidesHTML(ctx)
		if err != nil {
			return Result{}, err
		}
		issues = append(issues, rIssues...)
	}

	// Normalize issues: keep severity/where stable.
	for i := range issues {
		issues[i].Severity = strings.ToLower(strings.TrimSpace(issues[i].Severity))
		issues[i].Where = strings.TrimSpace(issues[i].Where)
		issues[i].What = strings.TrimSpace(issues[i].What)
		issues[i].Action = strings.TrimSpace(issues[i].Action)
	}

	issuesFile := verify.IssuesFile{
		SchemaVersion: 1,
		TaskID:        ctx.TaskID,
		Rev:           rev,
		Issues:        issues,
	}
	issuesBody, err := json.MarshalIndent(issuesFile, "", "  ")
	if err != nil {
		return Result{}, err
	}
	issuesBody = append(issuesBody, '\n')
	issuesPath := filepath.ToSlash(filepath.Join("revs", rev, "issues.json"))
	if err := ctx.GW.ReplaceFile(issuesPath, issuesBody, 0o644, "demo04: write issues.json"); err != nil {
		return Result{}, err
	}

	return Result{Issues: issues}, nil
}
