package cmd

import (
	"github.com/spf13/cobra"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Worker runtime commands",
}

func init() {
	rootCmd.AddCommand(workerCmd)
}
