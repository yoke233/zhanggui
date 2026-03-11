package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	agentruntime "github.com/yoke233/ai-workflow/internal/runtime/agent"
)

func RunExecutor(args []string) error {
	opts, err := parseExecutorArgs(args)
	if err != nil {
		return err
	}
	natsURL := opts.natsURL
	if natsURL == "" {
		natsURL = os.Getenv("AI_WORKFLOW_NATS_URL")
	}
	if natsURL == "" {
		return fmt.Errorf("--nats-url is required (or set AI_WORKFLOW_NATS_URL)")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, _, _, err := LoadConfig()
	if err != nil {
		return err
	}
	dbPath := expandStorePath(cfg.Store.Path)
	runtimeDBPath := strings.TrimSuffix(dbPath, ".db") + "_runtime.db"
	store, err := sqlite.New(runtimeDBPath)
	if err != nil {
		return fmt.Errorf("open runtime store: %w", err)
	}
	defer store.Close()
	seedRegistry(context.Background(), store, cfg, nil)
	nc, err := nats.Connect(natsURL, nats.RetryOnFailedConnect(true), nats.MaxReconnects(-1), nats.ReconnectWait(2*time.Second))
	if err != nil {
		return fmt.Errorf("connect to NATS at %s: %w", natsURL, err)
	}
	defer nc.Drain()
	streamPrefix := "aiworkflow"
	if cfg.Runtime.SessionManager.NATS.StreamPrefix != "" {
		streamPrefix = cfg.Runtime.SessionManager.NATS.StreamPrefix
	}
	worker, err := agentruntime.NewExecutorWorker(agentruntime.ExecutorWorkerConfig{
		NATSConn:       nc,
		StreamPrefix:   streamPrefix,
		WorkerID:       cfg.Runtime.SessionManager.ServerID,
		AgentTypes:     opts.agentTypes,
		Store:          store,
		Registry:       store,
		DefaultWorkDir: resolveDefaultWorkDir(),
		MaxConcurrent:  opts.maxConcurrent,
	})
	if err != nil {
		return fmt.Errorf("create executor worker: %w", err)
	}
	slog.Info("executor: starting worker", "agents", opts.agentTypes, "max_concurrent", opts.maxConcurrent)
	err = worker.Start(ctx)
	worker.Stop()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func resolveDefaultWorkDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

type executorCLIOptions struct {
	natsURL       string
	agentTypes    []string
	maxConcurrent int
}

func parseExecutorArgs(args []string) (executorCLIOptions, error) {
	opts := executorCLIOptions{maxConcurrent: 2}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--nats-url":
			i++
			if i >= len(args) {
				return executorCLIOptions{}, fmt.Errorf("missing value for --nats-url")
			}
			opts.natsURL = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--nats-url="):
			opts.natsURL = strings.TrimSpace(strings.TrimPrefix(arg, "--nats-url="))
		case arg == "--agents":
			i++
			if i >= len(args) {
				return executorCLIOptions{}, fmt.Errorf("missing value for --agents")
			}
			opts.agentTypes = parseAgentTypes(args[i])
		case strings.HasPrefix(arg, "--agents="):
			opts.agentTypes = parseAgentTypes(strings.TrimPrefix(arg, "--agents="))
		case arg == "--max-concurrent":
			i++
			if i >= len(args) {
				return executorCLIOptions{}, fmt.Errorf("missing value for --max-concurrent")
			}
			n, err := parsePositiveInt(args[i], "--max-concurrent")
			if err != nil {
				return executorCLIOptions{}, err
			}
			opts.maxConcurrent = n
		case strings.HasPrefix(arg, "--max-concurrent="):
			n, err := parsePositiveInt(strings.TrimPrefix(arg, "--max-concurrent="), "--max-concurrent")
			if err != nil {
				return executorCLIOptions{}, err
			}
			opts.maxConcurrent = n
		default:
			return executorCLIOptions{}, fmt.Errorf("unknown flag: %s", arg)
		}
	}
	return opts, nil
}

func parseAgentTypes(raw string) []string {
	var agents []string
	for _, a := range strings.Split(raw, ",") {
		if t := strings.TrimSpace(a); t != "" {
			agents = append(agents, t)
		}
	}
	return agents
}

func parsePositiveInt(raw string, flagName string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid value for %s: %s", flagName, raw)
	}
	return n, nil
}
