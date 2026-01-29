package execution

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type PPTIR struct {
	SchemaVersion int        `json:"schema_version"`
	Title         string     `json:"title"`
	Slides        []PPTSlide `json:"slides"`
}

type PPTSlide struct {
	Title   string   `json:"title"`
	Bullets []string `json:"bullets,omitempty"`
}

type mpuSummary struct {
	Path  string
	Kind  string
	Title string
}

func AssembleDemo04(ctx Context) error {
	if ctx.GW == nil {
		return fmt.Errorf("gw missing")
	}
	rev := ctx.Rev
	if strings.TrimSpace(rev) == "" {
		rev = "r1"
	}

	taskDir := ctx.GW.Root()
	mpusDir := filepath.Join(taskDir, "revs", rev, "mpus")

	var items []mpuSummary
	walkErr := filepath.WalkDir(mpusDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Base(path), "summary.md") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		kind, title := parseMPUSummary(b)
		items = append(items, mpuSummary{Path: path, Kind: kind, Title: title})
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return walkErr
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Title != items[j].Title {
			return items[i].Title < items[j].Title
		}
		return items[i].Path < items[j].Path
	})

	var report strings.Builder
	report.WriteString("# demo04 report\n\n")
	report.WriteString(`<a id="block-deliver-report-2"></a>` + "\n\n")
	report.WriteString("## MPUs\n\n")
	if len(items) == 0 {
		report.WriteString("- (none)\n")
	} else {
		for _, it := range items {
			title := it.Title
			if strings.TrimSpace(title) == "" {
				title = "(untitled)"
			}
			kind := it.Kind
			if strings.TrimSpace(kind) == "" {
				kind = "(unknown)"
			}
			report.WriteString(fmt.Sprintf("- [%s] %s\n", kind, title))
		}
	}
	report.WriteString("\n")

	reportPath := filepath.ToSlash(filepath.Join("revs", rev, "deliver", "report.md"))
	if err := ctx.GW.ReplaceFile(reportPath, []byte(report.String()), 0o644, "demo04: assemble deliver/report.md"); err != nil {
		return err
	}

	ir := PPTIR{
		SchemaVersion: 1,
		Title:         "Demo04",
	}
	for _, it := range items {
		if it.Kind != "ppt_slide" {
			continue
		}
		title := it.Title
		if strings.TrimSpace(title) == "" {
			title = "Slide"
		}
		ir.Slides = append(ir.Slides, PPTSlide{Title: title})
	}
	if len(ir.Slides) == 0 {
		ir.Slides = []PPTSlide{{Title: "Slide 1"}}
	}

	irBody, err := json.MarshalIndent(ir, "", "  ")
	if err != nil {
		return err
	}
	irBody = append(irBody, '\n')
	irPath := filepath.ToSlash(filepath.Join("revs", rev, "deliver", "ppt_ir.json"))
	if err := ctx.GW.ReplaceFile(irPath, irBody, 0o644, "demo04: assemble deliver/ppt_ir.json"); err != nil {
		return err
	}

	return nil
}

func parseMPUSummary(b []byte) (kind, title string) {
	lines := strings.Split(string(b), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "kind:") {
			kind = strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
			continue
		}
		if strings.HasPrefix(line, "title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
			continue
		}
	}
	return kind, title
}
