//go:build agentsdk

package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"time"

	agentsdkacp "github.com/cexll/agentsdk-go/pkg/acp"
	agentsdkapi "github.com/cexll/agentsdk-go/pkg/api"
	agentsdkmodel "github.com/cexll/agentsdk-go/pkg/model"
	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
)

const (
	agentsdkInprocLaunchCommand = "agentsdk-inproc"
	defaultAnthropicModel       = "claude-3-5-sonnet-20241022"
	defaultOpenAIModel          = "gpt-4o-mini"
)

func isAgentSDKInprocLaunch(command string) bool {
	return strings.EqualFold(strings.TrimSpace(command), agentsdkInprocLaunchCommand)
}

func newAgentSDKInprocClient(
	ctx context.Context,
	cfg acpclient.LaunchConfig,
	handler acpproto.Client,
	capabilities acpclient.ClientCapabilities,
	opts ...acpclient.Option,
) (ChatACPClient, error) {
	serverOptions, err := buildAgentSDKOptions(cfg)
	if err != nil {
		return nil, err
	}

	serverConn, clientConn := net.Pipe()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- agentsdkacp.ServeStdio(serverCtx, serverOptions, serverConn, serverConn)
	}()

	closeHook := func(closeCtx context.Context) error {
		serverCancel()
		_ = serverConn.Close()
		_ = clientConn.Close()
		select {
		case err := <-serverErrCh:
			if isBenignAgentSDKServeError(err) {
				return nil
			}
			return err
		case <-closeCtx.Done():
			return closeCtx.Err()
		}
	}

	allOpts := append([]acpclient.Option{}, opts...)
	allOpts = append(allOpts, acpclient.WithCloseHook(closeHook))
	client, err := acpclient.NewWithIO(cfg, handler, clientConn, clientConn, allOpts...)
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = closeHook(closeCtx)
		return nil, fmt.Errorf("create inproc acp client: %w", err)
	}
	if err := client.Initialize(ctx, capabilities); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = client.Close(closeCtx)
		return nil, err
	}
	return client, nil
}

func buildAgentSDKOptions(cfg acpclient.LaunchConfig) (agentsdkapi.Options, error) {
	env := cloneStringMap(cfg.Env)
	projectRoot := strings.TrimSpace(cfg.WorkDir)
	if projectRoot == "" {
		return agentsdkapi.Options{}, errors.New("workdir is required for agentsdk-inproc")
	}

	absProjectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return agentsdkapi.Options{}, fmt.Errorf("resolve absolute project root: %w", err)
	}

	settingsPath := strings.TrimSpace(env["AGENTSDK_SETTINGS_PATH"])
	if settingsPath == "" {
		if claudeDir := strings.TrimSpace(env["AGENTSDK_CLAUDE_DIR"]); claudeDir != "" {
			settingsPath = filepath.Join(claudeDir, "settings.json")
		}
	}
	if settingsPath != "" && !filepath.IsAbs(settingsPath) {
		settingsPath = filepath.Join(absProjectRoot, settingsPath)
	}

	entryPoint := agentsdkapi.EntryPoint(strings.ToLower(strings.TrimSpace(env["AGENTSDK_ENTRYPOINT"])))
	if entryPoint == "" {
		entryPoint = agentsdkapi.EntryPointCLI
	}

	modelFactory, err := buildAgentSDKModelFactory(env)
	if err != nil {
		return agentsdkapi.Options{}, err
	}

	return agentsdkapi.Options{
		EntryPoint:   entryPoint,
		ProjectRoot:  absProjectRoot,
		SettingsPath: settingsPath,
		ModelFactory: modelFactory,
	}, nil
}

func buildAgentSDKModelFactory(env map[string]string) (agentsdkapi.ModelFactory, error) {
	provider := strings.ToLower(strings.TrimSpace(env["AGENTSDK_MODEL_PROVIDER"]))
	if provider == "" {
		provider = "anthropic"
	}

	switch provider {
	case "anthropic", "claude":
		modelName := firstNonEmptyTrimmed(env["AGENTSDK_MODEL"], defaultAnthropicModel)
		return &agentsdkmodel.AnthropicProvider{
			APIKey:    firstNonEmptyTrimmed(env["ANTHROPIC_API_KEY"], env["ANTHROPIC_AUTH_TOKEN"]),
			BaseURL:   strings.TrimSpace(env["ANTHROPIC_BASE_URL"]),
			ModelName: modelName,
			System:    strings.TrimSpace(env["AGENTSDK_SYSTEM_PROMPT"]),
		}, nil
	case "openai":
		modelName := firstNonEmptyTrimmed(env["AGENTSDK_MODEL"], defaultOpenAIModel)
		return &agentsdkmodel.OpenAIProvider{
			APIKey:    strings.TrimSpace(env["OPENAI_API_KEY"]),
			BaseURL:   strings.TrimSpace(env["OPENAI_BASE_URL"]),
			ModelName: modelName,
			System:    strings.TrimSpace(env["AGENTSDK_SYSTEM_PROMPT"]),
		}, nil
	case "stub", "fixed":
		reply := firstNonEmptyTrimmed(env["AGENTSDK_STUB_RESPONSE"], "ok")
		return agentsdkapi.ModelFactoryFunc(func(context.Context) (agentsdkmodel.Model, error) {
			return fixedAgentSDKModel{reply: reply}, nil
		}), nil
	default:
		return nil, fmt.Errorf("unsupported AGENTSDK_MODEL_PROVIDER %q", provider)
	}
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isBenignAgentSDKServeError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "closed network connection") || strings.Contains(lowered, "closed pipe")
}

type fixedAgentSDKModel struct {
	reply string
}

func (m fixedAgentSDKModel) Complete(context.Context, agentsdkmodel.Request) (*agentsdkmodel.Response, error) {
	reply := firstNonEmptyTrimmed(m.reply, "ok")
	return &agentsdkmodel.Response{
		Message: agentsdkmodel.Message{
			Role:    "assistant",
			Content: reply,
		},
		StopReason: "end_turn",
	}, nil
}

func (m fixedAgentSDKModel) CompleteStream(_ context.Context, _ agentsdkmodel.Request, cb agentsdkmodel.StreamHandler) error {
	if cb == nil {
		return nil
	}
	reply := firstNonEmptyTrimmed(m.reply, "ok")
	for _, ch := range reply {
		if err := cb(agentsdkmodel.StreamResult{Delta: string(ch)}); err != nil {
			return err
		}
	}
	return cb(agentsdkmodel.StreamResult{
		Final: true,
		Response: &agentsdkmodel.Response{
			Message: agentsdkmodel.Message{
				Role:    "assistant",
				Content: reply,
			},
			StopReason: "end_turn",
		},
	})
}
