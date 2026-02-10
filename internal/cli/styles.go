package cli

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	Title     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	Subtitle  = lipgloss.NewStyle().Faint(true)
	Success   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	Warning   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	Error     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	Highlight = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	Prompt    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	Dim       = lipgloss.NewStyle().Faint(true)
)

// Link formats a clickable hyperlink using the OSC 8 escape sequence.
// Terminals that support it render the text as a clickable link.
// Terminals that don't gracefully fall back to showing just the text.
func Link(url, text string) string {
	return "\033]8;;" + url + "\033\\" + text + "\033]8;;\033\\"
}
