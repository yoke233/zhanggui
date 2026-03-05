package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	gitops "github.com/yoke233/ai-workflow/internal/git"
)

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
	if err := store.CreateProject(p); err != nil {
		return err
	}
	if p.DefaultBranch == "" {
		p.DefaultBranch = gitops.DetectDefaultBranch(p.RepoPath)
		if p.DefaultBranch != "" {
			_ = store.UpdateProject(p)
		}
	}
	return nil
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

func cmdRunCreate(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: ai-flow Run create <project-id> <name> <description> [template]")
	}

	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	bootstrapCfg, err := loadBootstrapConfig()
	if err != nil {
		return err
	}

	project, err := store.GetProject(args[0])
	if err != nil {
		return err
	}
	effectiveCfg, err := mergeBootstrapProjectConfig(bootstrapCfg, project.RepoPath)
	if err != nil {
		return err
	}

	template := strings.TrimSpace(effectiveCfg.Run.DefaultTemplate)
	if template == "" {
		template = "standard"
	}
	if len(args) > 3 {
		template = args[3]
	}

	p, err := exec.CreateRun(args[0], args[1], args[2], template, effectiveCfg.Run.MaxTotalRetries)
	if err != nil {
		return err
	}
	fmt.Printf("Run created: %s (template: %s, stages: %d)\n", p.ID, p.Template, len(p.Stages))
	return nil
}

func cmdRunstart(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow Run start <Run-id>")
	}

	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	scheduler, err := buildScheduler(exec, store)
	if err != nil {
		return err
	}
	if err := scheduler.Enqueue(args[0]); err != nil {
		return err
	}
	fmt.Printf("Run enqueued: %s\n", args[0])
	return nil
}

func cmdRunStatus(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow Run status <Run-id>")
	}

	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	p, err := store.GetRun(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Run: %s\n", p.ID)
	fmt.Printf("Status:   %s\n", p.Status)
	fmt.Printf("Stage:    %s\n", p.CurrentStage)
	fmt.Printf("Template: %s\n", p.Template)
	return nil
}

func cmdProjectScan(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai-flow project scan <root>")
	}
	root := args[0]

	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	repos, err := scanGitRepos(root)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Printf("No git repositories found under %s\n", root)
		return nil
	}

	existingProjects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return err
	}
	existingRepo := map[string]struct{}{}
	usedIDs := map[string]struct{}{}
	for _, p := range existingProjects {
		existingRepo[filepath.Clean(p.RepoPath)] = struct{}{}
		usedIDs[p.ID] = struct{}{}
	}

	added := 0
	skipped := 0
	for _, repoPath := range repos {
		cleanPath := filepath.Clean(repoPath)
		if _, ok := existingRepo[cleanPath]; ok {
			skipped++
			continue
		}

		id := uniqueProjectID(filepath.Base(cleanPath), usedIDs)
		project := &core.Project{
			ID:       id,
			Name:     filepath.Base(cleanPath),
			RepoPath: cleanPath,
		}
		if err := store.CreateProject(project); err != nil {
			return err
		}
		existingRepo[cleanPath] = struct{}{}
		usedIDs[id] = struct{}{}
		added++
	}

	fmt.Printf("Scan complete: discovered=%d added=%d skipped=%d\n", len(repos), added, skipped)
	return nil
}

func scanGitRepos(root string) ([]string, error) {
	var repos []string
	seen := map[string]struct{}{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		switch d.Name() {
		case ".worktrees":
			return filepath.SkipDir
		case ".git":
			repo := filepath.Dir(path)
			clean := filepath.Clean(repo)
			if _, ok := seen[clean]; !ok {
				seen[clean] = struct{}{}
				repos = append(repos, clean)
			}
			return filepath.SkipDir
		default:
			return nil
		}
	})
	return repos, err
}

func uniqueProjectID(base string, used map[string]struct{}) string {
	clean := strings.ToLower(strings.TrimSpace(base))
	clean = strings.ReplaceAll(clean, " ", "-")
	if clean == "" {
		clean = "project"
	}
	if _, exists := used[clean]; !exists {
		return clean
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", clean, i)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

func cmdRunList(args []string) error {
	_, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PROJECT\tRun\tSTATUS\tSTAGE\tQUEUED")

	if len(args) >= 1 && strings.TrimSpace(args[0]) != "" {
		Runs, err := store.ListRuns(args[0], core.RunFilter{Limit: 200})
		if err != nil {
			return err
		}
		for _, p := range Runs {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				p.ProjectID, p.ID, p.Status, p.CurrentStage, formatTime(p.QueuedAt))
		}
		return w.Flush()
	}

	projects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return err
	}
	for _, project := range projects {
		Runs, err := store.ListRuns(project.ID, core.RunFilter{Limit: 200})
		if err != nil {
			return err
		}
		for _, p := range Runs {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				p.ProjectID, p.ID, p.Status, p.CurrentStage, formatTime(p.QueuedAt))
		}
	}
	return w.Flush()
}

func cmdRunAction(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ai-flow Run action <Run-id> <approve|reject|modify|skip|rerun|change_role|abort|pause|resume> [--stage <stage>] [--role <role>] [--message <text>]")
	}

	actionType, err := parseActionType(args[1])
	if err != nil {
		return err
	}

	action := core.RunAction{
		RunID: args[0],
		Type:  actionType,
	}

	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--stage":
			i++
			if i >= len(args) {
				return fmt.Errorf("--stage requires a value")
			}
			action.Stage = core.StageID(args[i])
		case "--role":
			i++
			if i >= len(args) {
				return fmt.Errorf("--role requires a value")
			}
			action.Role = strings.TrimSpace(args[i])
		case "--message":
			i++
			if i >= len(args) {
				return fmt.Errorf("--message requires a value")
			}
			action.Message = strings.Join(args[i:], " ")
			i = len(args)
		default:
			// Backward-compatible positional tail as message.
			action.Message = strings.Join(args[i:], " ")
			i = len(args)
		}
	}

	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := exec.ApplyAction(context.Background(), action); err != nil {
		return err
	}
	fmt.Printf("Action applied: Run=%s action=%s\n", action.RunID, action.Type)
	return nil
}

func parseActionType(raw string) (core.HumanActionType, error) {
	switch core.HumanActionType(strings.ToLower(strings.TrimSpace(raw))) {
	case core.ActionApprove,
		core.ActionReject,
		core.ActionModify,
		core.ActionSkip,
		core.ActionRerun,
		core.ActionChangeRole,
		core.ActionAbort,
		core.ActionPause,
		core.ActionResume:
		return core.HumanActionType(strings.ToLower(strings.TrimSpace(raw))), nil
	default:
		return "", fmt.Errorf("unknown action type: %s", raw)
	}
}

func cmdSchedulerRun() error {
	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	scheduler, err := buildScheduler(exec, store)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := scheduler.Start(ctx); err != nil {
		return err
	}
	fmt.Println("Scheduler started. Press Ctrl+C to stop.")
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return scheduler.Stop(stopCtx)
}

func cmdSchedulerOnce() error {
	exec, store, err := bootstrap()
	if err != nil {
		return err
	}
	defer store.Close()

	scheduler, err := buildScheduler(exec, store)
	if err != nil {
		return err
	}
	if err := scheduler.RunOnce(context.Background()); err != nil {
		return err
	}
	fmt.Println("Scheduler run-once completed.")
	return nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}
