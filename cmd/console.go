package cmd

import (
	"github.com/spf13/cobra"
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Terminal console commands",
}

func init() {
	rootCmd.AddCommand(consoleCmd)
}
