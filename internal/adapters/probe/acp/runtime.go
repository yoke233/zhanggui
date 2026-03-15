package acp

import (
	"context"
	"fmt"
	"strings"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acp"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
)

type Target struct {
	Launch     acpclient.LaunchConfig
	Caps       acpclient.ClientCapabilities
	WorkDir    string
	MCPServers []acpproto.McpServer
	SessionID  acpproto.SessionId
	Question   string
	Timeout    time.Duration
}

func Run(ctx context.Context, target Target) (*probeapp.RunProbeRuntimeResult, error) {
	if strings.TrimSpace(string(target.SessionID)) == "" {
		return &probeapp.RunProbeRuntimeResult{
			Reachable:  false,
			Error:      "missing session route",
			ObservedAt: time.Now().UTC(),
		}, nil
	}
	if strings.TrimSpace(target.Question) == "" {
		return &probeapp.RunProbeRuntimeResult{
			Reachable:  false,
			Error:      "probe question is required",
			ObservedAt: time.Now().UTC(),
		}, nil
	}

	probeCtx := ctx
	cancel := func() {}
	if target.Timeout > 0 {
		probeCtx, cancel = context.WithTimeout(ctx, target.Timeout)
	}
	defer cancel()

	handler := acphandler.NewACPHandler(target.WorkDir, "", nil)
	handler.SetSuppressEvents(true)
	client, err := acpclient.New(target.Launch, handler)
	if err != nil {
		return &probeapp.RunProbeRuntimeResult{
			Reachable:  false,
			Error:      fmt.Sprintf("launch probe client: %v", err),
			ObservedAt: time.Now().UTC(),
		}, nil
	}
	defer client.Close(context.Background())

	if err := client.Initialize(probeCtx, target.Caps); err != nil {
		return &probeapp.RunProbeRuntimeResult{
			Reachable:  false,
			Error:      fmt.Sprintf("initialize probe client: %v", err),
			ObservedAt: time.Now().UTC(),
		}, nil
	}

	loadedSessionID, err := client.LoadSession(probeCtx, acpproto.LoadSessionRequest{
		SessionId:  target.SessionID,
		Cwd:        target.WorkDir,
		McpServers: target.MCPServers,
	})
	if err != nil {
		return &probeapp.RunProbeRuntimeResult{
			Reachable:  false,
			Error:      fmt.Sprintf("load probe session: %v", err),
			ObservedAt: time.Now().UTC(),
		}, nil
	}
	handler.SetSessionID(string(loadedSessionID))

	result, err := client.PromptText(probeCtx, loadedSessionID, target.Question)
	observedAt := time.Now().UTC()
	if err != nil {
		if probeCtx.Err() == context.DeadlineExceeded || ctx.Err() == context.DeadlineExceeded {
			return &probeapp.RunProbeRuntimeResult{
				Reachable:  true,
				Answered:   false,
				Error:      "probe timeout",
				ObservedAt: observedAt,
			}, nil
		}
		return &probeapp.RunProbeRuntimeResult{
			Reachable:  true,
			Answered:   false,
			Error:      fmt.Sprintf("probe prompt failed: %v", err),
			ObservedAt: observedAt,
		}, nil
	}

	return &probeapp.RunProbeRuntimeResult{
		Reachable:  true,
		Answered:   true,
		ReplyText:  strings.TrimSpace(result.Text),
		ObservedAt: observedAt,
	}, nil
}
