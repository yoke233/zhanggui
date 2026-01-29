package sandbox

import (
	"os"
	"path/filepath"
	"time"
)

func ensureDefaultArtifacts(outputDir, taskID, runID, rev string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	summaryPath := filepath.Join(outputDir, "summary.md")
	if _, err := os.Stat(summaryPath); err != nil {
		content := []byte("# Summary\n\n" +
			"- task_id: " + taskID + "\n" +
			"- run_id: " + runID + "\n" +
			"- rev: " + rev + "\n" +
			"- generated_at: " + time.Now().Format(time.RFC3339) + "\n\n" +
			"本次未指定沙箱执行命令，已生成默认产物。\n")
		if err := os.WriteFile(summaryPath, content, 0o644); err != nil {
			return err
		}
	}

	return nil
}
