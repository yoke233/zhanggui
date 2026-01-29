package main

import (
	"fmt"
	"os"

	"github.com/yoke233/zhanggui/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
