package appcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func RunQualityGate(args []string) error {
	runBackend := true
	runFrontend := true
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		switch arg {
		case "--backend-only":
			runFrontend = false
		case "--frontend-only":
			runBackend = false
		case "--skip-backend":
			runBackend = false
		case "--skip-frontend":
			runFrontend = false
		default:
			return fmt.Errorf("usage: ai-flow quality-gate [--backend-only|--frontend-only|--skip-backend|--skip-frontend]")
		}
	}
	if !runBackend && !runFrontend {
		return fmt.Errorf("quality-gate: nothing to run (both backend and frontend are disabled)")
	}

	ctx := context.Background()
	if runBackend {
		fmt.Println("[quality-gate] backend: go test ./...")
		if err := runCommand(ctx, "go", "test", "./..."); err != nil {
			return fmt.Errorf("backend checks failed: %w", err)
		}
	}
	if runFrontend {
		fmt.Println("[quality-gate] frontend: npm --prefix web run test")
		if err := runCommand(ctx, "npm", "--prefix", "web", "run", "test"); err != nil {
			return fmt.Errorf("frontend tests failed: %w", err)
		}
		fmt.Println("[quality-gate] frontend: npm --prefix web run build")
		if err := runCommand(ctx, "npm", "--prefix", "web", "run", "build"); err != nil {
			return fmt.Errorf("frontend build failed: %w", err)
		}
	}
	fmt.Println("[quality-gate] all checks passed")
	return nil
}

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
