package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	v2sandbox "github.com/yoke233/ai-workflow/internal/adapters/sandbox"
	"github.com/yoke233/ai-workflow/internal/core"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "env":
		return runEnv(args[1:])
	case "exec":
		return runExec(args[1:])
	case "acp":
		return runACP(args[1:])
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func printUsage() {
	fmt.Println(`sandbox-tester - sandbox / ACP smoke tool

Usage:
  go run ./cmd/sandbox-tester env  [flags]
  go run ./cmd/sandbox-tester exec [flags] -- <command> [args...]
  go run ./cmd/sandbox-tester acp  [flags]

Subcommands:
  env   Print the effective sandbox env/dirs for codex-acp.
  exec  Run a host command directly or through bwrap.
  acp   Launch a real codex-acp process directly or through bwrap.

Examples:
  go run ./cmd/sandbox-tester env
  go run ./cmd/sandbox-tester exec --sandbox bwrap -- go version
  go run ./cmd/sandbox-tester acp --sandbox bwrap --skip-prompt
  go run ./cmd/sandbox-tester acp --sandbox bwrap --prompt 'Run go version and reply with stdout only.'
`)
}

type commonOptions struct {
	sandbox       string
	workDir       string
	dataDir       string
	scope         string
	baseCodexHome string
	requireAuth   bool
	mountBaseHome bool
}

func bindCommonFlags(fs *flag.FlagSet, opts *commonOptions) {
	fs.StringVar(&opts.sandbox, "sandbox", "none", "sandbox mode: none or bwrap")
	fs.StringVar(&opts.workDir, "workdir", "", "working directory; default is current directory")
	fs.StringVar(&opts.dataDir, "data-dir", "", "sandbox data dir; default is <workdir>/.ai-workflow/sandbox-tester")
	fs.StringVar(&opts.scope, "scope", "sandbox-tester", "sandbox scope name")
	fs.StringVar(&opts.baseCodexHome, "base-codex-home", "", "base CODEX_HOME path; when --mount-base-home is set and this flag is empty, use the real user's ~/.codex")
	fs.BoolVar(&opts.requireAuth, "require-auth", false, "fail if isolated CODEX_HOME has no auth.json")
	fs.BoolVar(&opts.mountBaseHome, "mount-base-home", false, "mount the real/base ~/.codex directly as CODEX_HOME inside bwrap instead of using an isolated copy")
}

func runEnv(args []string) error {
	var opts commonOptions
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	bindCommonFlags(fs, &opts)
	if err := fs.Parse(args); err != nil {
		return err
	}

	prepared, err := prepareCodexSandbox(opts)
	if err != nil {
		return err
	}
	printPreparedEnv(prepared)
	return nil
}

func runExec(args []string) error {
	var opts commonOptions
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	bindCommonFlags(fs, &opts)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cmdArgs := fs.Args()
	if len(cmdArgs) == 0 {
		cmdArgs = []string{"go", "version"}
	}

	prepared, err := prepareCodexSandbox(opts)
	if err != nil {
		return err
	}
	printPreparedEnv(prepared)

	launch := acpclient.LaunchConfig{
		Command: cmdArgs[0],
		Args:    append([]string(nil), cmdArgs[1:]...),
		WorkDir: prepared.WorkDir,
		Env:     cloneEnvMap(prepared.Env),
	}
	launch, err = maybeWrapWithBwrap(launch, opts.sandbox, prepared)
	if err != nil {
		return err
	}

	fmt.Printf(">>> running command: %s %s\n", launch.Command, strings.Join(launch.Args, " "))
	cmd := exec.Command(launch.Command, launch.Args...)
	cmd.Dir = launch.WorkDir
	cmd.Env = mergeEnv(launch.Env)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runACP(args []string) error {
	var opts commonOptions
	var timeout time.Duration
	var prompt string
	var skipPrompt bool
	fs := flag.NewFlagSet("acp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	bindCommonFlags(fs, &opts)
	fs.DurationVar(&timeout, "timeout", 2*time.Minute, "overall timeout")
	fs.StringVar(&prompt, "prompt", "Reply with exactly: SANDBOX_ACP_OK", "prompt text to send after creating the ACP session")
	fs.BoolVar(&skipPrompt, "skip-prompt", false, "only initialize and create ACP session, do not prompt")
	if err := fs.Parse(args); err != nil {
		return err
	}

	prepared, err := prepareCodexSandbox(opts)
	if err != nil {
		return err
	}
	printPreparedEnv(prepared)

	launch := acpclient.LaunchConfig{
		Command: "npx",
		Args:    []string{"-y", "@zed-industries/codex-acp"},
		WorkDir: prepared.WorkDir,
		Env:     cloneEnvMap(prepared.Env),
	}
	launch, err = maybeWrapWithBwrap(launch, opts.sandbox, prepared)
	if err != nil {
		return err
	}

	fmt.Printf(">>> launching ACP: %s %s\n", launch.Command, strings.Join(launch.Args, " "))
	client, err := acpclient.New(launch, &acpclient.NopHandler{}, acpclient.WithEventHandler(loggingEventHandler{}))
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Close(closeCtx)
		cancel()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Println(">>> initialize")
	if err := client.Initialize(ctx, acpclient.ClientCapabilities{
		FSRead:   true,
		FSWrite:  true,
		Terminal: true,
	}); err != nil {
		return fmt.Errorf("initialize acp: %w", err)
	}

	fmt.Println(">>> new session")
	sessionID, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        prepared.WorkDir,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	fmt.Printf(">>> session id: %s\n", sessionID)

	if skipPrompt {
		fmt.Println(">>> skip prompt requested")
		return nil
	}

	fmt.Printf(">>> prompt: %s\n", prompt)
	result, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sessionID,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: prompt}},
		},
	})
	if err != nil {
		return fmt.Errorf("prompt: %w", err)
	}

	fmt.Printf(">>> stop reason: %s\n", result.StopReason)
	fmt.Printf(">>> text:\n%s\n", result.Text)
	return nil
}

type preparedSandbox struct {
	WorkDir   string
	DataDir   string
	BaseHome  string
	CodexHome string
	TempDir   string
	Env       map[string]string
}

func prepareCodexSandbox(opts commonOptions) (preparedSandbox, error) {
	workDir, err := resolveWorkDir(opts.workDir)
	if err != nil {
		return preparedSandbox{}, err
	}
	dataDir, err := resolveDataDir(opts.dataDir, workDir)
	if err != nil {
		return preparedSandbox{}, err
	}
	baseHome, err := resolveBaseCodexHome(opts.baseCodexHome, opts.mountBaseHome)
	if err != nil {
		return preparedSandbox{}, err
	}

	if opts.mountBaseHome {
		return prepareMountedBaseHome(workDir, dataDir, baseHome, opts)
	}

	sb := v2sandbox.HomeDirSandbox{
		DataDir:          dataDir,
		SkillsRoot:       filepath.Join(dataDir, "skills"),
		RequireCodexAuth: opts.requireAuth,
	}
	launch, err := sb.Prepare(context.Background(), v2sandbox.PrepareInput{
		Profile: &core.AgentProfile{
			ID: "sandbox-tester",
			Driver: core.DriverConfig{
				ID:  "codex-acp",
				Env: map[string]string{"CODEX_HOME": baseHome},
			},
		},
		Launch: acpclient.LaunchConfig{
			WorkDir: workDir,
			Env:     map[string]string{},
		},
		Scope: opts.scope,
	})
	if err != nil {
		return preparedSandbox{}, err
	}

	codexHome := launch.Env["CODEX_HOME"]
	tmpDir := launch.Env["TMPDIR"]
	if codexHome == "" || tmpDir == "" {
		return preparedSandbox{}, errors.New("sandbox prepare did not produce CODEX_HOME/TMPDIR")
	}

	env := cloneEnvMap(launch.Env)
	env["HOME"] = codexHome
	env["PATH"] = os.Getenv("PATH")
	env["NPM_CONFIG_CACHE"] = filepath.Join(tmpDir, "npm-cache")
	env["XDG_CACHE_HOME"] = filepath.Join(tmpDir, "xdg-cache")
	env["PIP_CACHE_DIR"] = filepath.Join(tmpDir, "pip-cache")
	env["UV_CACHE_DIR"] = filepath.Join(tmpDir, "uv-cache")
	env["PYTHONPYCACHEPREFIX"] = filepath.Join(tmpDir, "pycache")
	env["XDG_CONFIG_HOME"] = codexHome
	env["GOCACHE"] = filepath.Join(tmpDir, "go-build")
	env["GOTMPDIR"] = filepath.Join(tmpDir, "go-tmp")
	env["GOPATH"] = filepath.Join(codexHome, "go")
	env["GOMODCACHE"] = filepath.Join(env["GOPATH"], "pkg", "mod")
	for _, key := range inheritedOptionalEnvKeys() {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			env[key] = value
		}
	}
	for _, key := range []string{
		"TMPDIR",
		"TMP",
		"TEMP",
		"NPM_CONFIG_CACHE",
		"XDG_CACHE_HOME",
		"PIP_CACHE_DIR",
		"UV_CACHE_DIR",
		"PYTHONPYCACHEPREFIX",
		"GOCACHE",
		"GOTMPDIR",
		"GOPATH",
		"GOMODCACHE",
	} {
		if err := os.MkdirAll(env[key], 0o755); err != nil {
			return preparedSandbox{}, fmt.Errorf("create sandbox dir %s: %w", env[key], err)
		}
	}

	return preparedSandbox{
		WorkDir:   workDir,
		DataDir:   dataDir,
		BaseHome:  baseHome,
		CodexHome: codexHome,
		TempDir:   tmpDir,
		Env:       env,
	}, nil
}

func prepareMountedBaseHome(workDir, dataDir, baseHome string, opts commonOptions) (preparedSandbox, error) {
	tmpDir := filepath.Join(dataDir, "mounted-home", sanitizePathComponent(opts.scope), "tmp")
	env := map[string]string{
		"CODEX_HOME": baseHome,
		"HOME":       baseHome,
		"TMPDIR":     tmpDir,
		"TMP":        tmpDir,
		"TEMP":       tmpDir,
		"PATH":       os.Getenv("PATH"),
	}
	env["NPM_CONFIG_CACHE"] = filepath.Join(tmpDir, "npm-cache")
	env["XDG_CACHE_HOME"] = filepath.Join(tmpDir, "xdg-cache")
	env["PIP_CACHE_DIR"] = filepath.Join(tmpDir, "pip-cache")
	env["UV_CACHE_DIR"] = filepath.Join(tmpDir, "uv-cache")
	env["PYTHONPYCACHEPREFIX"] = filepath.Join(tmpDir, "pycache")
	env["XDG_CONFIG_HOME"] = baseHome
	env["GOCACHE"] = filepath.Join(tmpDir, "go-build")
	env["GOTMPDIR"] = filepath.Join(tmpDir, "go-tmp")
	env["GOPATH"] = filepath.Join(baseHome, "go")
	env["GOMODCACHE"] = filepath.Join(env["GOPATH"], "pkg", "mod")

	for _, key := range inheritedOptionalEnvKeys() {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			env[key] = value
		}
	}
	for _, key := range []string{
		"TMPDIR",
		"TMP",
		"TEMP",
		"NPM_CONFIG_CACHE",
		"XDG_CACHE_HOME",
		"PIP_CACHE_DIR",
		"UV_CACHE_DIR",
		"PYTHONPYCACHEPREFIX",
		"GOCACHE",
		"GOTMPDIR",
	} {
		if err := os.MkdirAll(env[key], 0o755); err != nil {
			return preparedSandbox{}, fmt.Errorf("create mounted-home dir %s: %w", env[key], err)
		}
	}
	if opts.requireAuth && !fileExists(filepath.Join(baseHome, "auth.json")) {
		return preparedSandbox{}, fmt.Errorf("codex auth.json missing in mounted base home: %s", filepath.Join(baseHome, "auth.json"))
	}

	return preparedSandbox{
		WorkDir:   workDir,
		DataDir:   dataDir,
		BaseHome:  baseHome,
		CodexHome: baseHome,
		TempDir:   tmpDir,
		Env:       env,
	}, nil
}

func resolveWorkDir(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return os.Getwd()
	}
	return filepath.Abs(raw)
}

func resolveDataDir(raw string, workDir string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return filepath.Join(workDir, ".ai-workflow", "sandbox-tester"), nil
	}
	return filepath.Abs(raw)
}

func resolveBaseCodexHome(raw string, preferRealUserHome bool) (string, error) {
	if strings.TrimSpace(raw) != "" {
		return filepath.Abs(raw)
	}
	if preferRealUserHome {
		return resolveRealUserCodexHome()
	}
	if env := strings.TrimSpace(os.Getenv("CODEX_HOME")); env != "" {
		return filepath.Abs(env)
	}
	return resolveRealUserCodexHome()
}

func resolveRealUserCodexHome() (string, error) {
	if current, err := user.Current(); err == nil && current != nil {
		if home := strings.TrimSpace(current.HomeDir); home != "" {
			return filepath.Join(home, ".codex"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func printPreparedEnv(prepared preparedSandbox) {
	fmt.Println(">>> sandbox dirs")
	fmt.Printf("  workdir:    %s\n", prepared.WorkDir)
	fmt.Printf("  data dir:   %s\n", prepared.DataDir)
	fmt.Printf("  base home:  %s\n", prepared.BaseHome)
	fmt.Printf("  CODEX_HOME: %s\n", prepared.CodexHome)
	fmt.Printf("  TMPDIR:     %s\n", prepared.TempDir)
	fmt.Printf("  base auth:  %s (%s)\n", filepath.Join(prepared.BaseHome, "auth.json"), yesNo(fileExists(filepath.Join(prepared.BaseHome, "auth.json"))))
	fmt.Printf("  iso auth:   %s (%s)\n", filepath.Join(prepared.CodexHome, "auth.json"), yesNo(fileExists(filepath.Join(prepared.CodexHome, "auth.json"))))
	fmt.Println(">>> effective env")
	keys := make([]string, 0, len(prepared.Env))
	for key := range prepared.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("  %s=%s\n", key, prepared.Env[key])
	}
}

func maybeWrapWithBwrap(cfg acpclient.LaunchConfig, mode string, prepared preparedSandbox) (acpclient.LaunchConfig, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "none":
		return cfg, nil
	case "bwrap":
		return wrapLaunchWithBwrap(cfg, prepared)
	default:
		return cfg, fmt.Errorf("unsupported sandbox mode %q", mode)
	}
}

func wrapLaunchWithBwrap(cfg acpclient.LaunchConfig, prepared preparedSandbox) (acpclient.LaunchConfig, error) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		return cfg, fmt.Errorf("find bwrap: %w", err)
	}

	args := []string{
		"--die-with-parent",
		"--new-session",
		"--ro-bind", "/", "/",
		"--dev-bind", "/dev", "/dev",
		"--proc", "/proc",
		"--clearenv",
	}
	if strings.TrimSpace(prepared.TempDir) != "" {
		args = append(args, "--bind", prepared.TempDir, "/tmp")
	}

	writableRoots := uniqueStrings([]string{
		prepared.WorkDir,
		prepared.CodexHome,
		prepared.TempDir,
		prepared.Env["NPM_CONFIG_CACHE"],
		prepared.Env["XDG_CACHE_HOME"],
		prepared.Env["GOCACHE"],
		prepared.Env["GOTMPDIR"],
		prepared.Env["GOPATH"],
		prepared.Env["GOMODCACHE"],
	})
	for _, root := range writableRoots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return cfg, fmt.Errorf("create bwrap root %s: %w", root, err)
		}
		args = append(args, "--bind", root, root)
	}

	keys := make([]string, 0, len(cfg.Env))
	for key := range cfg.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--setenv", key, cfg.Env[key])
	}
	args = append(args, "--chdir", cfg.WorkDir, cfg.Command)
	args = append(args, cfg.Args...)

	return acpclient.LaunchConfig{
		Command: "bwrap",
		Args:    args,
		WorkDir: "",
		Env:     map[string]string{},
	}, nil
}

func mergeEnv(extra map[string]string) []string {
	env := os.Environ()
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}

func cloneEnvMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = filepath.Clean(strings.TrimSpace(item))
		if item == "" || item == "." {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func inheritedOptionalEnvKeys() []string {
	return []string{
		"LANG",
		"LC_ALL",
		"TERM",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"ALL_PROXY",
		"NO_PROXY",
		"http_proxy",
		"https_proxy",
		"all_proxy",
		"no_proxy",
		"SSL_CERT_FILE",
		"NODE_EXTRA_CA_CERTS",
		"NPM_CONFIG_REGISTRY",
		"npm_config_registry",
	}
}

type loggingEventHandler struct{}

func (loggingEventHandler) HandleSessionUpdate(_ context.Context, update acpclient.SessionUpdate) error {
	if update.Type == "" {
		return nil
	}
	if strings.TrimSpace(update.Text) != "" {
		fmt.Printf(">>> event: type=%s text=%q\n", update.Type, truncate(update.Text, 120))
		return nil
	}
	if strings.TrimSpace(update.Status) != "" {
		fmt.Printf(">>> event: type=%s status=%s\n", update.Type, update.Status)
		return nil
	}
	fmt.Printf(">>> event: type=%s\n", update.Type)
	return nil
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func yesNo(ok bool) string {
	if ok {
		return "present"
	}
	return "missing"
}

func sanitizePathComponent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "default"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	s = replacer.Replace(s)
	s = strings.Trim(s, "._-")
	if s == "" {
		return "default"
	}
	return s
}
