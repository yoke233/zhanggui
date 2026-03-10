package main

import "github.com/yoke233/ai-workflow/internal/appdata"

func resolveDataDir() (string, error) {
	return appdata.ResolveDataDir()
}
