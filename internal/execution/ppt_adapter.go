package execution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/zhanggui/internal/verify"
)

type PPTRendererInput struct {
	SchemaVersion int        `json:"schema_version"`
	Title         string     `json:"title"`
	Slides        []PPTSlide `json:"slides"`
}

func AdaptPPTIRToRendererInput(ctx Context) ([]verify.Issue, error) {
	if ctx.GW == nil {
		return nil, fmt.Errorf("gw missing")
	}
	rev := strings.TrimSpace(ctx.Rev)
	if rev == "" {
		rev = "r1"
	}

	pptIRAbs := filepath.Join(ctx.GW.Root(), "revs", rev, "deliver", "ppt_ir.json")
	b, err := os.ReadFile(pptIRAbs)
	if err != nil {
		return []verify.Issue{{
			Severity: "blocker",
			Where:    "adapter",
			What:     "缺少必需文件 deliver/ppt_ir.json",
			Action:   "先生成 deliver/ppt_ir.json 再运行 adapter",
		}}, nil
	}

	var ir PPTIR
	if err := json.Unmarshal(b, &ir); err != nil {
		return []verify.Issue{{
			Severity: "blocker",
			Where:    "adapter",
			What:     "ppt_ir.json 不是合法 JSON",
			Action:   "修复 ppt_ir.json 格式",
		}}, nil
	}

	var issues []verify.Issue
	if ir.SchemaVersion == 0 {
		issues = append(issues, verify.Issue{
			Severity: "blocker",
			Where:    "adapter",
			What:     "ppt_ir.json 缺少 schema_version",
			Action:   "补充 schema_version",
		})
	} else if ir.SchemaVersion != 1 {
		issues = append(issues, verify.Issue{
			Severity: "blocker",
			Where:    "adapter",
			What:     fmt.Sprintf("不支持的 schema_version: %d", ir.SchemaVersion),
			Action:   "将 schema_version 调整为 1",
		})
	}
	if len(ir.Slides) == 0 {
		issues = append(issues, verify.Issue{
			Severity: "blocker",
			Where:    "adapter",
			What:     "ppt_ir.json slides 不能为空",
			Action:   "至少提供 1 张 slide",
		})
	}

	for _, it := range issues {
		if strings.EqualFold(it.Severity, "blocker") {
			return issues, nil
		}
	}

	out := PPTRendererInput{
		SchemaVersion: 1,
		Title:         strings.TrimSpace(ir.Title),
		Slides:        ir.Slides,
	}
	if out.Title == "" {
		out.Title = "Untitled"
	}
	for i := range out.Slides {
		if strings.TrimSpace(out.Slides[i].Title) == "" {
			out.Slides[i].Title = fmt.Sprintf("Slide %d", i+1)
		}
	}

	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	body = append(body, '\n')

	dst := filepath.ToSlash(filepath.Join("revs", rev, "deliver", "ppt_renderer_input.json"))
	if err := ctx.GW.ReplaceFile(dst, body, 0o644, "adapter: write deliver/ppt_renderer_input.json"); err != nil {
		return nil, err
	}

	return nil, nil
}
