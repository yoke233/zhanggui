package main

import (
	"fmt"
	"os"

	"github.com/yoke233/zhanggui/internal/zhcli"
)

func main() {
	if err := zhcli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
