package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/yoke233/zhanggui/internal/state"
)

func NewInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <task_dir>",
		Short: "读取任务目录并打印状态机信息",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskDir := args[0]
			statePath := filepath.Join(taskDir, "state.json")
			st, err := state.ReadJSON(statePath)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(st); err != nil {
				return fmt.Errorf("输出 state.json 失败: %w", err)
			}
			return nil
		},
	}
	return cmd
}
