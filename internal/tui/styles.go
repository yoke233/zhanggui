package tui

import "github.com/charmbracelet/lipgloss"

var (
	StyleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	StyleStatus = map[string]lipgloss.Style{
		"created":        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		"running":        lipgloss.NewStyle().Foreground(lipgloss.Color("32")),
		"waiting_review": lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		"done":           lipgloss.NewStyle().Foreground(lipgloss.Color("34")),
		"failed":         lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		"timeout":        lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
	StyleHelp  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	StyleInput = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)
)
