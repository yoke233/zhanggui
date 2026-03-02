package secretary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/user/ai-workflow/internal/acpclient"
	"github.com/user/ai-workflow/internal/core"
)

const (
	defaultTemplatePath    = "configs/prompts/secretary.tmpl"
	defaultMaxTurns        = 12
	defaultRoleID          = "plan_parser"
	defaultSecretaryRoleID = "secretary"
)

var defaultAllowedTools = []string{"Read(*)"}

// Request is the input payload for secretary decomposition.
type Request struct {
	Conversation string
	ProjectName  string
	TechStack    string
	RepoPath     string
	Role         string

	// Regeneration input fields (rules 10.3).
	OriginalConversationSummary string
	PreviousTaskPlanJSON        string
	AIReviewSummaryJSON         string
	HumanFeedbackJSON           string

	WorkDir  string
	Env      map[string]string
	MaxTurns int
	Timeout  time.Duration
}

type promptVars struct {
	Conversation                string
	ProjectName                 string
	TechStack                   string
	RepoPath                    string
	OriginalConversationSummary string
	PreviousTaskPlanJSON        string
	AIReviewSummaryJSON         string
	HumanFeedbackJSON           string
}

type taskPlanOutput struct {
	Name             string           `json:"name"`
	SpecProfile      string           `json:"spec_profile"`
	ContractVersion  string           `json:"contract_version"`
	ContractChecksum string           `json:"contract_checksum"`
	Tasks            []taskItemOutput `json:"tasks"`
}

type taskItemOutput struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	DependsOn   []string `json:"depends_on"`
	Inputs      []string `json:"inputs"`
	Outputs     []string `json:"outputs"`
	Acceptance  []string `json:"acceptance"`
	Constraints []string `json:"constraints"`
	Template    string   `json:"template"`
}

// Agent is the secretary decomposition driver based on core.AgentPlugin.
type Agent struct {
	agent        core.AgentPlugin
	runtime      core.RuntimePlugin
	promptTmpl   *template.Template
	allowedTools []string
	maxTurns     int
}

type secretarySessionClient interface {
	LoadSession(ctx context.Context, req acpclient.LoadSessionRequest) (acpclient.SessionInfo, error)
	NewSession(ctx context.Context, req acpclient.NewSessionRequest) (acpclient.SessionInfo, error)
}

func NewAgent(agent core.AgentPlugin, runtime core.RuntimePlugin) (*Agent, error) {
	return NewAgentWithTemplatePath(agent, runtime, "")
}

func NewAgentWithTemplatePath(agent core.AgentPlugin, runtime core.RuntimePlugin, templatePath string) (*Agent, error) {
	if agent == nil {
		return nil, errors.New("agent plugin is required")
	}

	content, resolvedPath, err := readTemplateContent(templatePath)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New(filepath.Base(resolvedPath)).Option("missingkey=error").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse secretary template %q: %w", resolvedPath, err)
	}

	return &Agent{
		agent:        agent,
		runtime:      runtime,
		promptTmpl:   tmpl,
		allowedTools: copyStrings(defaultAllowedTools),
		maxTurns:     defaultMaxTurns,
	}, nil
}

// BuildCommand renders prompt and delegates to core.AgentPlugin.BuildCommand.
func (a *Agent) BuildCommand(req Request) ([]string, error) {
	prompt, err := a.RenderPrompt(req)
	if err != nil {
		return nil, err
	}

	opts := a.buildExecOpts(req, prompt)
	cmd, err := a.agent.BuildCommand(opts)
	if err != nil {
		return nil, fmt.Errorf("build command: %w", err)
	}
	return cmd, nil
}

// Decompose executes secretary task decomposition and parses JSON output into TaskPlan.
func (a *Agent) Decompose(ctx context.Context, req Request) (*core.TaskPlan, error) {
	if a.runtime == nil {
		return nil, errors.New("runtime plugin is required for decompose")
	}

	prompt, err := a.RenderPrompt(req)
	if err != nil {
		return nil, err
	}

	opts := a.buildExecOpts(req, prompt)
	cmd, err := a.agent.BuildCommand(opts)
	if err != nil {
		return nil, fmt.Errorf("build command: %w", err)
	}
	log.Printf("[secretary] decompose agent=%s cmd=%v", a.agent.Name(), cmd)

	sess, err := a.runtime.Create(ctx, core.RuntimeOpts{
		WorkDir: req.WorkDir,
		Env:     copyMap(req.Env),
		Command: cmd,
	})
	if err != nil {
		return nil, fmt.Errorf("create runtime session: %w", err)
	}

	// Drain stderr in the background to avoid deadlocks when the child process
	// writes a lot of progress output to stderr (common for CLIs).
	//
	// Without this, the stderr pipe buffer can fill up, causing the child process
	// to block indefinitely and the HTTP request to "hang".
	var stderrBuf bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		// Keep only a bounded amount to avoid unbounded memory growth.
		_, _ = io.CopyN(&stderrBuf, sess.Stderr, 64<<10)
		// Drain the remaining data (if any) without buffering it.
		_, _ = io.Copy(io.Discard, sess.Stderr)
	}()

	parser := a.agent.NewStreamParser(sess.Stdout)
	rawOutput, parseErr := collectOutput(parser)
	waitErr := sess.Wait()
	<-stderrDone
	if parseErr != nil {
		return nil, parseErr
	}
	if waitErr != nil {
		stderrText := strings.TrimSpace(stderrBuf.String())
		if stderrText != "" {
			return nil, fmt.Errorf("wait session: %w (stderr: %s)", waitErr, stderrText)
		}
		return nil, fmt.Errorf("wait session: %w", waitErr)
	}

	plan, err := ParseTaskPlan(rawOutput)
	if err != nil {
		// Attach stderr to help debug agent CLI failures / prompt blocking.
		stderrText := strings.TrimSpace(stderrBuf.String())
		if stderrText != "" {
			return nil, fmt.Errorf("%w (stderr: %s)", err, stderrText)
		}
		return nil, err
	}
	return plan, nil
}

func (a *Agent) RenderPrompt(req Request) (string, error) {
	conversation := strings.TrimSpace(req.Conversation)
	originalSummary := strings.TrimSpace(req.OriginalConversationSummary)
	if originalSummary == "" {
		originalSummary = conversation
	}
	if originalSummary == "" {
		return "", errors.New("conversation is required")
	}
	if conversation == "" {
		conversation = originalSummary
	}

	vars := promptVars{
		Conversation:                conversation,
		ProjectName:                 strings.TrimSpace(req.ProjectName),
		TechStack:                   strings.TrimSpace(req.TechStack),
		RepoPath:                    strings.TrimSpace(req.RepoPath),
		OriginalConversationSummary: originalSummary,
		PreviousTaskPlanJSON:        defaultJSONPlaceholder(req.PreviousTaskPlanJSON),
		AIReviewSummaryJSON:         defaultJSONPlaceholder(req.AIReviewSummaryJSON),
		HumanFeedbackJSON:           defaultJSONPlaceholder(req.HumanFeedbackJSON),
	}

	if vars.ProjectName == "" {
		vars.ProjectName = "unknown-project"
	}
	if vars.TechStack == "" {
		vars.TechStack = "unknown"
	}
	if vars.RepoPath == "" {
		vars.RepoPath = "."
	}

	var b strings.Builder
	if err := a.promptTmpl.Execute(&b, vars); err != nil {
		return "", fmt.Errorf("render secretary template: %w", err)
	}
	return strings.TrimSpace(b.String()), nil
}

func (a *Agent) buildExecOpts(req Request, prompt string) core.ExecOpts {
	maxTurns := a.maxTurns
	if req.MaxTurns > 0 {
		maxTurns = req.MaxTurns
	}
	roleID := resolveRoleID(req.Role)

	env := copyMap(req.Env)
	// For agents that support schema-constrained outputs (e.g. Codex CLI), this
	// guides them to return strict JSON for the TaskPlan contract.
	if env == nil {
		env = make(map[string]string)
	}
	if strings.TrimSpace(env["AI_WORKFLOW_CODEX_OUTPUT_SCHEMA"]) == "" && strings.TrimSpace(req.WorkDir) != "" {
		env["AI_WORKFLOW_CODEX_OUTPUT_SCHEMA"] = filepath.Join(req.WorkDir, "configs", "schemas", "task_plan_v1.schema.json")
	}

	return core.ExecOpts{
		Prompt:        prompt,
		WorkDir:       req.WorkDir,
		AllowedTools:  copyStrings(a.allowedTools),
		MaxTurns:      maxTurns,
		Timeout:       req.Timeout,
		Env:           env,
		AppendContext: roleContextJSON(roleID),
	}
}

// ParseTaskPlan parses strict JSON output into core.TaskPlan.
func ParseTaskPlan(rawOutput string) (*core.TaskPlan, error) {
	jsonText, err := extractJSONObject(rawOutput)
	if err != nil {
		return nil, fmt.Errorf("extract json: %w", err)
	}

	var out taskPlanOutput
	dec := json.NewDecoder(strings.NewReader(jsonText))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode task plan: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("task plan JSON contains trailing content")
	}

	name := strings.TrimSpace(out.Name)
	if name == "" {
		return nil, errors.New("task plan name is required")
	}

	plan := &core.TaskPlan{
		Name:             name,
		Status:           core.PlanDraft,
		WaitReason:       core.WaitNone,
		FailPolicy:       core.FailBlock,
		SpecProfile:      strings.TrimSpace(out.SpecProfile),
		ContractVersion:  strings.TrimSpace(out.ContractVersion),
		ContractChecksum: strings.TrimSpace(out.ContractChecksum),
		Tasks:            make([]core.TaskItem, 0, len(out.Tasks)),
	}
	if plan.SpecProfile == "" {
		plan.SpecProfile = "default"
	}
	if plan.ContractVersion == "" {
		plan.ContractVersion = "v1"
	}

	for i, task := range out.Tasks {
		item, err := toTaskItem(task)
		if err != nil {
			return nil, fmt.Errorf("task %d: %w", i+1, err)
		}
		plan.Tasks = append(plan.Tasks, item)
	}
	return plan, nil
}

func toTaskItem(task taskItemOutput) (core.TaskItem, error) {
	item := core.TaskItem{
		ID:          strings.TrimSpace(task.ID),
		Title:       strings.TrimSpace(task.Title),
		Description: strings.TrimSpace(task.Description),
		Labels:      compactStrings(task.Labels),
		DependsOn:   compactStrings(task.DependsOn),
		Inputs:      compactStrings(task.Inputs),
		Outputs:     compactStrings(task.Outputs),
		Acceptance:  compactStrings(task.Acceptance),
		Constraints: compactStrings(task.Constraints),
		Template:    strings.TrimSpace(task.Template),
		Status:      core.ItemPending,
	}
	if item.ID == "" {
		return core.TaskItem{}, errors.New("id is required")
	}
	if item.Title == "" {
		return core.TaskItem{}, errors.New("title is required")
	}
	if item.Template == "" {
		item.Template = "standard"
	}
	if !isValidTemplate(item.Template) {
		return core.TaskItem{}, fmt.Errorf("invalid template %q", item.Template)
	}
	if err := item.Validate(true); err != nil {
		return core.TaskItem{}, err
	}
	return item, nil
}

func isValidTemplate(value string) bool {
	switch value {
	case "full", "standard", "quick", "hotfix":
		return true
	default:
		return false
	}
}

func collectOutput(parser core.StreamParser) (string, error) {
	var textChunks []string
	resultChunk := ""

	for {
		evt, err := parser.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("parse stream output: %w", err)
		}
		if evt == nil {
			continue
		}

		content := strings.TrimSpace(evt.Content)
		if content == "" {
			continue
		}
		switch evt.Type {
		case "done":
			resultChunk = content
		case "text":
			textChunks = append(textChunks, content)
		}
	}

	if resultChunk != "" {
		return resultChunk, nil
	}
	if len(textChunks) > 0 {
		return strings.Join(textChunks, "\n"), nil
	}
	return "", errors.New("agent returned empty output")
}

func extractJSONObject(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", errors.New("empty output")
	}

	text = stripCodeFence(text)
	if json.Valid([]byte(text)) {
		return text, nil
	}

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return "", errors.New("json object not found")
	}

	candidate := strings.TrimSpace(text[start : end+1])
	if !json.Valid([]byte(candidate)) {
		return "", errors.New("invalid json object")
	}
	return candidate, nil
}

func stripCodeFence(text string) string {
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return text
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		return text
	}

	end := -1
	for i := len(lines) - 1; i >= 1; i-- {
		if strings.TrimSpace(lines[i]) == "```" {
			end = i
			break
		}
	}
	if end <= 1 {
		return text
	}
	return strings.TrimSpace(strings.Join(lines[1:end], "\n"))
}

func readTemplateContent(explicitPath string) ([]byte, string, error) {
	candidates := make([]string, 0, 3)
	if strings.TrimSpace(explicitPath) != "" {
		candidates = append(candidates, explicitPath)
	}
	candidates = append(candidates,
		defaultTemplatePath,
		filepath.Join("..", "..", defaultTemplatePath),
	)

	seen := map[string]struct{}{}
	var errs []string
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		data, err := os.ReadFile(path)
		if err == nil {
			return data, path, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", path, err))
	}

	return nil, "", fmt.Errorf("secretary template not found (%s)", strings.Join(errs, "; "))
}

func defaultJSONPlaceholder(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "{}"
	}
	return trimmed
}

func resolveRoleID(role string) string {
	trimmed := strings.TrimSpace(role)
	if trimmed == "" {
		return defaultRoleID
	}
	return trimmed
}

func resolveSecretaryRoleID(explicitRole, boundRole string) string {
	if trimmed := strings.TrimSpace(explicitRole); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(boundRole); trimmed != "" {
		return trimmed
	}
	return defaultSecretaryRoleID
}

func startSecretarySession(
	ctx context.Context,
	client secretarySessionClient,
	resolver *acpclient.RoleResolver,
	explicitRole string,
	boundRole string,
	persistedSessionID string,
	cwd string,
	mcpServers []acpclient.MCPServerConfig,
) (acpclient.SessionInfo, string, error) {
	if client == nil {
		return acpclient.SessionInfo{}, "", errors.New("secretary session client is required")
	}

	roleID := resolveSecretaryRoleID(explicitRole, boundRole)
	resolvedRole := acpclient.RoleProfile{}
	if resolver != nil {
		_, role, err := resolver.Resolve(roleID)
		if err != nil {
			return acpclient.SessionInfo{}, "", fmt.Errorf("resolve secretary role %q: %w", roleID, err)
		}
		resolvedRole = role
	}

	trimmedCWD := strings.TrimSpace(cwd)
	metadata := map[string]string{
		"role_id": roleID,
	}
	if sessionID := strings.TrimSpace(persistedSessionID); shouldLoadPersistedSecretarySession(resolvedRole.SessionPolicy, sessionID) {
		loaded, err := client.LoadSession(ctx, acpclient.LoadSessionRequest{
			SessionID: sessionID,
			CWD:       trimmedCWD,
			Metadata:  metadata,
		})
		if err == nil && strings.TrimSpace(loaded.SessionID) != "" {
			return loaded, roleID, nil
		}
	}

	effectiveMCPServers := append([]acpclient.MCPServerConfig(nil), mcpServers...)
	if len(effectiveMCPServers) == 0 {
		effectiveMCPServers = MCPToolsFromRoleConfig(resolvedRole)
	}

	session, err := client.NewSession(ctx, acpclient.NewSessionRequest{
		CWD:        trimmedCWD,
		MCPServers: effectiveMCPServers,
		Metadata:   metadata,
	})
	if err != nil {
		return acpclient.SessionInfo{}, "", err
	}
	return session, roleID, nil
}

func shouldLoadPersistedSecretarySession(policy acpclient.SessionPolicy, persistedSessionID string) bool {
	if strings.TrimSpace(persistedSessionID) == "" {
		return false
	}
	if !policy.Reuse {
		return false
	}
	if !policy.PreferLoadSession {
		return false
	}
	return true
}

func roleContextJSON(roleID string) string {
	payload, err := json.Marshal(map[string]string{
		"role_id": roleID,
	})
	if err != nil {
		return fmt.Sprintf(`{"role_id":%q}`, defaultRoleID)
	}
	return string(payload)
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func copyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func copyMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
