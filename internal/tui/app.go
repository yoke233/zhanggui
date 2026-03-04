package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/tui/views"
)

type RunExecutor interface {
	CreateRun(projectID, name, description, template string) (*core.Run, error)
	Run(ctx context.Context, RunID string) error
	ApplyAction(ctx context.Context, action core.RunAction) error
}

type Model struct {
	store    core.Store
	executor RunExecutor
	claude   core.AgentPlugin
	runtime  core.RuntimePlugin
	workDir  string

	projects []core.Project
	Runs     []core.Run

	selectedProjectID string
	projectCursor     int

	input   string
	history []string

	width   int
	height  int
	running bool

	chatCh        chan tea.Msg
	chatSessionID string
}

type snapshotMsg struct {
	projects []core.Project
	Runs     []core.Run
}

type commandResultMsg struct {
	output string
	err    error
}

type chatSessionMsg struct {
	sessionID string
}

type chatOutputMsg struct {
	content string
}

type chatDoneMsg struct {
	err error
}

type tickMsg time.Time
type errMsg error

func NewModel(executor RunExecutor, store core.Store, claude core.AgentPlugin, runtime core.RuntimePlugin) Model {
	wd, _ := os.Getwd()
	return Model{
		store:    store,
		executor: executor,
		claude:   claude,
		runtime:  runtime,
		workDir:  wd,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadSnapshotCmd(m.store), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case snapshotMsg:
		m.projects = msg.projects
		m.Runs = msg.Runs
		m.syncProjectSelection()
		return m, nil

	case errMsg:
		m.appendHistory("error: " + error(msg).Error())
		return m, nil

	case commandResultMsg:
		m.running = false
		if msg.err != nil {
			m.appendHistory("error: " + msg.err.Error())
		} else {
			m.appendHistory(msg.output)
		}
		return m, loadSnapshotCmd(m.store)

	case chatSessionMsg:
		m.chatSessionID = msg.sessionID
		return m, waitChatMsgCmd(m.chatCh)

	case chatOutputMsg:
		m.appendHistory("claude> " + msg.content)
		return m, waitChatMsgCmd(m.chatCh)

	case chatDoneMsg:
		m.running = false
		m.chatSessionID = ""
		if msg.err != nil {
			m.appendHistory("error: " + msg.err.Error())
		} else {
			m.appendHistory("Claude 对话结束。")
		}
		return m, loadSnapshotCmd(m.store)

	case tickMsg:
		return m, tea.Batch(loadSnapshotCmd(m.store), tickCmd())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.running && m.chatSessionID != "" {
				_ = m.runtime.Kill(m.chatSessionID)
				m.appendHistory("已请求停止当前 Claude 对话。")
			}
			return m, nil

		case "tab":
			m.switchProject(1)
			return m, nil

		case "shift+tab":
			m.switchProject(-1)
			return m, nil

		case "enter":
			line := strings.TrimSpace(m.input)
			m.input = ""
			if line == "" {
				return m, nil
			}

			switch line {
			case "q", "quit", "exit", ":q":
				return m, tea.Quit
			case "clear":
				m.history = nil
				return m, nil
			}

			if m.running {
				m.appendHistory("已有任务正在执行，请稍候。")
				return m, nil
			}

			if strings.HasPrefix(line, "/") {
				cmdLine := strings.TrimSpace(strings.TrimPrefix(line, "/"))
				if cmdLine == "" {
					m.appendHistory("error: 请输入命令，例如 /help")
					return m, nil
				}
				if cmdLine == "clear" {
					m.history = nil
					return m, nil
				}
				m.appendHistory("> /" + cmdLine)
				m.running = true
				return m, executeCommandCmd(m.store, m.executor, cmdLine)
			}

			prompt, proj, autoMatched, err := resolveChatInputWithSelection(line, m.projects, m.workDir, m.selectedProjectID)
			if err != nil && canAttemptAutoCreateProject(line) {
				autoProj, created, createErr := ensureProjectForWorkDir(m.store, m.projects, m.workDir)
				if createErr != nil {
					err = fmt.Errorf("自动创建项目失败: %w", createErr)
				} else {
					if !projectIDExists(m.projects, autoProj.ID) {
						m.projects = append(m.projects, autoProj)
					}
					if created {
						m.appendHistory(fmt.Sprintf("自动创建项目: %s -> %s", autoProj.ID, autoProj.RepoPath))
					}
					prompt, proj, autoMatched, err = resolveChatInputWithSelection(line, m.projects, m.workDir, m.selectedProjectID)
				}
			}
			if err != nil {
				m.appendHistory("error: " + err.Error())
				return m, nil
			}
			if autoMatched {
				m.appendHistory(fmt.Sprintf("自动匹配项目: %s", proj.ID))
			}
			m.appendHistory(fmt.Sprintf("你@%s> %s", proj.ID, prompt))

			m.running = true
			m.chatSessionID = ""
			m.chatCh = make(chan tea.Msg, 64)
			startClaudeChatWorker(m.chatCh, m.claude, m.runtime, prompt, proj.RepoPath)
			return m, waitChatMsgCmd(m.chatCh)

		case "backspace", "ctrl+h":
			runes := []rune(m.input)
			if len(runes) > 0 {
				m.input = string(runes[:len(runes)-1])
			}
			return m, nil

		case "ctrl+u":
			m.input = ""
			return m, nil
		}

		if len(msg.Runes) > 0 {
			m.input += string(msg.Runes)
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) appendHistory(text string) {
	ts := time.Now().Format("15:04:05")
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m.history = append(m.history, fmt.Sprintf("[%s] %s", ts, line))
	}
	if len(m.history) > 500 {
		m.history = m.history[len(m.history)-500:]
	}
}

func (m *Model) syncProjectSelection() {
	if len(m.projects) == 0 {
		m.selectedProjectID = ""
		m.projectCursor = 0
		return
	}

	for i, p := range m.projects {
		if p.ID == m.selectedProjectID {
			m.projectCursor = i
			return
		}
	}

	m.selectedProjectID = m.projects[0].ID
	m.projectCursor = 0
}

func (m *Model) switchProject(delta int) {
	if len(m.projects) == 0 {
		return
	}
	if delta == 0 {
		return
	}

	n := len(m.projects)
	next := (m.projectCursor + delta) % n
	if next < 0 {
		next += n
	}
	m.projectCursor = next
	m.selectedProjectID = m.projects[next].ID
}

func (m Model) selectedProjectLabel() string {
	if m.selectedProjectID == "" {
		return "(未选择)"
	}
	return m.selectedProjectID
}

func (m Model) RunsForSelectedProject() []core.Run {
	if m.selectedProjectID == "" {
		return m.Runs
	}

	out := make([]core.Run, 0, len(m.Runs))
	for _, p := range m.Runs {
		if p.ProjectID == m.selectedProjectID {
			out = append(out, p)
		}
	}
	return out
}

func summarizeSchedulerState(Runs []core.Run) (running, queued, waitingHuman int) {
	for _, p := range Runs {
		switch p.Status {
		case core.StatusRunning:
			running++
		case core.StatusCreated:
			queued++
		case core.StatusWaitingReview:
			waitingHuman++
		}
	}
	return
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("AI Workflow Orchestrator (Chat + Run)") + "\n")
	b.WriteString(StyleHelp.Render("直接输入文本会发送给 Claude。输入 /help 查看流水线命令。Tab/Shift+Tab 切换项目。") + "\n")

	runningCount, queuedCount, waitingCount := summarizeSchedulerState(m.Runs)
	b.WriteString(StyleHelp.Render(fmt.Sprintf(
		"当前项目: %s | 调度状态 running=%d queued=%d waiting_review=%d",
		m.selectedProjectLabel(), runningCount, queuedCount, waitingCount,
	)) + "\n\n")

	statusRenderer := map[string]func(string) string{}
	for k, st := range StyleStatus {
		style := st
		statusRenderer[k] = func(s string) string {
			return style.Render(s)
		}
	}

	b.WriteString("Projects\n")
	b.WriteString(views.RenderProjectList(m.projects, m.projectCursor) + "\n")

	b.WriteString("Runs\n")
	RunView := views.RenderRunList(m.RunsForSelectedProject(), -1, statusRenderer)
	b.WriteString(RunView + "\n")

	b.WriteString("Output\n")
	maxOutputLines := 12
	if m.height > 0 {
		RunLines := strings.Count(strings.TrimSuffix(RunView, "\n"), "\n") + 1
		maxOutputLines = m.height - RunLines - 10
		if maxOutputLines < 4 {
			maxOutputLines = 4
		}
	}
	outLines := m.history
	if len(outLines) > maxOutputLines {
		outLines = outLines[len(outLines)-maxOutputLines:]
	}
	if len(outLines) == 0 {
		b.WriteString("(暂无输出)\n")
	} else {
		for _, line := range outLines {
			b.WriteString(line + "\n")
		}
	}

	state := "idle"
	if m.running {
		state = "running"
	}
	b.WriteString("\n")
	b.WriteString(StyleInput.Render("> "+m.input) + "\n")
	b.WriteString(StyleHelp.Render("Enter 发送 | Esc 停止 Claude 当前输出 | /help 命令 | /Run action ... | clear 清屏 | q 退出 | 状态: " + state))
	return b.String()
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func waitChatMsgCmd(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func loadSnapshotCmd(store core.Store) tea.Cmd {
	return func() tea.Msg {
		projects, err := store.ListProjects(core.ProjectFilter{})
		if err != nil {
			return errMsg(err)
		}

		var Runs []core.Run
		for _, proj := range projects {
			items, err := store.ListRuns(proj.ID, core.RunFilter{})
			if err != nil {
				return errMsg(err)
			}
			Runs = append(Runs, items...)
		}
		sort.Slice(Runs, func(i, j int) bool {
			return Runs[i].CreatedAt.After(Runs[j].CreatedAt)
		})
		return snapshotMsg{projects: projects, Runs: Runs}
	}
}

func executeCommandCmd(store core.Store, executor RunExecutor, line string) tea.Cmd {
	return func() tea.Msg {
		out, err := runCommand(context.Background(), store, executor, line)
		return commandResultMsg{output: out, err: err}
	}
}

func startClaudeChatWorker(ch chan<- tea.Msg, claude core.AgentPlugin, runtime core.RuntimePlugin, prompt string, workDir string) {
	go func() {
		cmd, err := claude.BuildCommand(core.ExecOpts{
			Prompt:   prompt,
			WorkDir:  workDir,
			MaxTurns: 30,
		})
		if err != nil {
			ch <- chatDoneMsg{err: fmt.Errorf("build claude command: %w", err)}
			return
		}

		sess, err := runtime.Create(context.Background(), core.RuntimeOpts{
			Command: cmd,
			WorkDir: workDir,
		})
		if err != nil {
			ch <- chatDoneMsg{err: fmt.Errorf("start claude session: %w", err)}
			return
		}
		ch <- chatSessionMsg{sessionID: sess.ID}

		parser := claude.NewStreamParser(sess.Stdout)
		for {
			evt, err := parser.Next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				ch <- chatDoneMsg{err: fmt.Errorf("parse claude stream: %w", err)}
				return
			}

			text := formatChatEvent(evt)
			if strings.TrimSpace(text) == "" {
				continue
			}
			ch <- chatOutputMsg{content: text}
		}

		if err := sess.Wait(); err != nil {
			ch <- chatDoneMsg{err: fmt.Errorf("claude session failed: %w", err)}
			return
		}
		ch <- chatDoneMsg{}
	}()
}

func formatChatEvent(evt *core.StreamEvent) string {
	switch evt.Type {
	case "tool_call":
		if evt.ToolInput == "" {
			return fmt.Sprintf("[tool] %s", evt.ToolName)
		}
		return fmt.Sprintf("[tool] %s %s", evt.ToolName, evt.ToolInput)
	default:
		return strings.TrimSpace(evt.Content)
	}
}

func resolveChatInput(line string, projects []core.Project, workDir string) (string, core.Project, error) {
	msg := strings.TrimSpace(line)
	if msg == "" {
		return "", core.Project{}, fmt.Errorf("输入内容为空")
	}
	if len(projects) == 0 {
		return "", core.Project{}, fmt.Errorf("没有可用项目，请先用 /project add <id> <repo-path> 添加")
	}

	if strings.HasPrefix(msg, "@") {
		rest := strings.TrimSpace(strings.TrimPrefix(msg, "@"))
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) < 2 {
			return "", core.Project{}, fmt.Errorf("请使用 @<project-id> <内容> 格式")
		}

		proj, ok := findProjectByID(parts[0], projects)
		if !ok {
			inferred, inferredOK := inferProjectByDir(workDir, projects)
			if !inferredOK {
				return "", core.Project{}, fmt.Errorf("未找到项目 %q，且无法根据当前目录自动匹配项目（当前目录: %s）", parts[0], workDir)
			}
			proj = inferred
		}
		content := strings.TrimSpace(parts[1])
		if content == "" {
			return "", core.Project{}, fmt.Errorf("消息内容为空")
		}
		return content, proj, nil
	}

	if len(projects) == 1 {
		return msg, projects[0], nil
	}

	inferred, ok := inferProjectByDir(workDir, projects)
	if ok {
		return msg, inferred, nil
	}

	return "", core.Project{}, fmt.Errorf("检测到多个项目，且无法根据当前目录自动匹配项目（当前目录: %s）", workDir)
}

func resolveChatInputWithSelection(
	line string,
	projects []core.Project,
	workDir string,
	selectedProjectID string,
) (string, core.Project, bool, error) {
	msg := strings.TrimSpace(line)
	if msg == "" {
		return "", core.Project{}, false, fmt.Errorf("输入内容为空")
	}

	if strings.HasPrefix(msg, "@") {
		prompt, proj, err := resolveChatInput(line, projects, workDir)
		return prompt, proj, false, err
	}

	if selectedProjectID != "" {
		if proj, ok := findProjectByID(selectedProjectID, projects); ok {
			return msg, proj, true, nil
		}
	}

	prompt, proj, err := resolveChatInput(line, projects, workDir)
	if err != nil {
		return "", core.Project{}, false, err
	}
	// No explicit @prefix and matched by fallback rules, treat as auto-matched.
	return prompt, proj, true, nil
}

func findProjectByID(id string, projects []core.Project) (core.Project, bool) {
	for _, p := range projects {
		if p.ID == id {
			return p, true
		}
	}
	return core.Project{}, false
}

func inferProjectByDir(workDir string, projects []core.Project) (core.Project, bool) {
	if strings.TrimSpace(workDir) == "" {
		return core.Project{}, false
	}

	wd := normalizePath(workDir)
	best := core.Project{}
	bestLen := -1

	for _, p := range projects {
		repo := normalizePath(p.RepoPath)
		if repo == "" {
			continue
		}
		if wd == repo || strings.HasPrefix(wd, repo+string(filepath.Separator)) {
			if len(repo) > bestLen {
				best = p
				bestLen = len(repo)
			}
		}
	}

	if bestLen < 0 {
		return core.Project{}, false
	}
	return best, true
}

func normalizePath(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	clean := filepath.Clean(p)
	return strings.ToLower(clean)
}

func projectIDExists(projects []core.Project, id string) bool {
	for _, p := range projects {
		if p.ID == id {
			return true
		}
	}
	return false
}

func canAttemptAutoCreateProject(line string) bool {
	msg := strings.TrimSpace(line)
	if msg == "" {
		return false
	}
	if !strings.HasPrefix(msg, "@") {
		return true
	}
	rest := strings.TrimSpace(strings.TrimPrefix(msg, "@"))
	parts := strings.SplitN(rest, " ", 2)
	return len(parts) == 2 && strings.TrimSpace(parts[1]) != ""
}

func ensureProjectForWorkDir(store core.Store, projects []core.Project, workDir string) (core.Project, bool, error) {
	wd := strings.TrimSpace(workDir)
	if wd == "" {
		return core.Project{}, false, fmt.Errorf("当前目录为空，无法自动创建项目")
	}

	normalizedWD := normalizePath(wd)
	for _, p := range projects {
		if normalizePath(p.RepoPath) == normalizedWD {
			return p, false, nil
		}
	}

	baseID := projectIDFromPath(wd)
	candidate := baseID
	for i := 2; ; i++ {
		if !projectIDExists(projects, candidate) {
			break
		}
		candidate = fmt.Sprintf("%s-%d", baseID, i)
	}

	newProj := core.Project{
		ID:       candidate,
		Name:     candidate,
		RepoPath: wd,
	}
	if err := store.CreateProject(&newProj); err != nil {
		return core.Project{}, false, err
	}
	return newProj, true, nil
}

func projectIDFromPath(path string) string {
	base := strings.ToLower(filepath.Base(filepath.Clean(path)))
	if strings.TrimSpace(base) == "" || base == "." || base == string(filepath.Separator) {
		return fmt.Sprintf("project-%d", time.Now().Unix())
	}

	var b strings.Builder
	prevDash := false
	for _, r := range base {
		isAllowed := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
		if isAllowed {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fmt.Sprintf("project-%d", time.Now().Unix())
	}
	return out
}

func runCommand(ctx context.Context, store core.Store, executor RunExecutor, line string) (string, error) {
	args, err := splitArgs(line)
	if err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", nil
	}

	switch args[0] {
	case "help":
		return helpText(), nil
	case "refresh":
		return "已刷新。", nil
	case "project":
		return runProjectCommand(store, args[1:])
	case "Run":
		return runRunCommand(ctx, store, executor, args[1:])
	default:
		return "", fmt.Errorf("unknown command: %s", args[0])
	}
}

func runProjectCommand(store core.Store, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: project <add|list>")
	}

	switch args[0] {
	case "add":
		if len(args) < 3 {
			return "", fmt.Errorf("usage: project add <id> <repo-path>")
		}
		p := &core.Project{
			ID:       args[1],
			Name:     args[1],
			RepoPath: args[2],
		}
		if _, err := os.Stat(p.RepoPath); err != nil {
			return "", fmt.Errorf("repo path invalid: %w", err)
		}
		if err := store.CreateProject(p); err != nil {
			return "", err
		}
		return fmt.Sprintf("Project added: %s -> %s", p.ID, p.RepoPath), nil

	case "list", "ls":
		projects, err := store.ListProjects(core.ProjectFilter{})
		if err != nil {
			return "", err
		}
		if len(projects) == 0 {
			return "No projects.", nil
		}

		var b strings.Builder
		b.WriteString("Projects:")
		for _, p := range projects {
			b.WriteString(fmt.Sprintf("\n- %s | %s", p.ID, p.RepoPath))
		}
		return b.String(), nil
	default:
		return "", fmt.Errorf("unknown project command: %s", args[0])
	}
}

func runRunCommand(ctx context.Context, store core.Store, executor RunExecutor, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: Run <create|start|status|list|action>")
	}

	switch args[0] {
	case "create":
		if len(args) < 4 {
			return "", fmt.Errorf("usage: Run create <project-id> <name> <description> [template]")
		}
		template := "standard"
		if len(args) > 4 {
			template = args[4]
		}
		p, err := executor.CreateRun(args[1], args[2], args[3], template)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Run created: %s (template: %s, stages: %d)", p.ID, p.Template, len(p.Stages)), nil

	case "start":
		if len(args) < 2 {
			return "", fmt.Errorf("usage: Run start <Run-id>")
		}
		if err := executor.Run(ctx, args[1]); err != nil {
			return "", err
		}
		p, err := store.GetRun(args[1])
		if err != nil {
			return "Run run finished.", nil
		}
		return "Run run finished.\n" + formatRunStatus(p), nil

	case "status":
		if len(args) < 2 {
			return "", fmt.Errorf("usage: Run status <Run-id>")
		}
		p, err := store.GetRun(args[1])
		if err != nil {
			return "", err
		}
		return formatRunStatus(p), nil

	case "list", "ls":
		var items []core.Run
		if len(args) >= 2 {
			found, err := store.ListRuns(args[1], core.RunFilter{})
			if err != nil {
				return "", err
			}
			items = found
		} else {
			found, err := listAllRuns(store)
			if err != nil {
				return "", err
			}
			items = found
		}

		if len(items) == 0 {
			return "No Runs.", nil
		}

		var b strings.Builder
		b.WriteString("Runs:")
		for _, p := range items {
			stage := string(p.CurrentStage)
			if stage == "" {
				stage = "-"
			}
			b.WriteString(fmt.Sprintf("\n- %s | %s | %s | %s", p.ID, p.Name, stage, p.Status))
		}
		return b.String(), nil

	case "action":
		if len(args) < 3 {
			return "", fmt.Errorf("usage: Run action <Run-id> <approve|reject|modify|skip|rerun|change_role|abort|pause|resume> [--stage <stage>] [--role <role>] [--message <text>]")
		}
		actionType, err := parseHumanActionType(args[2])
		if err != nil {
			return "", err
		}

		action := core.RunAction{
			RunID: args[1],
			Type:  actionType,
		}
		for i := 3; i < len(args); i++ {
			switch args[i] {
			case "--stage":
				i++
				if i >= len(args) {
					return "", fmt.Errorf("--stage requires a value")
				}
				action.Stage = core.StageID(args[i])
			case "--role":
				i++
				if i >= len(args) {
					return "", fmt.Errorf("--role requires a value")
				}
				action.Role = args[i]
			case "--message":
				i++
				if i >= len(args) {
					return "", fmt.Errorf("--message requires a value")
				}
				action.Message = strings.Join(args[i:], " ")
				i = len(args)
			default:
				action.Message = strings.Join(args[i:], " ")
				i = len(args)
			}
		}

		if err := executor.ApplyAction(ctx, action); err != nil {
			return "", err
		}
		return fmt.Sprintf("Action applied: %s %s", action.RunID, action.Type), nil

	default:
		return "", fmt.Errorf("unknown Run command: %s", args[0])
	}
}

func parseHumanActionType(raw string) (core.HumanActionType, error) {
	action := core.HumanActionType(strings.ToLower(strings.TrimSpace(raw)))
	switch action {
	case core.ActionApprove,
		core.ActionReject,
		core.ActionModify,
		core.ActionSkip,
		core.ActionRerun,
		core.ActionChangeRole,
		core.ActionAbort,
		core.ActionPause,
		core.ActionResume:
		return action, nil
	default:
		return "", fmt.Errorf("unknown action type: %s", raw)
	}
}

func listAllRuns(store core.Store) ([]core.Run, error) {
	projects, err := store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return nil, err
	}

	var out []core.Run
	for _, p := range projects {
		items, err := store.ListRuns(p.ID, core.RunFilter{})
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func splitArgs(line string) ([]string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}

	var args []string
	var b strings.Builder
	var quote rune

	flush := func() {
		if b.Len() > 0 {
			args = append(args, b.String())
			b.Reset()
		}
	}

	for _, r := range line {
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			continue
		}

		if r == '"' || r == '\'' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		b.WriteRune(r)
	}

	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote in input")
	}
	flush()
	return args, nil
}

func formatRunStatus(p *core.Run) string {
	return fmt.Sprintf(
		"Run: %s\nStatus:   %s\nStage:    %s\nTemplate: %s",
		p.ID, p.Status, p.CurrentStage, p.Template,
	)
}

func helpText() string {
	return strings.Join([]string{
		"聊天模式:",
		"- 直接输入文本并回车：发送给 Claude",
		"- 多项目时会优先按当前启动目录自动匹配项目",
		"- 若当前目录不属于已注册项目，会自动创建项目并继续对话",
		"- 也可显式写 @<project-id> 前缀，例如 @demo 讨论需求",
		"",
		"命令模式（以 / 开头）:",
		"- /help",
		"- /refresh",
		"- /clear",
		"- /project add <id> <repo-path>",
		"- /project list",
		"- /Run create <project-id> <name> <description> [template]",
		"- /Run start <Run-id>",
		"- /Run status <Run-id>",
		"- /Run list [project-id]",
		"- /Run action <Run-id> <approve|reject|modify|skip|rerun|change_role|abort|pause|resume> [--stage <stage>] [--role <role>] [--message <text>]",
		`Tip: 含空格参数请加引号，例如: /Run create demo p1 "实现登录与注册" quick`,
	}, "\n")
}

func Run(executor RunExecutor, store core.Store, claude core.AgentPlugin, runtime core.RuntimePlugin) error {
	p := tea.NewProgram(NewModel(executor, store, claude, runtime))
	_, err := p.Run()
	return err
}
