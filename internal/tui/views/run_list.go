package views

import (
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

func RenderRunList(Runs []core.Run, cursor int, styleStatus map[string]func(string) string) string {
	if len(Runs) == 0 {
		return "No Runs found. Use `ai-flow Run create` to get started.\n"
	}

	var b strings.Builder
	for i, p := range Runs {
		prefix := "  "
		if i == cursor {
			prefix = "> "
		}

		status := string(p.Status)
		if fn, ok := styleStatus[status]; ok {
			status = fn(status)
		}
		currentStage := string(p.CurrentStage)
		if currentStage == "" {
			currentStage = "-"
		}
		b.WriteString(fmt.Sprintf("%s%-12s %-21s %-20s %-16s %s\n", prefix, p.ProjectID, p.ID, p.Name, currentStage, status))
	}
	return b.String()
}
