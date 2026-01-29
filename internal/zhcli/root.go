package zhcli

import (
	"github.com/spf13/cobra"

	"github.com/yoke233/zhanggui/internal/config"
)

const (
	flagConfigPath = "config"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "zhanggui",
		Short:         "zhanggui：本地运行与对接服务",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString(flagConfigPath)
			return config.LoadWithOptions(config.Options{
				ConfigPath: configPath,
				EnvPrefix:  "ZHANGGUI",
			})
		},
	}

	cmd.PersistentFlags().String(flagConfigPath, "", "配置文件路径（可选）")

	cmd.AddCommand(NewServeCmd())

	return cmd
}

func Execute() error {
	cmd := NewRootCmd()
	return cmd.Execute()
}
