/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"context"
	"os"

	"zhanggui/cmd"
)

func main() {
	if err := cmd.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}
