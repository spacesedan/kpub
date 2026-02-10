package cli

import (
	"strings"

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

// FilterDockerLine decides whether a docker output line is worth showing
// and returns a cleaned-up version. Returns ("", false) for noisy lines.
func FilterDockerLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}

	lower := strings.ToLower(trimmed)

	// BuildKit output: lines starting with "#N ..."
	if strings.HasPrefix(lower, "#") {
		parts := strings.SplitN(trimmed, " ", 3)
		if len(parts) < 2 {
			return "", false
		}
		second := strings.ToLower(parts[1])

		// Always skip raw sha256 digest lines — pure noise.
		if strings.HasPrefix(second, "sha256:") {
			return "", false
		}

		// Skip "..." placeholder lines.
		if second == "..." {
			return "", false
		}

		// For DONE/CACHED lines like "#5 DONE 1.2s", show as "done (1.2s)"
		// so the user sees progress ticking forward.
		if second == "done" || second == "cached" {
			tag := second
			if len(parts) == 3 {
				tag += " (" + strings.TrimSpace(parts[2]) + ")"
			}
			return tag, true
		}

		// Keep extracting/downloading progress.
		if second == "extracting" || second == "downloading" || second == "exporting" {
			if len(parts) == 3 {
				return second + " " + strings.TrimSpace(parts[2]), true
			}
			return second, true
		}

		// Everything else is a step description like "#5 [2/5] COPY go.mod ."
		// Strip the "#N " prefix for cleanliness.
		desc := strings.SplitN(trimmed, " ", 2)
		if len(desc) == 2 {
			return desc[1], true
		}
		return trimmed, true
	}

	// Docker pull output: "abc123: Downloading 12.3MB/45.6MB" or "abc123: Pull complete"
	// Strip the layer hash prefix and show just the action + progress.
	if parts := strings.SplitN(trimmed, ": ", 2); len(parts) == 2 {
		action := strings.TrimSpace(parts[1])
		actionLower := strings.ToLower(action)

		// Skip noisy status lines.
		if strings.Contains(actionLower, "already exists") ||
			strings.Contains(actionLower, "waiting") ||
			strings.Contains(actionLower, "verifying checksum") ||
			actionLower == "pulling fs layer" {
			return "", false
		}

		// Show meaningful progress: "Downloading 12.3MB/45.6MB", "Extracting 100%", "Pull complete"
		if strings.Contains(actionLower, "downloading") ||
			strings.Contains(actionLower, "extracting") ||
			strings.Contains(actionLower, "pull complete") ||
			strings.Contains(actionLower, "download complete") {
			return action, true
		}

		return action, true
	}

	// Skip other noisy lines.
	if strings.Contains(lower, "already exists") ||
		strings.Contains(lower, "waiting") {
		return "", false
	}

	return trimmed, true
}

// TruncateLine truncates a string to maxLen characters, appending "..." if needed.
func TruncateLine(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
