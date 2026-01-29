package cli

import (
	"github.com/spf13/cobra"

	"github.com/yoke233/zhanggui/internal/config"
)

const (
	flagConfigPath = "config"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "taskctl",
		Short:         "本地单跑任务执行器（沙箱 + 落盘 + 打包）",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString(flagConfigPath)
			if err := config.Load(configPath); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.PersistentFlags().String(flagConfigPath, "", "配置文件路径（可选）")

	cmd.AddCommand(NewRunCmd())
	cmd.AddCommand(NewInspectCmd())
	cmd.AddCommand(NewPackCmd())
	cmd.AddCommand(NewApproveCmd())

	return cmd
}

func Execute() error {
	cmd := NewRootCmd()
	if err := cmd.Execute(); err != nil {
		return err
	}
	return nil
}
