package secretary

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/user/ai-workflow/internal/acpclient"
	"github.com/user/ai-workflow/internal/core"
)

type mockAgent struct {
	opts   []core.ExecOpts
	cmd    []string
	parser core.StreamParser
}

func (a *mockAgent) Name() string { return "mock-agent" }

func (a *mockAgent) Init(context.Context) error { return nil }

func (a *mockAgent) Close() error { return nil }

func (a *mockAgent) BuildCommand(opts core.ExecOpts) ([]string, error) {
	a.opts = append(a.opts, opts)
	if len(a.cmd) == 0 {
		return []string{"mock"}, nil
	}
	return a.cmd, nil
}

func (a *mockAgent) NewStreamParser(io.Reader) core.StreamParser {
	return a.parser
}

type fakeRuntime struct {
	lastOpts core.RuntimeOpts
	session  *core.Session
}

func (r *fakeRuntime) Name() string { return "fake-runtime" }

func (r *fakeRuntime) Init(context.Context) error { return nil }

func (r *fakeRuntime) Close() error { return nil }

func (r *fakeRuntime) Kill(string) error { return nil }

func (r *fakeRuntime) Create(_ context.Context, opts core.RuntimeOpts) (*core.Session, error) {
	r.lastOpts = opts
	return r.session, nil
}

type sliceParser struct {
	events []*core.StreamEvent
	index  int
}

func (p *sliceParser) Next() (*core.StreamEvent, error) {
	if p.index >= len(p.events) {
		return nil, io.EOF
	}
	evt := p.events[p.index]
	p.index++
	return evt, nil
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(data []byte) (int, error) { return len(data), nil }

func (nopWriteCloser) Close() error { return nil }

type stubSecretarySessionClient struct {
	loadReqs []acpclient.LoadSessionRequest
	newReqs  []acpclient.NewSessionRequest
	calls    []string
	loadResp acpclient.SessionInfo
	loadErr  error
	newResp  acpclient.SessionInfo
	newErr   error
}

func (c *stubSecretarySessionClient) LoadSession(_ context.Context, req acpclient.LoadSessionRequest) (acpclient.SessionInfo, error) {
	c.calls = append(c.calls, "load")
	c.loadReqs = append(c.loadReqs, req)
	if c.loadErr != nil {
		return acpclient.SessionInfo{}, c.loadErr
	}
	return c.loadResp, nil
}

func (c *stubSecretarySessionClient) NewSession(_ context.Context, req acpclient.NewSessionRequest) (acpclient.SessionInfo, error) {
	c.calls = append(c.calls, "new")
	c.newReqs = append(c.newReqs, req)
	if c.newErr != nil {
		return acpclient.SessionInfo{}, c.newErr
	}
	return c.newResp, nil
}

func TestAgentDecomposeBuildsPromptAndParsesTaskPlan(t *testing.T) {
	waitCalled := false
	runtime := &fakeRuntime{
		session: &core.Session{
			ID:     "session-1",
			Stdin:  nopWriteCloser{},
			Stdout: strings.NewReader(""),
			Stderr: strings.NewReader(""),
			Wait: func() error {
				waitCalled = true
				return nil
			},
		},
	}

	output := "```json\n{\n  \"name\": \"oauth-rollout\",\n  \"tasks\": [\n    {\n      \"id\": \"task-1\",\n      \"title\": \"后端接入 OAuth\",\n      \"description\": \"完成 OAuth 登录接口并补充单测。\",\n      \"labels\": [\"backend\", \"auth\"],\n      \"depends_on\": [],\n      \"inputs\": [\"oauth_app_id\", \"oauth_secret\"],\n      \"outputs\": [\"oauth_login_api\"],\n      \"acceptance\": [\"valid callback returns 200\"],\n      \"constraints\": [\"保持现有用户表结构\"],\n      \"template\": \"standard\"\n    },\n    {\n      \"id\": \"task-2\",\n      \"title\": \"审计日志落库\",\n      \"description\": \"记录登录审计日志并提供查询接口。\",\n      \"labels\": [\"backend\", \"database\"],\n      \"depends_on\": [\"task-1\"],\n      \"inputs\": [\"oauth_user_id\"],\n      \"outputs\": [\"audit_log_query_api\"],\n      \"acceptance\": [\"audit log query works\"],\n      \"constraints\": [\"最小化写放大\"],\n      \"template\": \"full\"\n    }\n  ]\n}\n```"
	agent := &mockAgent{
		cmd: []string{"mock-secretary"},
		parser: &sliceParser{
			events: []*core.StreamEvent{
				{Type: "done", Content: output},
			},
		},
	}

	templatePath := filepath.Join("..", "..", "configs", "prompts", "secretary.tmpl")
	driver, err := NewAgentWithTemplatePath(agent, runtime, templatePath)
	if err != nil {
		t.Fatalf("new secretary agent: %v", err)
	}

	req := Request{
		Conversation:                "用户希望新增 OAuth 登录并补充审计日志。",
		ProjectName:                 "ai-workflow",
		TechStack:                   "Go + SQLite",
		RepoPath:                    "D:/project/ai-workflow",
		OriginalConversationSummary: "用户希望增加 OAuth 登录与审计日志能力。",
		PreviousTaskPlanJSON:        `{"name":"oauth-v1","tasks":[{"id":"task-1","title":"旧任务"}]}`,
		AIReviewSummaryJSON:         `{"rounds":2,"last_decision":"fix","top_issues":["coverage_gap"]}`,
		HumanFeedbackJSON:           `{"category":"coverage_gap","detail":"上一版遗漏了审计日志相关任务","expected_direction":"补齐日志任务并明确依赖"}`,
		WorkDir:                     "D:/project/ai-workflow",
	}

	plan, err := driver.Decompose(context.Background(), req)
	if err != nil {
		t.Fatalf("decompose failed: %v", err)
	}

	if !waitCalled {
		t.Fatal("session.Wait must be called")
	}
	if len(agent.opts) != 1 {
		t.Fatalf("BuildCommand should be called once, got %d", len(agent.opts))
	}
	if agent.opts[0].WorkDir != req.WorkDir {
		t.Fatalf("exec opts workdir mismatch, got %q", agent.opts[0].WorkDir)
	}
	if agent.opts[0].MaxTurns <= 0 {
		t.Fatalf("max turns should be set, got %d", agent.opts[0].MaxTurns)
	}
	if !reflect.DeepEqual(agent.opts[0].AllowedTools, []string{"Read(*)"}) {
		t.Fatalf("allowed tools mismatch: %#v", agent.opts[0].AllowedTools)
	}

	prompt := agent.opts[0].Prompt
	for _, s := range []string{
		"输入 1：原始对话摘要",
		"输入 2：上一版 TaskPlan（完整 JSON）",
		"输入 3：AI review 问题摘要（结构化）",
		"输入 4：人类反馈（标准化 JSON）",
		req.OriginalConversationSummary,
		req.PreviousTaskPlanJSON,
		req.AIReviewSummaryJSON,
		req.HumanFeedbackJSON,
		req.Conversation,
		req.ProjectName,
		req.TechStack,
		req.RepoPath,
	} {
		if !strings.Contains(prompt, s) {
			t.Fatalf("prompt must include %q, got:\n%s", s, prompt)
		}
	}

	if runtime.lastOpts.WorkDir != req.WorkDir {
		t.Fatalf("runtime workdir mismatch, got %q", runtime.lastOpts.WorkDir)
	}
	if !reflect.DeepEqual(runtime.lastOpts.Command, []string{"mock-secretary"}) {
		t.Fatalf("runtime command mismatch: %#v", runtime.lastOpts.Command)
	}

	if plan.Name != "oauth-rollout" {
		t.Fatalf("unexpected plan name: %q", plan.Name)
	}
	if plan.Status != core.PlanDraft {
		t.Fatalf("expected status draft, got %q", plan.Status)
	}
	if plan.FailPolicy != core.FailBlock {
		t.Fatalf("expected fail_policy block, got %q", plan.FailPolicy)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(plan.Tasks))
	}
	if plan.Tasks[0].ID != "task-1" || plan.Tasks[0].Template != "standard" {
		t.Fatalf("unexpected first task: %#v", plan.Tasks[0])
	}
	if plan.Tasks[1].ID != "task-2" || plan.Tasks[1].Template != "full" {
		t.Fatalf("unexpected second task: %#v", plan.Tasks[1])
	}
	if plan.Tasks[1].Status != core.ItemPending {
		t.Fatalf("expected pending status, got %q", plan.Tasks[1].Status)
	}
}

func TestPlanParserUsesRoleBinding(t *testing.T) {
	agent := &mockAgent{}
	templatePath := filepath.Join("..", "..", "configs", "prompts", "secretary.tmpl")
	driver, err := NewAgentWithTemplatePath(agent, nil, templatePath)
	if err != nil {
		t.Fatalf("new secretary agent: %v", err)
	}

	defaultReq := Request{
		Conversation: "请拆解当前任务",
		WorkDir:      "D:/project/ai-workflow",
	}
	if _, err := driver.BuildCommand(defaultReq); err != nil {
		t.Fatalf("build command with default role: %v", err)
	}
	if len(agent.opts) != 1 {
		t.Fatalf("expected 1 exec opts, got %d", len(agent.opts))
	}
	assertRoleID(t, agent.opts[0].AppendContext, "plan_parser")

	overrideReq := defaultReq
	overrideReq.Role = "custom_role"
	if _, err := driver.BuildCommand(overrideReq); err != nil {
		t.Fatalf("build command with custom role: %v", err)
	}
	if len(agent.opts) != 2 {
		t.Fatalf("expected 2 exec opts, got %d", len(agent.opts))
	}
	assertRoleID(t, agent.opts[1].AppendContext, "custom_role")
}

func TestSecretaryUsesBoundRole(t *testing.T) {
	resolver := acpclient.NewRoleResolver(
		[]acpclient.AgentProfile{
			{
				ID: "codex",
				CapabilitiesMax: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
		[]acpclient.RoleProfile{
			{
				ID:      "secretary_custom",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
				SessionPolicy: acpclient.SessionPolicy{
					Reuse:             true,
					PreferLoadSession: true,
				},
				MCPTools: []string{"query_plans"},
			},
		},
	)
	client := &stubSecretarySessionClient{
		loadErr: errors.New("session not found"),
		newResp: acpclient.SessionInfo{SessionID: "sid-new"},
	}

	session, roleID, err := startSecretarySession(
		context.Background(),
		client,
		resolver,
		"",
		"secretary_custom",
		"sid-old",
		"D:/project/ai-workflow",
		nil,
	)
	if err != nil {
		t.Fatalf("startSecretarySession() error = %v", err)
	}
	if roleID != "secretary_custom" {
		t.Fatalf("role id = %q, want %q", roleID, "secretary_custom")
	}
	if session.SessionID != "sid-new" {
		t.Fatalf("session id = %q, want %q", session.SessionID, "sid-new")
	}
	if !reflect.DeepEqual(client.calls, []string{"load", "new"}) {
		t.Fatalf("call order = %#v, want load->new fallback", client.calls)
	}
	if len(client.loadReqs) != 1 {
		t.Fatalf("LoadSession calls = %d, want 1", len(client.loadReqs))
	}
	if len(client.newReqs) != 1 {
		t.Fatalf("NewSession calls = %d, want 1", len(client.newReqs))
	}
	if got := client.loadReqs[0].Metadata["role_id"]; got != "secretary_custom" {
		t.Fatalf("load metadata role_id = %q, want %q", got, "secretary_custom")
	}
	if got := client.newReqs[0].Metadata["role_id"]; got != "secretary_custom" {
		t.Fatalf("new metadata role_id = %q, want %q", got, "secretary_custom")
	}
	if len(client.newReqs[0].MCPServers) != 1 {
		t.Fatalf("new session mcp servers = %d, want 1 from role config", len(client.newReqs[0].MCPServers))
	}
	if got := client.newReqs[0].MCPServers[0].Env["AI_WORKFLOW_MCP_TOOL"]; got != "query_plans" {
		t.Fatalf("new session mcp tool = %q, want %q", got, "query_plans")
	}
}

func TestStartSecretarySessionSkipsLoadWhenReuseDisabled(t *testing.T) {
	resolver := acpclient.NewRoleResolver(
		[]acpclient.AgentProfile{
			{
				ID: "codex",
				CapabilitiesMax: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
		[]acpclient.RoleProfile{
			{
				ID:      "secretary_custom",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
				SessionPolicy: acpclient.SessionPolicy{
					Reuse:             false,
					PreferLoadSession: true,
				},
			},
		},
	)
	client := &stubSecretarySessionClient{
		loadResp: acpclient.SessionInfo{SessionID: "sid-loaded"},
		newResp:  acpclient.SessionInfo{SessionID: "sid-new"},
	}

	session, roleID, err := startSecretarySession(
		context.Background(),
		client,
		resolver,
		"",
		"secretary_custom",
		"sid-old",
		"D:/project/ai-workflow",
		nil,
	)
	if err != nil {
		t.Fatalf("startSecretarySession() error = %v", err)
	}
	if roleID != "secretary_custom" {
		t.Fatalf("role id = %q, want %q", roleID, "secretary_custom")
	}
	if session.SessionID != "sid-new" {
		t.Fatalf("session id = %q, want %q", session.SessionID, "sid-new")
	}
	if len(client.loadReqs) != 0 {
		t.Fatalf("LoadSession calls = %d, want 0", len(client.loadReqs))
	}
	if len(client.newReqs) != 1 {
		t.Fatalf("NewSession calls = %d, want 1", len(client.newReqs))
	}
}

func TestStartSecretarySessionSkipsLoadWhenPreferLoadDisabled(t *testing.T) {
	resolver := acpclient.NewRoleResolver(
		[]acpclient.AgentProfile{
			{
				ID: "codex",
				CapabilitiesMax: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
		[]acpclient.RoleProfile{
			{
				ID:      "secretary_custom",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
				SessionPolicy: acpclient.SessionPolicy{
					Reuse:             true,
					PreferLoadSession: false,
				},
			},
		},
	)
	client := &stubSecretarySessionClient{
		loadResp: acpclient.SessionInfo{SessionID: "sid-loaded"},
		newResp:  acpclient.SessionInfo{SessionID: "sid-new"},
	}

	session, roleID, err := startSecretarySession(
		context.Background(),
		client,
		resolver,
		"",
		"secretary_custom",
		"sid-old",
		"D:/project/ai-workflow",
		nil,
	)
	if err != nil {
		t.Fatalf("startSecretarySession() error = %v", err)
	}
	if roleID != "secretary_custom" {
		t.Fatalf("role id = %q, want %q", roleID, "secretary_custom")
	}
	if session.SessionID != "sid-new" {
		t.Fatalf("session id = %q, want %q", session.SessionID, "sid-new")
	}
	if len(client.loadReqs) != 0 {
		t.Fatalf("LoadSession calls = %d, want 0", len(client.loadReqs))
	}
	if len(client.newReqs) != 1 {
		t.Fatalf("NewSession calls = %d, want 1", len(client.newReqs))
	}
}

func TestParseTaskPlanRejectsInvalidTemplate(t *testing.T) {
	_, err := ParseTaskPlan(`{
  "name": "invalid-plan",
  "tasks": [
    {
      "id": "task-1",
      "title": "bad template",
      "description": "this should fail",
      "labels": ["backend"],
      "depends_on": [],
      "template": "unsupported-template"
    }
  ]
}`)
	if err == nil {
		t.Fatal("expected parse error for invalid template")
	}
	if !strings.Contains(err.Error(), "invalid template") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTaskPlan_IncludesInputsOutputsAcceptance(t *testing.T) {
	plan, err := ParseTaskPlan(`{
  "name": "structured-plan",
  "tasks": [
    {
      "id": "task-1",
      "title": "design contract",
      "description": "define io contract",
      "labels": ["backend"],
      "depends_on": [],
      "inputs": ["oauth_app_id"],
      "outputs": ["oauth_token"],
      "acceptance": ["callback endpoint returns 200"],
      "constraints": ["keep api backward compatible"],
      "template": "standard"
    }
  ]
}`)
	if err != nil {
		t.Fatalf("parse task plan: %v", err)
	}

	if len(plan.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(plan.Tasks))
	}
	task := plan.Tasks[0]
	if len(task.Inputs) != 1 || task.Inputs[0] != "oauth_app_id" {
		t.Fatalf("unexpected inputs: %#v", task.Inputs)
	}
	if len(task.Outputs) != 1 || task.Outputs[0] != "oauth_token" {
		t.Fatalf("unexpected outputs: %#v", task.Outputs)
	}
	if len(task.Acceptance) != 1 || task.Acceptance[0] != "callback endpoint returns 200" {
		t.Fatalf("unexpected acceptance: %#v", task.Acceptance)
	}
	if len(task.Constraints) != 1 || task.Constraints[0] != "keep api backward compatible" {
		t.Fatalf("unexpected constraints: %#v", task.Constraints)
	}
}

func TestToTaskItem_MapsStructuredFields(t *testing.T) {
	item, err := toTaskItem(taskItemOutput{
		ID:          "task-2",
		Title:       "implement oauth",
		Description: "implement oauth endpoint",
		Labels:      []string{"backend"},
		DependsOn:   []string{"task-1"},
		Inputs:      []string{"oauth_app_id"},
		Outputs:     []string{"oauth_token"},
		Acceptance:  []string{"endpoint returns 200"},
		Constraints: []string{"no breaking changes"},
		Template:    "standard",
	})
	if err != nil {
		t.Fatalf("toTaskItem: %v", err)
	}

	if len(item.Inputs) != 1 || item.Inputs[0] != "oauth_app_id" {
		t.Fatalf("inputs mapping failed: %#v", item.Inputs)
	}
	if len(item.Outputs) != 1 || item.Outputs[0] != "oauth_token" {
		t.Fatalf("outputs mapping failed: %#v", item.Outputs)
	}
	if len(item.Acceptance) != 1 || item.Acceptance[0] != "endpoint returns 200" {
		t.Fatalf("acceptance mapping failed: %#v", item.Acceptance)
	}
	if len(item.Constraints) != 1 || item.Constraints[0] != "no breaking changes" {
		t.Fatalf("constraints mapping failed: %#v", item.Constraints)
	}
}

func assertRoleID(t *testing.T, appendContext, wantRole string) {
	t.Helper()

	if strings.TrimSpace(appendContext) == "" {
		t.Fatal("append context should not be empty")
	}

	payload := map[string]string{}
	if err := json.Unmarshal([]byte(appendContext), &payload); err != nil {
		t.Fatalf("append context should be json: %v", err)
	}

	if payload["role_id"] != wantRole {
		t.Fatalf("unexpected role_id, got %q want %q", payload["role_id"], wantRole)
	}
}
