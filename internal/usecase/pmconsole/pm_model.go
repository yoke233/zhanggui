package pmconsole

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"zhanggui/internal/bootstrap/logging"
	"zhanggui/internal/usecase/outbox"
)

const maxShownEvents = 4
const maxAuditLines = 8

type PMOptions struct {
	Role            string
	Assignee        string
	StateFilter     string
	WorkflowFile    string
	ConfigFile      string
	ExecutablePath  string
	RefreshInterval time.Duration
}

type pmModel struct {
	ctx             context.Context
	service         *outbox.Service
	scopeRole       string
	assigneeFilter  string
	stateFilter     string
	workflowFile    string
	configFile      string
	executablePath  string
	refreshInterval time.Duration

	enabledRoles   []string
	enabledRoleSet map[string]struct{}

	issues         []outbox.IssueListItem
	selectedIndex  int
	detail         outbox.IssueDetail
	hasDetail      bool
	activeRunID    string
	activeRunFound bool
	artifacts      runArtifacts
	hasArtifacts   bool
	status         string
	auditLogs      []string
}

type issuesLoadedMsg struct {
	items []outbox.IssueListItem
	err   error
}

type workflowSummaryLoadedMsg struct {
	enabledRoles []string
	err          error
}

type issueDetailLoadedMsg struct {
	issueRef       string
	detail         outbox.IssueDetail
	hasDetail      bool
	activeRunID    string
	activeRunFound bool
	artifacts      runArtifacts
	hasArtifacts   bool
	err            error
}

type tickMsg struct{}

type actionDoneMsg struct {
	action   string
	issueRef string
	role     string
	actor    string
	result   string
	err      error
}

type issueRoute struct {
	Role     string
	Assignee string
	Source   string
	Err      error
}

type runArtifacts struct {
	ContextPackDir    string
	ContextPackExists bool

	Workdir       string
	WorkdirExists bool

	WorkResultExists bool
	TestsResult      string
	ResultCode       string
	Summary          string

	StdoutPath     string
	StderrPath     string
	WorkResultJSON string
	WorkResultText string
	WorkAuditJSON  string
}

func NewPMModel(ctx context.Context, service *outbox.Service, options PMOptions) tea.Model {
	scopeRole := normalizeScopeRole(options.Role)

	assignee := strings.TrimSpace(options.Assignee)
	if scopeRole != "all" && assignee == "" {
		assignee = "lead-" + scopeRole
	}

	stateFilter := normalizeStateFilter(options.StateFilter)
	workflowFile := strings.TrimSpace(options.WorkflowFile)
	if workflowFile == "" {
		workflowFile = "workflow.toml"
	}
	configFile := strings.TrimSpace(options.ConfigFile)
	executablePath := strings.TrimSpace(options.ExecutablePath)
	interval := options.RefreshInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}

	return &pmModel{
		ctx:             ctx,
		service:         service,
		scopeRole:       scopeRole,
		assigneeFilter:  assignee,
		stateFilter:     stateFilter,
		workflowFile:    workflowFile,
		configFile:      configFile,
		executablePath:  executablePath,
		refreshInterval: interval,
		status:          "初始化中",
	}
}

func (m *pmModel) Init() tea.Cmd {
	return tea.Batch(m.loadWorkflowSummaryCmd(), m.loadIssuesCmd(), m.tickCmd())
}

func (m *pmModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tickMsg:
		return m, tea.Batch(m.loadIssuesCmd(), m.tickCmd())
	case workflowSummaryLoadedMsg:
		if msg.err != nil {
			m.enabledRoles = nil
			m.enabledRoleSet = nil
			m.status = "workflow 加载失败: " + msg.err.Error()
			return m, nil
		}
		m.enabledRoles = msg.enabledRoles
		m.enabledRoleSet = toStringSet(msg.enabledRoles)
		m.status = fmt.Sprintf("workflow 已加载 roles=%d", len(m.enabledRoles))
		if len(m.issues) > 0 {
			return m, m.loadSelectedIssueDetailCmd()
		}
		return m, nil
	case issuesLoadedMsg:
		if msg.err != nil {
			m.status = "刷新失败: " + msg.err.Error()
			return m, nil
		}
		m.issues = msg.items
		if len(m.issues) == 0 {
			m.selectedIndex = 0
			m.hasDetail = false
			m.activeRunID = ""
			m.activeRunFound = false
			m.hasArtifacts = false
			m.artifacts = runArtifacts{}
			m.status = "队列为空"
			return m, nil
		}
		if m.selectedIndex < 0 {
			m.selectedIndex = 0
		}
		if m.selectedIndex >= len(m.issues) {
			m.selectedIndex = len(m.issues) - 1
		}
		m.status = fmt.Sprintf("已刷新，共 %d 条", len(m.issues))
		return m, m.loadSelectedIssueDetailCmd()
	case issueDetailLoadedMsg:
		if !m.isCurrentSelectedIssue(msg.issueRef) {
			return m, nil
		}
		if msg.err != nil {
			m.hasDetail = false
			m.activeRunID = ""
			m.activeRunFound = false
			m.hasArtifacts = false
			m.artifacts = runArtifacts{}
			m.status = "详情加载失败: " + msg.err.Error()
			return m, nil
		}
		m.hasDetail = msg.hasDetail
		m.detail = msg.detail
		m.activeRunID = msg.activeRunID
		m.activeRunFound = msg.activeRunFound
		m.artifacts = msg.artifacts
		m.hasArtifacts = msg.hasArtifacts
		return m, nil
	case actionDoneMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("%s 失败: %v", msg.action, msg.err)
			m.appendAuditLog(msg.action, msg.issueRef, msg.role, msg.actor, "failed", msg.err)
		} else {
			m.status = fmt.Sprintf("%s 完成: %s", msg.action, msg.result)
			m.appendAuditLog(msg.action, msg.issueRef, msg.role, msg.actor, msg.result, nil)
		}
		return m, m.loadIssuesCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "g":
			m.status = "手动刷新中"
			return m, m.loadIssuesCmd()
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				return m, m.loadSelectedIssueDetailCmd()
			}
			return m, nil
		case "down", "j":
			if m.selectedIndex < len(m.issues)-1 {
				m.selectedIndex++
				return m, m.loadSelectedIssueDetailCmd()
			}
			return m, nil
		case "c":
			return m, m.claimOrUnclaimCmd()
		case "s":
			return m, m.spawnCmd(false)
		case "w":
			return m, m.spawnCmd(true)
		case "r":
			return m, m.replyCmd()
		case "b":
			return m, m.toggleBlockedCmd()
		case "x":
			return m, m.closeCmd()
		case "D":
			return m, m.cleanupWorkdirCmd()
		}
	}
	return m, nil
}

func (m *pmModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62"))

	enabled := "-"
	if len(m.enabledRoles) > 0 {
		enabled = strings.Join(m.enabledRoles, ",")
	}

	var builder strings.Builder
	builder.WriteString(titleStyle.Render("PM Console 2.1（全局视图）"))
	builder.WriteString("\n")
	builder.WriteString(dimStyle.Render(fmt.Sprintf(
		"scope=%s assignee=%s state=%s workflow=%s roles=%s refresh=%s",
		m.scopeRole,
		firstNonEmpty(m.assigneeFilter, "-"),
		firstNonEmpty(m.stateFilter, "all"),
		m.workflowFile,
		enabled,
		m.refreshInterval,
	)))
	builder.WriteString("\n\n")

	builder.WriteString(sectionStyle.Render("Queue"))
	builder.WriteString("\n")
	if len(m.issues) == 0 {
		builder.WriteString(dimStyle.Render("- no issues"))
		builder.WriteString("\n\n")
	} else {
		for index, item := range m.issues {
			state := stateFromLabels(item.Labels)
			if state == "" {
				state = "open"
			}
			issueAssignee := firstNonEmpty(strings.TrimSpace(item.Assignee), "-")

			route := m.resolveRoute(item.IssueRef, item.Assignee, item.Labels)
			routeRole := firstNonEmpty(route.Role, "unknown")
			routeSource := firstNonEmpty(route.Source, "-")
			line := fmt.Sprintf(
				"%s [%s] role=%s src=%s assignee=%s title=%s",
				item.IssueRef,
				strings.TrimPrefix(state, "state:"),
				routeRole,
				routeSource,
				issueAssignee,
				item.Title,
			)
			if index == m.selectedIndex {
				builder.WriteString(selectedStyle.Render("> " + line))
			} else {
				builder.WriteString("  " + line)
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString(sectionStyle.Render("Detail"))
	builder.WriteString("\n")
	if !m.hasDetail {
		builder.WriteString(dimStyle.Render("- no detail"))
		builder.WriteString("\n\n")
	} else {
		route := m.resolveRoute(m.detail.IssueRef, m.detail.Assignee, m.detail.Labels)
		routeRole := firstNonEmpty(route.Role, "unknown")
		routeSource := firstNonEmpty(route.Source, "-")
		builder.WriteString(fmt.Sprintf("IssueRef: %s\n", m.detail.IssueRef))
		builder.WriteString(fmt.Sprintf("Route: role=%s src=%s\n", routeRole, routeSource))
		builder.WriteString(fmt.Sprintf("Status: %s\n", mapClosedState(m.detail.IsClosed, stateFromLabels(m.detail.Labels))))
		builder.WriteString(fmt.Sprintf("Assignee: %s\n", firstNonEmpty(strings.TrimSpace(m.detail.Assignee), "-")))
		if m.activeRunFound {
			builder.WriteString(fmt.Sprintf("ActiveRun: %s\n", m.activeRunID))
		} else {
			builder.WriteString("ActiveRun: none\n")
		}
		builder.WriteString("Artifacts:\n")
		if !m.activeRunFound {
			builder.WriteString("- none\n")
		} else if !m.hasArtifacts {
			builder.WriteString("- loading...\n")
		} else {
			contextPackStatus := "missing"
			if m.artifacts.ContextPackExists {
				contextPackStatus = "ok"
			}
			builder.WriteString(fmt.Sprintf("- ContextPack(%s): %s\n", contextPackStatus, firstNonEmpty(m.artifacts.ContextPackDir, "-")))

			workdirStatus := "missing"
			if m.artifacts.WorkdirExists {
				workdirStatus = "ok"
			}
			builder.WriteString(fmt.Sprintf("- Workdir(%s): %s\n", workdirStatus, firstNonEmpty(m.artifacts.Workdir, "-")))

			if m.artifacts.WorkResultExists {
				builder.WriteString(fmt.Sprintf("- WorkResult: tests=%s result_code=%s\n", firstNonEmpty(m.artifacts.TestsResult, "-"), firstNonEmpty(m.artifacts.ResultCode, "none")))
			} else {
				builder.WriteString("- WorkResult: missing\n")
			}

			builder.WriteString(fmt.Sprintf("- stdout.log: %s\n", firstNonEmpty(m.artifacts.StdoutPath, "-")))
			builder.WriteString(fmt.Sprintf("- stderr.log: %s\n", firstNonEmpty(m.artifacts.StderrPath, "-")))
			builder.WriteString(fmt.Sprintf("- work_audit.json: %s\n", firstNonEmpty(m.artifacts.WorkAuditJSON, "-")))
		}
		builder.WriteString(fmt.Sprintf("Labels: %s\n", strings.Join(m.detail.Labels, ",")))
		builder.WriteString("\nRecent Events:\n")
		events := m.detail.Events
		if len(events) == 0 {
			builder.WriteString("- none\n")
		} else {
			start := len(events) - maxShownEvents
			if start < 0 {
				start = 0
			}
			for _, event := range events[start:] {
				builder.WriteString(fmt.Sprintf("- e%d %s %s\n", event.EventID, event.Actor, firstNonEmptyLine(event.Body)))
			}
		}
		builder.WriteString("\n")
	}

	builder.WriteString(sectionStyle.Render("Status"))
	builder.WriteString("\n")
	builder.WriteString("- " + firstNonEmpty(m.status, "ready"))
	builder.WriteString("\n\n")

	builder.WriteString(sectionStyle.Render("Actions"))
	builder.WriteString("\n")
	builder.WriteString("- c claim/unclaim\n")
	builder.WriteString("- s spawn worker\n")
	builder.WriteString("- w switch worker\n")
	builder.WriteString("- r normalize+reply\n")
	builder.WriteString("- b blocked/unblock\n")
	builder.WriteString("- D cleanup workdir\n")
	builder.WriteString("- x close issue\n")
	builder.WriteString("\n")

	builder.WriteString(sectionStyle.Render("Audit Log"))
	builder.WriteString("\n")
	if len(m.auditLogs) == 0 {
		builder.WriteString(dimStyle.Render("- no actions"))
		builder.WriteString("\n\n")
	} else {
		for _, line := range m.auditLogs {
			builder.WriteString("- " + line)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString(dimStyle.Render("Keys: ↑/k ↓/j 移动  g 刷新  c/s/w/r/b/D/x 动作  q 退出"))
	return builder.String()
}

func (m *pmModel) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *pmModel) loadWorkflowSummaryCmd() tea.Cmd {
	return func() tea.Msg {
		summary, err := m.service.GetWorkflowSummary(m.ctx, m.workflowFile)
		if err != nil {
			return workflowSummaryLoadedMsg{err: err}
		}
		return workflowSummaryLoadedMsg{enabledRoles: summary.EnabledRoles}
	}
}

func (m *pmModel) loadIssuesCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.service.ListIssues(m.ctx, false, "")
		if err != nil {
			return issuesLoadedMsg{err: err}
		}
		filtered := filterIssues(items, m.scopeRole, m.assigneeFilter, m.stateFilter)
		return issuesLoadedMsg{items: filtered}
	}
}

func (m *pmModel) loadSelectedIssueDetailCmd() tea.Cmd {
	selected, ok := m.selectedIssue()
	if !ok {
		return nil
	}

	return func() tea.Msg {
		detail, err := m.service.GetIssue(m.ctx, selected.IssueRef)
		if err != nil {
			return issueDetailLoadedMsg{
				issueRef: selected.IssueRef,
				err:      err,
			}
		}

		activeRunID := ""
		found := false
		artifacts := runArtifacts{}
		hasArtifacts := false
		route := m.resolveRoute(detail.IssueRef, detail.Assignee, detail.Labels)
		if route.Role != "" {
			activeRunID, found, err = m.service.GetActiveRunID(m.ctx, route.Role, selected.IssueRef)
			if err != nil {
				return issueDetailLoadedMsg{
					issueRef: selected.IssueRef,
					err:      err,
				}
			}
			if found && strings.TrimSpace(activeRunID) != "" {
				loaded, ok, loadErr := loadRunArtifacts(selected.IssueRef, activeRunID)
				if loadErr != nil {
					return issueDetailLoadedMsg{issueRef: selected.IssueRef, err: loadErr}
				}
				artifacts = loaded
				hasArtifacts = ok
			}
		}

		return issueDetailLoadedMsg{
			issueRef:       selected.IssueRef,
			detail:         detail,
			hasDetail:      true,
			activeRunID:    activeRunID,
			activeRunFound: found,
			artifacts:      artifacts,
			hasArtifacts:   hasArtifacts,
		}
	}
}

func loadRunArtifacts(issueRef string, runID string) (runArtifacts, bool, error) {
	dir := contextPackDir(issueRef, runID)
	artifacts := runArtifacts{
		ContextPackDir: dir,
		StdoutPath:     filepath.Join(dir, "stdout.log"),
		StderrPath:     filepath.Join(dir, "stderr.log"),
		WorkResultJSON: filepath.Join(dir, "work_result.json"),
		WorkResultText: filepath.Join(dir, "work_result.txt"),
		WorkAuditJSON:  filepath.Join(dir, "work_audit.json"),
	}

	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return artifacts, true, nil
		}
		return runArtifacts{}, false, err
	}
	artifacts.ContextPackExists = true

	workOrderPath := filepath.Join(dir, "work_order.json")
	if raw, err := os.ReadFile(workOrderPath); err == nil {
		var order struct {
			RepoDir string `json:"RepoDir"`
		}
		if err := json.Unmarshal(raw, &order); err != nil {
			return runArtifacts{}, false, err
		}
		artifacts.Workdir = strings.TrimSpace(order.RepoDir)
		if artifacts.Workdir != "" {
			if _, err := os.Stat(artifacts.Workdir); err == nil {
				artifacts.WorkdirExists = true
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return runArtifacts{}, false, err
			}
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return runArtifacts{}, false, err
	}

	if raw, err := os.ReadFile(artifacts.WorkResultJSON); err == nil {
		var result outbox.WorkResultEnvelope
		if err := json.Unmarshal(raw, &result); err != nil {
			return runArtifacts{}, false, err
		}
		artifacts.WorkResultExists = true
		artifacts.TestsResult = strings.TrimSpace(result.Tests.Result)
		artifacts.ResultCode = strings.TrimSpace(result.ResultCode)
		artifacts.Summary = strings.TrimSpace(result.Summary)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return runArtifacts{}, false, err
	}

	return artifacts, true, nil
}

func contextPackDir(issueRef string, runID string) string {
	sanitized := strings.NewReplacer("/", "_", "#", "_", ":", "_", "\\", "_").Replace(strings.TrimSpace(issueRef))
	return filepath.Join("state", "context_packs", sanitized, strings.TrimSpace(runID))
}

func (m *pmModel) cleanupWorkdirCmd() tea.Cmd {
	selected, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	m.status = "清理 workdir 中..."

	return func() tea.Msg {
		latest, err := m.service.GetIssue(m.ctx, selected.IssueRef)
		if err != nil {
			return actionDoneMsg{action: "cleanup-workdir", issueRef: selected.IssueRef, err: err}
		}

		route := m.resolveRoute(latest.IssueRef, latest.Assignee, latest.Labels)
		if route.Err != nil || route.Role == "" {
			return actionDoneMsg{action: "cleanup-workdir", issueRef: selected.IssueRef, err: route.Err}
		}

		runID, found, err := m.service.GetActiveRunID(m.ctx, route.Role, selected.IssueRef)
		if err != nil {
			return actionDoneMsg{action: "cleanup-workdir", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}
		if !found || strings.TrimSpace(runID) == "" {
			return actionDoneMsg{action: "cleanup-workdir", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: errors.New("ActiveRun not found")}
		}

		cleanup, err := m.service.CleanupWorkdir(m.ctx, outbox.CleanupWorkdirInput{
			WorkflowFile: m.workflowFile,
			Role:         route.Role,
			IssueRef:     selected.IssueRef,
			RunID:        runID,
		})
		if err != nil {
			return actionDoneMsg{action: "cleanup-workdir", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}
		return actionDoneMsg{action: "cleanup-workdir", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, result: cleanup.Workdir}
	}
}

func (m *pmModel) claimOrUnclaimCmd() tea.Cmd {
	selected, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	m.status = "执行 claim/unclaim 中..."
	return func() tea.Msg {
		latest, err := m.service.GetIssue(m.ctx, selected.IssueRef)
		if err != nil {
			return actionDoneMsg{action: "claim/unclaim", issueRef: selected.IssueRef, err: err}
		}
		route := m.resolveRoute(latest.IssueRef, latest.Assignee, latest.Labels)
		if route.Err != nil || route.Role == "" {
			return actionDoneMsg{action: "claim/unclaim", issueRef: selected.IssueRef, err: route.Err}
		}
		if latest.IsClosed {
			return actionDoneMsg{action: "claim/unclaim", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: errors.New("issue is closed")}
		}

		if strings.TrimSpace(latest.Assignee) == "" {
			if err := m.service.ClaimIssue(m.ctx, outbox.ClaimIssueInput{
				IssueRef: latest.IssueRef,
				Assignee: route.Assignee,
				Actor:    route.Assignee,
				Comment:  "console claim",
			}); err != nil {
				return actionDoneMsg{action: "claim", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
			}
			return actionDoneMsg{action: "claim", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, result: route.Assignee}
		}

		if strings.TrimSpace(latest.Assignee) != strings.TrimSpace(route.Assignee) {
			return actionDoneMsg{
				action:   "unclaim",
				issueRef: selected.IssueRef,
				role:     route.Role,
				actor:    route.Assignee,
				err:      fmt.Errorf("issue already claimed by %s", latest.Assignee),
			}
		}
		if err := m.service.UnclaimIssue(m.ctx, outbox.UnclaimIssueInput{
			IssueRef: latest.IssueRef,
			Actor:    route.Assignee,
			Comment:  "console unclaim",
		}); err != nil {
			return actionDoneMsg{action: "unclaim", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}
		return actionDoneMsg{action: "unclaim", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, result: "state:todo"}
	}
}

func (m *pmModel) spawnCmd(forceSpawn bool) tea.Cmd {
	selected, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	action := "spawn"
	if forceSpawn {
		action = "switch"
	}
	m.status = "执行 " + action + " 中..."
	return func() tea.Msg {
		latest, err := m.service.GetIssue(m.ctx, selected.IssueRef)
		if err != nil {
			return actionDoneMsg{action: action, issueRef: selected.IssueRef, err: err}
		}
		route := m.resolveRoute(latest.IssueRef, latest.Assignee, latest.Labels)
		if err := m.checkActionAllowed(action, latest, route); err != nil {
			return actionDoneMsg{action: action, issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}

		result, err := m.service.LeadRunIssueOnce(m.ctx, outbox.LeadRunIssueInput{
			Role:           route.Role,
			Assignee:       route.Assignee,
			IssueRef:       latest.IssueRef,
			WorkflowFile:   m.workflowFile,
			ConfigFile:     m.configFile,
			ExecutablePath: m.executablePath,
			ForceSpawn:     forceSpawn,
		})
		if err != nil {
			return actionDoneMsg{action: action, issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}

		status := "skipped"
		if result.Processed {
			status = "processed"
		}
		if result.Blocked {
			status = "blocked"
		}
		if result.Spawned && result.Blocked {
			status = "spawned+blocked"
		} else if result.Spawned {
			status = "spawned"
		}

		return actionDoneMsg{
			action:   action,
			issueRef: selected.IssueRef,
			role:     route.Role,
			actor:    route.Assignee,
			result:   status,
		}
	}
}

func (m *pmModel) replyCmd() tea.Cmd {
	selected, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	m.status = "执行 normalize+reply 中..."
	return func() tea.Msg {
		latest, err := m.service.GetIssue(m.ctx, selected.IssueRef)
		if err != nil {
			return actionDoneMsg{action: "reply", issueRef: selected.IssueRef, err: err}
		}
		route := m.resolveRoute(latest.IssueRef, latest.Assignee, latest.Labels)
		if err := m.checkActionAllowed("reply", latest, route); err != nil {
			return actionDoneMsg{action: "reply", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}

		state := trimStatePrefix(stateFromLabels(latest.Labels))
		if state == "" {
			state = "doing"
		}

		if err := m.service.CommentIssue(m.ctx, outbox.CommentIssueInput{
			IssueRef: latest.IssueRef,
			Actor:    route.Assignee,
			Body:     "console normalized reply",
			State:    state,
		}); err != nil {
			return actionDoneMsg{action: "reply", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}
		return actionDoneMsg{action: "reply", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, result: "state:" + state}
	}
}

func (m *pmModel) toggleBlockedCmd() tea.Cmd {
	selected, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	m.status = "执行 blocked/unblock 中..."
	return func() tea.Msg {
		latest, err := m.service.GetIssue(m.ctx, selected.IssueRef)
		if err != nil {
			return actionDoneMsg{action: "block/unblock", issueRef: selected.IssueRef, err: err}
		}
		route := m.resolveRoute(latest.IssueRef, latest.Assignee, latest.Labels)
		currentState := stateFromLabels(latest.Labels)

		nextState := "blocked"
		action := "block"
		if currentState == "state:blocked" {
			action = "unblock"
			if strings.TrimSpace(latest.Assignee) == "" {
				nextState = "todo"
			} else {
				nextState = "doing"
			}
		}

		if err := m.checkActionAllowed(action, latest, route); err != nil {
			return actionDoneMsg{action: action, issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}

		if err := m.service.CommentIssue(m.ctx, outbox.CommentIssueInput{
			IssueRef: latest.IssueRef,
			Actor:    route.Assignee,
			Body:     "console " + action,
			State:    nextState,
		}); err != nil {
			return actionDoneMsg{action: action, issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}
		return actionDoneMsg{action: action, issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, result: "state:" + nextState}
	}
}

func (m *pmModel) closeCmd() tea.Cmd {
	selected, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	m.status = "执行 close 中..."
	return func() tea.Msg {
		latest, err := m.service.GetIssue(m.ctx, selected.IssueRef)
		if err != nil {
			return actionDoneMsg{action: "close", issueRef: selected.IssueRef, err: err}
		}
		route := m.resolveRoute(latest.IssueRef, latest.Assignee, latest.Labels)
		if err := m.checkActionAllowed("close", latest, route); err != nil {
			return actionDoneMsg{action: "close", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}

		if err := m.service.CloseIssue(m.ctx, outbox.CloseIssueInput{
			IssueRef: latest.IssueRef,
			Actor:    route.Assignee,
			Comment:  "console close",
		}); err != nil {
			return actionDoneMsg{action: "close", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, err: err}
		}
		return actionDoneMsg{action: "close", issueRef: selected.IssueRef, role: route.Role, actor: route.Assignee, result: "closed"}
	}
}

func (m *pmModel) selectedIssue() (outbox.IssueListItem, bool) {
	if len(m.issues) == 0 {
		return outbox.IssueListItem{}, false
	}
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.issues) {
		return outbox.IssueListItem{}, false
	}
	return m.issues[m.selectedIndex], true
}

func (m *pmModel) isCurrentSelectedIssue(issueRef string) bool {
	selected, ok := m.selectedIssue()
	if !ok {
		return false
	}
	return strings.TrimSpace(selected.IssueRef) == strings.TrimSpace(issueRef)
}

func (m *pmModel) resolveRoute(issueRef string, assignee string, labels []string) issueRoute {
	_ = issueRef

	if len(m.enabledRoleSet) == 0 {
		return issueRoute{
			Source: "unknown",
			Err:    errors.New("workflow roles not loaded"),
		}
	}

	if role, ok := parseLeadRoleFromAssignee(assignee); ok {
		if _, enabled := m.enabledRoleSet[role]; enabled {
			return issueRoute{
				Role:     role,
				Assignee: strings.TrimSpace(assignee),
				Source:   "assignee",
			}
		}
		return issueRoute{
			Source: "unknown",
			Err:    fmt.Errorf("assignee role %s is not enabled in workflow", role),
		}
	}

	roleMatches := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, label := range labels {
		normalized := strings.TrimSpace(label)
		if !strings.HasPrefix(normalized, "to:") {
			continue
		}
		role := strings.TrimSpace(strings.TrimPrefix(normalized, "to:"))
		if role == "" {
			continue
		}
		if _, enabled := m.enabledRoleSet[role]; !enabled {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roleMatches = append(roleMatches, role)
	}
	if len(roleMatches) == 1 {
		role := roleMatches[0]
		return issueRoute{
			Role:     role,
			Assignee: "lead-" + role,
			Source:   "to-label",
		}
	}
	if len(roleMatches) > 1 {
		return issueRoute{
			Source: "ambiguous",
			Err:    fmt.Errorf("multiple to:* labels: %s", strings.Join(roleMatches, ",")),
		}
	}

	if stateFromLabels(labels) == "state:review" {
		if _, enabled := m.enabledRoleSet["reviewer"]; enabled {
			return issueRoute{
				Role:     "reviewer",
				Assignee: "lead-reviewer",
				Source:   "state-review",
			}
		}
	}

	return issueRoute{
		Source: "unknown",
		Err:    errors.New("cannot resolve role (need assignee lead-<role> or exactly one to:<role> label)"),
	}
}

func (m *pmModel) checkActionAllowed(action string, latest outbox.IssueDetail, route issueRoute) error {
	if latest.IsClosed {
		return errors.New("issue is closed")
	}
	if route.Role == "" || route.Assignee == "" || route.Source == "unknown" || route.Source == "ambiguous" {
		if route.Err != nil {
			return route.Err
		}
		return errors.New("route is required")
	}

	if strings.TrimSpace(latest.Assignee) == "" {
		if action == "claim" || action == "claim/unclaim" {
			return nil
		}
		return errors.New("issue is not claimed (assignee is empty)")
	}
	if strings.TrimSpace(latest.Assignee) != strings.TrimSpace(route.Assignee) {
		return fmt.Errorf("issue assignee=%s does not match actor=%s", latest.Assignee, route.Assignee)
	}

	if hasLabel(latest.Labels, "needs-human") {
		switch action {
		case "spawn", "switch", "reply", "unblock", "close":
			return errors.New("needs-human present; auto advance action is not allowed")
		}
	}

	if hasLabel(latest.Labels, "autoflow:off") {
		switch action {
		case "spawn", "switch":
			return errors.New("autoflow:off present; worker spawn is disabled")
		}
	}

	return nil
}

func (m *pmModel) appendAuditLog(action string, issueRef string, role string, actor string, result string, opErr error) {
	outcome := strings.TrimSpace(result)
	if opErr != nil {
		outcome = "error: " + opErr.Error()
	}
	if outcome == "" {
		outcome = "ok"
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s actor=%s role=%s issue=%s action=%s result=%s", timestamp, actor, role, issueRef, action, outcome)
	m.auditLogs = append([]string{line}, m.auditLogs...)
	if len(m.auditLogs) > maxAuditLines {
		m.auditLogs = m.auditLogs[:maxAuditLines]
	}

	logging.Info(m.ctx, "pm console action",
		slog.String("time", timestamp),
		slog.String("actor", actor),
		slog.String("route_role", role),
		slog.String("issue_ref", issueRef),
		slog.String("action", action),
		slog.String("result", outcome),
	)
}

func filterIssues(items []outbox.IssueListItem, scopeRole string, assigneeFilter string, stateFilter string) []outbox.IssueListItem {
	filtered := make([]outbox.IssueListItem, 0, len(items))
	role := normalizeScopeRole(scopeRole)
	assigneeFilter = strings.TrimSpace(assigneeFilter)

	for _, item := range items {
		if stateFilter != "" && stateFromLabels(item.Labels) != stateFilter {
			continue
		}

		if role == "all" {
			if assigneeFilter != "" && strings.TrimSpace(item.Assignee) != assigneeFilter {
				continue
			}
			filtered = append(filtered, item)
			continue
		}

		if !roleMatchesIssue(role, assigneeFilter, item) {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.SliceStable(filtered, func(i int, j int) bool {
		if filtered[i].UpdatedAt == filtered[j].UpdatedAt {
			return filtered[i].IssueRef < filtered[j].IssueRef
		}
		return filtered[i].UpdatedAt > filtered[j].UpdatedAt
	})
	return filtered
}

func roleMatchesIssue(role string, assignee string, issue outbox.IssueListItem) bool {
	normalizedRole := strings.TrimSpace(role)
	normalizedAssignee := strings.TrimSpace(assignee)
	if normalizedAssignee != "" && strings.TrimSpace(issue.Assignee) == normalizedAssignee {
		return true
	}

	if containsLabel(issue.Labels, "to:"+normalizedRole) {
		return true
	}
	if normalizedRole == "reviewer" && stateFromLabels(issue.Labels) == "state:review" {
		return true
	}
	return false
}

func normalizeScopeRole(input string) string {
	value := strings.TrimSpace(strings.ToLower(input))
	if value == "" {
		return "all"
	}
	switch value {
	case "*", "all":
		return "all"
	default:
		return value
	}
}

func normalizeStateFilter(input string) string {
	value := strings.TrimSpace(strings.ToLower(input))
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "state:") {
		return value
	}
	switch value {
	case "todo", "doing", "blocked", "review", "done":
		return "state:" + value
	default:
		return value
	}
}

func trimStatePrefix(state string) string {
	return strings.TrimPrefix(strings.TrimSpace(state), "state:")
}

func parseLeadRoleFromAssignee(assignee string) (string, bool) {
	normalized := strings.TrimSpace(assignee)
	if !strings.HasPrefix(normalized, "lead-") {
		return "", false
	}
	role := strings.TrimSpace(strings.TrimPrefix(normalized, "lead-"))
	if role == "" {
		return "", false
	}
	return role, true
}

func toStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	return set
}

func hasLabel(labels []string, target string) bool {
	normalizedTarget := strings.TrimSpace(target)
	for _, label := range labels {
		if strings.TrimSpace(label) == normalizedTarget {
			return true
		}
	}
	return false
}

func stateFromLabels(labels []string) string {
	for _, label := range labels {
		normalized := strings.TrimSpace(label)
		if strings.HasPrefix(normalized, "state:") {
			return normalized
		}
	}
	return ""
}

func containsLabel(labels []string, target string) bool {
	normalizedTarget := strings.TrimSpace(target)
	for _, label := range labels {
		if strings.TrimSpace(label) == normalizedTarget {
			return true
		}
	}
	return false
}

func mapClosedState(isClosed bool, stateLabel string) string {
	if isClosed {
		return "closed"
	}
	if strings.TrimSpace(stateLabel) != "" {
		return strings.TrimPrefix(stateLabel, "state:")
	}
	return "open"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized != "" {
			return normalized
		}
	}
	return ""
}

func firstNonEmptyLine(body string) string {
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line != "" {
			return line
		}
	}
	return "empty"
}
