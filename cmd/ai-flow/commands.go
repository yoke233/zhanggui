package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"text/tabwriter"

	"github.com/user/ai-workflow/internal/config"
	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/engine"
	"github.com/user/ai-workflow/internal/eventbus"
	pluginfactory "github.com/user/ai-workflow/internal/plugins/factory"
)

var recoveryOnce sync.Once

func bootstrap() (*engine.Executor, core.Store, error) {
	cfg, err := loadBootstrapConfig()
	if err != nil {
		return nil, nil, err
	}

	bootstrapSet, err := pluginfactory.BuildFromConfig(*cfg)
	if err != nil {
		return nil, nil, err
	}

	bus := eventbus.New()
	logger := slog.Default()
	exec := engine.NewExecutor(bootstrapSet.Store, bus, bootstrapSet.Agents, bootstrapSet.Runtime, logger)

	recoveryOnce.Do(func() {
		go func() {
			if recErr := exec.RecoverActivePipelines(context.Background()); recErr != nil {
				logger.Error("recovery failed", "error", recErr)
			}
		}()
	})

	return exec, bootstrapSet.Store, nil
}

func loadBootstrapConfig() (*config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(home, ".ai-workflow")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(dataDir, "config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return config.LoadGlobal(cfgPath)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	cfg := config.Defaults()
	if err := config.ApplyEnvOverrides(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func cmdProjectAdd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ai-flow project add <id> <repo-path>")
	}
	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	p := &core.Project{ID: args[0], Name: args[0], RepoPath: args[1]}
	return store.CreateProject(p)
}

func cmdProjectList() error {
	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	projects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tPATH")
	for _, p := range projects {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Name, p.RepoPath)
	}
	return w.Flush()
}

func cmdPipelineCreate(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: ai-flow pipeline create <project-id> <name> <description> [template]")
	}

	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	template := "standard"
	if len(args) > 3 {
		template = args[3]
	}

	p, err := exec.CreatePipeline(args[0], args[1], args[2], template)
	if err != nil {
		return err
	}
	fmt.Printf("Pipeline created: %s (template: %s, stages: %d)\n", p.ID, p.Template, len(p.Stages))
	return nil
}

func cmdPipelineStart(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow pipeline start <pipeline-id>")
	}

	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	return exec.Run(context.Background(), args[0])
}

func cmdPipelineStatus(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow pipeline status <pipeline-id>")
	}

	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	p, err := store.GetPipeline(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Pipeline: %s\n", p.ID)
	fmt.Printf("Status:   %s\n", p.Status)
	fmt.Printf("Stage:    %s\n", p.CurrentStage)
	fmt.Printf("Template: %s\n", p.Template)
	return nil
}
