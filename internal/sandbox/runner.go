package sandbox

import (
	"fmt"
	"strings"

	"github.com/yoke233/zhanggui/internal/taskdir"
)

func NewRunner(cfg taskdir.SandboxSpec) (Runner, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "docker"
	}

	switch mode {
	case "docker":
		return NewDockerRunner(cfg), nil
	case "local":
		return NewLocalRunner(cfg), nil
	default:
		return nil, fmt.Errorf("不支持的 sandbox mode: %s（仅支持 docker/local）", cfg.Mode)
	}
}
