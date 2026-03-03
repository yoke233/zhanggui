package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

const dropPrefix = "[litebox-acp/drop] "

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, " ")
}

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type bridgeConfig struct {
	runnerPath  string
	runnerArgs  []string
	program     string
	programArgs []string
}

func main() {
	cfg, err := parseBridgeConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	exitCode, runErr := runBridge(cfg, os.Stdin, os.Stdout, os.Stderr)
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	if runErr != nil {
		os.Exit(1)
	}
}

func parseBridgeConfig(args []string) (bridgeConfig, error) {
	var cfg bridgeConfig
	var runnerArgs stringSliceFlag
	var programArgs stringSliceFlag

	fs := flag.NewFlagSet("litebox-acp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.runnerPath, "runner", "", "LiteBox runner 可执行文件路径（必填）")
	fs.Var(&runnerArgs, "runner-arg", "传给 runner 的额外参数（可重复）")
	fs.StringVar(&cfg.program, "program", "", "LiteBox 内运行的 Linux 程序路径（必填）")
	fs.Var(&programArgs, "program-arg", "传给 program 的参数（可重复）")

	if err := fs.Parse(args); err != nil {
		return bridgeConfig{}, err
	}
	if strings.TrimSpace(cfg.runnerPath) == "" {
		return bridgeConfig{}, errors.New("--runner is required")
	}
	if strings.TrimSpace(cfg.program) == "" {
		return bridgeConfig{}, errors.New("--program is required")
	}

	cfg.runnerArgs = append([]string(nil), runnerArgs...)
	cfg.programArgs = append([]string(nil), programArgs...)
	return cfg, nil
}

func runBridge(cfg bridgeConfig, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := make([]string, 0, len(cfg.runnerArgs)+1+len(cfg.programArgs))
	args = append(args, cfg.runnerArgs...)
	args = append(args, cfg.program)
	args = append(args, cfg.programArgs...)

	cmd := exec.CommandContext(ctx, cfg.runnerPath, args...)
	cmd.Stdin = stdin

	childStdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, fmt.Errorf("create runner stdout pipe: %w", err)
	}
	childStderr, err := cmd.StderrPipe()
	if err != nil {
		return 1, fmt.Errorf("create runner stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("start runner process: %w", err)
	}

	stdoutErrCh := make(chan error, 1)
	go func() {
		stdoutErrCh <- filterStdoutLines(childStdout, stdout, stderr)
	}()

	stderrErrCh := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(stderr, childStderr)
		stderrErrCh <- copyErr
	}()

	waitErr := cmd.Wait()
	stdoutErr := <-stdoutErrCh
	stderrErr := <-stderrErrCh

	if stderrErr != nil {
		return 1, fmt.Errorf("copy runner stderr: %w", stderrErr)
	}
	if stdoutErr != nil {
		return 1, fmt.Errorf("filter runner stdout: %w", stdoutErr)
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return exitErr.ExitCode(), fmt.Errorf("runner exited with code %d", exitErr.ExitCode())
		}
		return 1, fmt.Errorf("wait runner process: %w", waitErr)
	}
	return 0, nil
}

func filterStdoutLines(r io.Reader, jsonOut io.Writer, diagOut io.Writer) error {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	w := bufio.NewWriter(jsonOut)
	for scanner.Scan() {
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if isJSONRPCLine(trimmed) {
			if _, err := w.WriteString(trimmed + "\n"); err != nil {
				return err
			}
			if err := w.Flush(); err != nil {
				return err
			}
			continue
		}

		if diagOut != nil {
			if _, err := fmt.Fprintf(diagOut, "%s%s\n", dropPrefix, raw); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return w.Flush()
}

func isJSONRPCLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	var probe struct {
		JSONRPC string `json:"jsonrpc"`
	}
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return false
	}
	return probe.JSONRPC == "2.0"
}
