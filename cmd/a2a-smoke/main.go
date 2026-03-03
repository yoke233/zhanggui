package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
)

const (
	defaultCardBaseURL = "http://127.0.0.1:8080"
	defaultPrompt      = "请回复：A2A_GO_OK"
	defaultA2AVersion  = "0.3"

	a2aVersionHeader    = "A2A-Version"
	authorizationHeader = "Authorization"
)

type smokeConfig struct {
	CardBaseURL  string
	RPCURL       string
	Prompt       string
	A2AVersion   string
	Token        string
	Timeout      time.Duration
	PollInterval time.Duration
	MaxPoll      int
	Verbose      bool
	Legacy       bool
	InsecureTLS  bool
	HTTPClient   *http.Client
}

func main() {
	var (
		cardBaseURL     string
		rpcURL          string
		prompt          string
		a2aVersion      string
		token           string
		timeout         time.Duration
		pollInterval    time.Duration
		maxPoll         int
		verbose         bool
		legacy          bool
		insecureSkipTLS bool
	)

	flag.StringVar(&cardBaseURL, "card-base-url", defaultCardBaseURL, "AgentCard 基础地址（自动请求 /.well-known/agent-card.json）")
	flag.StringVar(&rpcURL, "rpc-url", "", "A2A JSON-RPC 端点地址（优先级高于 card-base-url）")
	flag.StringVar(&prompt, "prompt", defaultPrompt, "发送给 A2A agent 的文本")
	flag.StringVar(&a2aVersion, "a2a-version", defaultA2AVersion, "请求头 A2A-Version（官方 a2a-go 当前稳定线建议 0.3）")
	flag.StringVar(&token, "token", "", "A2A Bearer Token（可选）")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "整体调用超时")
	flag.DurationVar(&pollInterval, "poll-interval", 1*time.Second, "任务轮询间隔")
	flag.IntVar(&maxPoll, "max-poll", 20, "最大轮询次数（<=0 表示不轮询）")
	flag.BoolVar(&verbose, "verbose", false, "输出更多调试信息")
	flag.BoolVar(&legacy, "legacy", false, "兼容旧用法（保留参数，等价于默认 0.3）")
	flag.BoolVar(&insecureSkipTLS, "insecure-skip-tls-verify", false, "跳过 TLS 证书校验（仅测试环境）")
	flag.Parse()

	if strings.TrimSpace(prompt) == "" {
		fmt.Fprintln(os.Stderr, "error: prompt 不能为空")
		os.Exit(2)
	}
	if legacy && strings.TrimSpace(a2aVersion) == "" {
		a2aVersion = "0.3"
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	cfg := smokeConfig{
		CardBaseURL:  cardBaseURL,
		RPCURL:       rpcURL,
		Prompt:       prompt,
		A2AVersion:   a2aVersion,
		Token:        token,
		Timeout:      timeout,
		PollInterval: pollInterval,
		MaxPoll:      maxPoll,
		Verbose:      verbose,
		Legacy:       legacy,
		InsecureTLS:  insecureSkipTLS,
		HTTPClient:   client,
	}

	if err := runSmoke(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runSmoke(parent context.Context, cfg smokeConfig, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	}

	ctx := parent
	cancel := func() {}
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, cfg.Timeout)
	}
	defer cancel()

	card, err := resolveAgentCard(ctx, cfg)
	if err != nil {
		return err
	}
	if cfg.Verbose {
		printEventJSON(out, "agent_card", card)
	}
	fmt.Fprintf(out, "rpc_url=%s\n", card.URL)
	if strings.TrimSpace(card.ProtocolVersion) != "" {
		fmt.Fprintf(out, "card_protocol_version=%s\n", strings.TrimSpace(card.ProtocolVersion))
	}

	client, err := newA2AClient(ctx, cfg, card)
	if err != nil {
		return err
	}
	defer client.Destroy()

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: cfg.Prompt})
	result, err := client.SendMessage(ctx, &a2a.MessageSendParams{
		Message: msg,
		Config: &a2a.MessageSendConfig{
			Blocking: boolPtr(true),
		},
	})
	if err != nil {
		return fmt.Errorf("send message failed: %w", err)
	}

	printEventJSON(out, "send_result", result)

	task, ok := result.(*a2a.Task)
	if !ok || cfg.MaxPoll <= 0 {
		return nil
	}
	if task.Status.State.Terminal() {
		fmt.Fprintf(out, "task_state=%s\n", task.Status.State)
		return nil
	}

	taskID := strings.TrimSpace(string(task.ID))
	if taskID == "" {
		return errors.New("send result task missing id")
	}

	for i := 0; i < cfg.MaxPoll; i++ {
		if i > 0 && cfg.PollInterval > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cfg.PollInterval):
			}
		}

		latest, getErr := client.GetTask(ctx, &a2a.TaskQueryParams{
			ID: a2a.TaskID(taskID),
		})
		if getErr != nil {
			return fmt.Errorf("get task failed task_id=%s: %w", taskID, getErr)
		}

		printEventJSON(out, fmt.Sprintf("task_result[%d]", i+1), latest)
		fmt.Fprintf(out, "task_state=%s\n", latest.Status.State)
		if latest.Status.State.Terminal() {
			return nil
		}
	}

	return fmt.Errorf("task %s did not reach terminal state after %d polls", taskID, cfg.MaxPoll)
}

func resolveAgentCard(ctx context.Context, cfg smokeConfig) (*a2a.AgentCard, error) {
	if strings.TrimSpace(cfg.RPCURL) != "" {
		card := &a2a.AgentCard{
			URL:                strings.TrimSpace(cfg.RPCURL),
			PreferredTransport: a2a.TransportProtocolJSONRPC,
			ProtocolVersion:    strings.TrimSpace(cfg.A2AVersion),
			Capabilities:       a2a.AgentCapabilities{Streaming: true},
		}
		return card, nil
	}

	baseURL := strings.TrimSpace(cfg.CardBaseURL)
	if baseURL == "" {
		return nil, errors.New("card base URL is required when rpc-url is empty")
	}

	resolver := agentcard.NewResolver(cfg.HTTPClient)
	resolveOpts := make([]agentcard.ResolveOption, 0, 1)
	if trimmed := strings.TrimSpace(cfg.A2AVersion); trimmed != "" {
		resolveOpts = append(resolveOpts, agentcard.WithRequestHeader(a2aVersionHeader, trimmed))
	}
	if authHeader, ok := bearerAuthHeader(cfg.Token); ok {
		resolveOpts = append(resolveOpts, agentcard.WithRequestHeader(authorizationHeader, authHeader))
	}

	card, err := resolver.Resolve(ctx, baseURL, resolveOpts...)
	if err != nil {
		return nil, fmt.Errorf("resolve agent card failed: %w", err)
	}
	normalizeCard(card, cfg.A2AVersion)

	if strings.TrimSpace(card.URL) == "" {
		return nil, errors.New("agent card does not contain rpc URL")
	}
	return card, nil
}

func newA2AClient(ctx context.Context, cfg smokeConfig, card *a2a.AgentCard) (*a2aclient.Client, error) {
	if card == nil {
		return nil, errors.New("agent card is required")
	}
	if strings.TrimSpace(card.URL) == "" {
		return nil, errors.New("agent card does not contain rpc URL")
	}

	opts := []a2aclient.FactoryOption{
		a2aclient.WithDefaultsDisabled(),
		a2aclient.WithJSONRPCTransport(cfg.HTTPClient),
	}
	if trimmed := strings.TrimSpace(cfg.A2AVersion); trimmed != "" {
		meta := a2aclient.CallMeta{}
		meta.Append(a2aVersionHeader, trimmed)
		if authHeader, ok := bearerAuthHeader(cfg.Token); ok {
			meta.Append(authorizationHeader, authHeader)
		}
		opts = append(opts, a2aclient.WithInterceptors(a2aclient.NewStaticCallMetaInjector(meta)))
	} else if authHeader, ok := bearerAuthHeader(cfg.Token); ok {
		meta := a2aclient.CallMeta{}
		meta.Append(authorizationHeader, authHeader)
		opts = append(opts, a2aclient.WithInterceptors(a2aclient.NewStaticCallMetaInjector(meta)))
	}

	client, err := a2aclient.NewFromCard(ctx, card, opts...)
	if err != nil {
		return nil, fmt.Errorf("create a2a client from card failed: %w", err)
	}
	return client, nil
}

func printEventJSON(out io.Writer, key string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(out, "%s=<marshal_error:%v>\n", key, err)
		return
	}
	fmt.Fprintf(out, "%s=%s\n", key, compactJSON(data))
}

func normalizeCard(card *a2a.AgentCard, fallbackVersion string) {
	if card == nil {
		return
	}

	if card.PreferredTransport == "" {
		card.PreferredTransport = a2a.TransportProtocolJSONRPC
	}
	if strings.TrimSpace(card.URL) == "" {
		for _, it := range card.AdditionalInterfaces {
			if strings.TrimSpace(it.URL) == "" {
				continue
			}
			card.URL = strings.TrimSpace(it.URL)
			if it.Transport != "" {
				card.PreferredTransport = it.Transport
			}
			break
		}
	}
	if strings.TrimSpace(card.ProtocolVersion) == "" {
		card.ProtocolVersion = strings.TrimSpace(fallbackVersion)
	}
}

func compactJSON(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	var obj any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return strings.TrimSpace(string(raw))
	}
	c, err := json.Marshal(obj)
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	return string(c)
}

func boolPtr(v bool) *bool {
	return &v
}

func bearerAuthHeader(token string) (string, bool) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "", false
	}
	if len(trimmed) > len("bearer ") && strings.EqualFold(trimmed[:len("bearer ")], "bearer ") {
		return trimmed, true
	}
	return "Bearer " + trimmed, true
}
