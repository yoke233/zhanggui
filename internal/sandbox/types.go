package sandbox

import "context"

type RunSpec struct {
	TaskID     string
	RunID      string
	Rev        string
	OutputDir  string
	Entrypoint []string
}

type Result struct {
	Mode     string `json:"mode"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

type Runner interface {
	Run(ctx context.Context, spec RunSpec) (Result, error)
}
