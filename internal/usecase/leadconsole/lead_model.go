package leadconsole

import (
	"context"
	"fmt"
	"log/slog"
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

type LeadOptions struct {
	Role            string
	Assignee        string
	StateFilter     string
	WorkflowFile    string
	ConfigFile      string
	ExecutablePath  string
	RefreshInterval time.Duration
}

type leadModel struct {
	ctx             context.Context
	service         *outbox.Service
	role            string
	assignee        string
	stateFilter     string
	workflowFile    string
	configFile      string
	executablePath  string
	refreshInterval time.Duration

	issues         []outbox.IssueListItem
	selectedIndex  int
	detail         outbox.IssueDetail
	hasDetail      bool
	activeRunID    string
	activeRunFound bool
	status         string
	auditLogs      []string
}

type issuesLoadedMsg struct {
	items []outbox.IssueListItem
	err   error
}

type issueDetailLoadedMsg struct {
	issueRef       string
	detail         outbox.IssueDetail
	hasDetail      bool
	activeRunID    string
	activeRunFound bool
	err            error
}

type tickMsg struct{}

type actionDoneMsg struct {
	action   string
	issueRef string
	result   string
	err      error
}

func NewLeadModel(ctx context.Context, service *outbox.Service, options LeadOptions) tea.Model {
	role := strings.TrimSpace(options.Role)
	if role == "" {
		role = "backend"
	}
	assignee := strings.TrimSpace(options.Assignee)
	if assignee == "" {
		assignee = "lead-" + role
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

	return &leadModel{
		ctx:             ctx,
		service:         service,
		role:            role,
		assignee:        assignee,
		stateFilter:     stateFilter,
		workflowFile:    workflowFile,
		configFile:      configFile,
		executablePath:  executablePath,
		refreshInterval: interval,
		status:          "初始化中",
	}
}

func (m *leadModel) Init() tea.Cmd {
	return tea.Batch(m.loadIssuesCmd(), m.tickCmd())
}

func (m *leadModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tickMsg:
		return m, tea.Batch(m.loadIssuesCmd(), m.tickCmd())
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
			m.status = "详情加载失败: " + msg.err.Error()
			return m, nil
		}
		m.hasDetail = msg.hasDetail
		m.detail = msg.detail
		m.activeRunID = msg.activeRunID
		m.activeRunFound = msg.activeRunFound
		return m, nil
	case actionDoneMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("%s 失败: %v", msg.action, msg.err)
			m.appendAuditLog(msg.action, msg.issueRef, "failed", msg.err)
		} else {
			m.status = fmt.Sprintf("%s 完成: %s", msg.action, msg.result)
			m.appendAuditLog(msg.action, msg.issueRef, msg.result, nil)
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
		}
	}
	return m, nil
}

func (m *leadModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62"))

	var builder strings.Builder
	builder.WriteString(titleStyle.Render("Lead Console 2.1"))
	builder.WriteString("\n")
	builder.WriteString(dimStyle.Render(fmt.Sprintf(
		"role=%s assignee=%s state=%s workflow=%s refresh=%s",
		m.role,
		m.assignee,
		firstNonEmpty(m.stateFilter, "all"),
		m.workflowFile,
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
			line := fmt.Sprintf("%s [%s] assignee=%s title=%s", item.IssueRef, strings.TrimPrefix(state, "state:"), issueAssignee, item.Title)
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
		builder.WriteString(fmt.Sprintf("IssueRef: %s\n", m.detail.IssueRef))
		builder.WriteString(fmt.Sprintf("Status: %s\n", mapClosedState(m.detail.IsClosed, stateFromLabels(m.detail.Labels))))
		builder.WriteString(fmt.Sprintf("Assignee: %s\n", firstNonEmpty(strings.TrimSpace(m.detail.Assignee), "-")))
		if m.activeRunFound {
			builder.WriteString(fmt.Sprintf("ActiveRun: %s\n", m.activeRunID))
		} else {
			builder.WriteString("ActiveRun: none\n")
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

	builder.WriteString(dimStyle.Render("Keys: ↑/k ↓/j 移动  g 刷新  c/s/w/r/b/x 动作  q 退出"))
	return builder.String()
}

func (m *leadModel) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *leadModel) loadIssuesCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.service.ListIssues(m.ctx, false, "")
		if err != nil {
			return issuesLoadedMsg{err: err}
		}
		filtered := filterIssues(items, m.role, m.assignee, m.stateFilter)
		return issuesLoadedMsg{items: filtered}
	}
}

func (m *leadModel) loadSelectedIssueDetailCmd() tea.Cmd {
	if len(m.issues) == 0 {
		return nil
	}
	selected := m.issues[m.selectedIndex]
	return func() tea.Msg {
		detail, err := m.service.GetIssue(m.ctx, selected.IssueRef)
		if err != nil {
			return issueDetailLoadedMsg{
				issueRef: selected.IssueRef,
				err:      err,
			}
		}
		activeRunID, found, err := m.service.GetActiveRunID(m.ctx, m.role, selected.IssueRef)
		if err != nil {
			return issueDetailLoadedMsg{
				issueRef: selected.IssueRef,
				err:      err,
			}
		}
		return issueDetailLoadedMsg{
			issueRef:       selected.IssueRef,
			detail:         detail,
			hasDetail:      true,
			activeRunID:    activeRunID,
			activeRunFound: found,
		}
	}
}

func (m *leadModel) claimOrUnclaimCmd() tea.Cmd {
	issue, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	isClaimedBySelf := strings.TrimSpace(issue.Assignee) == m.assignee
	issueRef := issue.IssueRef
	m.status = "执行 claim/unclaim 中..."
	return func() tea.Msg {
		if isClaimedBySelf {
			err := m.service.UnclaimIssue(m.ctx, outbox.UnclaimIssueInput{
				IssueRef: issueRef,
				Actor:    m.assignee,
				Comment:  "console unclaim",
			})
			if err != nil {
				return actionDoneMsg{action: "unclaim", issueRef: issueRef, err: err}
			}
			return actionDoneMsg{action: "unclaim", issueRef: issueRef, result: "state:todo"}
		}

		err := m.service.ClaimIssue(m.ctx, outbox.ClaimIssueInput{
			IssueRef: issueRef,
			Assignee: m.assignee,
			Actor:    m.assignee,
			Comment:  "console claim",
		})
		if err != nil {
			return actionDoneMsg{action: "claim", issueRef: issueRef, err: err}
		}
		return actionDoneMsg{action: "claim", issueRef: issueRef, result: m.assignee}
	}
}

func (m *leadModel) spawnCmd(forceSpawn bool) tea.Cmd {
	issue, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	action := "spawn"
	if forceSpawn {
		action = "switch"
	}
	if m.shouldBlockAutoAction(action) {
		m.status = "存在 needs-human，禁止自动推进动作"
		return nil
	}
	issueRef := issue.IssueRef
	m.status = "执行 " + action + " 中..."
	return func() tea.Msg {
		result, err := m.service.LeadRunIssueOnce(m.ctx, outbox.LeadRunIssueInput{
			Role:           m.role,
			Assignee:       m.assignee,
			IssueRef:       issueRef,
			WorkflowFile:   m.workflowFile,
			ConfigFile:     m.configFile,
			ExecutablePath: m.executablePath,
			ForceSpawn:     forceSpawn,
		})
		if err != nil {
			return actionDoneMsg{action: action, issueRef: issueRef, err: err}
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
			issueRef: issueRef,
			result:   status,
		}
	}
}

func (m *leadModel) replyCmd() tea.Cmd {
	issue, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	if m.shouldBlockAutoAction("reply") {
		m.status = "存在 needs-human，禁止自动推进动作"
		return nil
	}

	state := trimStatePrefix(stateFromLabels(issue.Labels))
	if state == "" {
		state = "doing"
	}

	issueRef := issue.IssueRef
	m.status = "执行 normalize+reply 中..."
	return func() tea.Msg {
		err := m.service.CommentIssue(m.ctx, outbox.CommentIssueInput{
			IssueRef: issueRef,
			Actor:    m.assignee,
			Body:     "console normalized reply",
			State:    state,
		})
		if err != nil {
			return actionDoneMsg{action: "reply", issueRef: issueRef, err: err}
		}
		return actionDoneMsg{action: "reply", issueRef: issueRef, result: "state:" + state}
	}
}

func (m *leadModel) toggleBlockedCmd() tea.Cmd {
	issue, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}

	currentState := stateFromLabels(issue.Labels)
	nextState := "blocked"
	action := "block"
	if currentState == "state:blocked" {
		if m.shouldBlockAutoAction("unblock") {
			m.status = "存在 needs-human，禁止自动推进动作"
			return nil
		}
		action = "unblock"
		if strings.TrimSpace(issue.Assignee) == "" {
			nextState = "todo"
		} else {
			nextState = "doing"
		}
	}

	issueRef := issue.IssueRef
	m.status = "执行 " + action + " 中..."
	return func() tea.Msg {
		err := m.service.CommentIssue(m.ctx, outbox.CommentIssueInput{
			IssueRef: issueRef,
			Actor:    m.assignee,
			Body:     "console " + action,
			State:    nextState,
		})
		if err != nil {
			return actionDoneMsg{action: action, issueRef: issueRef, err: err}
		}
		return actionDoneMsg{action: action, issueRef: issueRef, result: "state:" + nextState}
	}
}

func (m *leadModel) closeCmd() tea.Cmd {
	issue, ok := m.selectedIssue()
	if !ok {
		m.status = "没有可操作 issue"
		return nil
	}
	if m.shouldBlockAutoAction("close") {
		m.status = "存在 needs-human，禁止自动推进动作"
		return nil
	}

	issueRef := issue.IssueRef
	m.status = "执行 close 中..."
	return func() tea.Msg {
		err := m.service.CloseIssue(m.ctx, outbox.CloseIssueInput{
			IssueRef: issueRef,
			Actor:    m.assignee,
			Comment:  "console close",
		})
		if err != nil {
			return actionDoneMsg{action: "close", issueRef: issueRef, err: err}
		}
		return actionDoneMsg{action: "close", issueRef: issueRef, result: "closed"}
	}
}

func (m *leadModel) selectedIssue() (outbox.IssueListItem, bool) {
	if len(m.issues) == 0 {
		return outbox.IssueListItem{}, false
	}
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.issues) {
		return outbox.IssueListItem{}, false
	}
	return m.issues[m.selectedIndex], true
}

func (m *leadModel) isCurrentSelectedIssue(issueRef string) bool {
	selected, ok := m.selectedIssue()
	if !ok {
		return false
	}
	return strings.TrimSpace(selected.IssueRef) == strings.TrimSpace(issueRef)
}

func (m *leadModel) shouldBlockAutoAction(action string) bool {
	if !m.hasDetail || !containsLabel(m.detail.Labels, "needs-human") {
		return false
	}
	switch action {
	case "spawn", "switch", "reply", "unblock", "close":
		return true
	default:
		return false
	}
}

func (m *leadModel) appendAuditLog(action string, issueRef string, result string, opErr error) {
	outcome := strings.TrimSpace(result)
	if opErr != nil {
		outcome = "error: " + opErr.Error()
	}
	if outcome == "" {
		outcome = "ok"
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s actor=%s issue=%s action=%s result=%s", timestamp, m.assignee, issueRef, action, outcome)
	m.auditLogs = append([]string{line}, m.auditLogs...)
	if len(m.auditLogs) > maxAuditLines {
		m.auditLogs = m.auditLogs[:maxAuditLines]
	}

	logging.Info(m.ctx, "lead console action",
		slog.String("time", timestamp),
		slog.String("actor", m.assignee),
		slog.String("issue_ref", issueRef),
		slog.String("action", action),
		slog.String("result", outcome),
	)
}

func filterIssues(items []outbox.IssueListItem, role string, assignee string, stateFilter string) []outbox.IssueListItem {
	filtered := make([]outbox.IssueListItem, 0, len(items))
	for _, item := range items {
		if !roleMatchesIssue(role, assignee, item) {
			continue
		}
		if stateFilter != "" && stateFromLabels(item.Labels) != stateFilter {
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
