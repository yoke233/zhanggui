package execution

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/zhanggui/internal/verify"
)

func RenderSlidesHTML(ctx Context) ([]verify.Issue, error) {
	if ctx.GW == nil {
		return nil, fmt.Errorf("gw missing")
	}
	rev := strings.TrimSpace(ctx.Rev)
	if rev == "" {
		rev = "r1"
	}

	inAbs := filepath.Join(ctx.GW.Root(), "revs", rev, "deliver", "ppt_renderer_input.json")
	b, err := os.ReadFile(inAbs)
	if err != nil {
		return []verify.Issue{{
			Severity: "blocker",
			Where:    "renderer",
			What:     "缺少必需文件 deliver/ppt_renderer_input.json",
			Action:   "先生成 deliver/ppt_renderer_input.json 再运行 renderer",
		}}, nil
	}

	var in PPTRendererInput
	if err := json.Unmarshal(b, &in); err != nil {
		return []verify.Issue{{
			Severity: "blocker",
			Where:    "renderer",
			What:     "ppt_renderer_input.json 不是合法 JSON",
			Action:   "修复 ppt_renderer_input.json 格式",
		}}, nil
	}

	var issues []verify.Issue
	if in.SchemaVersion == 0 {
		issues = append(issues, verify.Issue{
			Severity: "blocker",
			Where:    "renderer",
			What:     "ppt_renderer_input.json 缺少 schema_version",
			Action:   "补充 schema_version",
		})
	} else if in.SchemaVersion != 1 {
		issues = append(issues, verify.Issue{
			Severity: "blocker",
			Where:    "renderer",
			What:     fmt.Sprintf("不支持的 schema_version: %d", in.SchemaVersion),
			Action:   "将 schema_version 调整为 1",
		})
	}
	if len(in.Slides) == 0 {
		issues = append(issues, verify.Issue{
			Severity: "blocker",
			Where:    "renderer",
			What:     "ppt_renderer_input.json slides 不能为空",
			Action:   "至少提供 1 张 slide",
		})
	}

	for _, it := range issues {
		if strings.EqualFold(it.Severity, "blocker") {
			return issues, nil
		}
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Untitled"
	}

	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n")
	sb.WriteString("<title>" + html.EscapeString(title) + "</title>\n")
	sb.WriteString("</head>\n<body>\n")
	sb.WriteString("<h1>" + html.EscapeString(title) + "</h1>\n")
	for _, s := range in.Slides {
		st := strings.TrimSpace(s.Title)
		if st == "" {
			st = "Slide"
		}
		sb.WriteString("<section class=\"slide\">\n")
		sb.WriteString("<h2>" + html.EscapeString(st) + "</h2>\n")
		if len(s.Bullets) > 0 {
			sb.WriteString("<ul>\n")
			for _, b := range s.Bullets {
				bt := strings.TrimSpace(b)
				if bt == "" {
					continue
				}
				sb.WriteString("<li>" + html.EscapeString(bt) + "</li>\n")
			}
			sb.WriteString("</ul>\n")
		}
		sb.WriteString("</section>\n")
	}
	sb.WriteString("</body>\n</html>\n")

	dst := filepath.ToSlash(filepath.Join("revs", rev, "deliver", "slides.html"))
	if err := ctx.GW.ReplaceFile(dst, []byte(sb.String()), 0o644, "renderer: write deliver/slides.html"); err != nil {
		return nil, err
	}

	return nil, nil
}
