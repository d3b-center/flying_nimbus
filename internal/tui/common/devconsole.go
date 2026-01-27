package common

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var devConsoleStyle = lipgloss.NewStyle().Padding(0, 1)
var devConsoleTitleStyle = lipgloss.NewStyle().Bold(true)

// RenderDevConsole shows the latest log lines within a fixed-height panel.
func RenderDevConsole(lines []string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	title := devConsoleTitleStyle.Render("Dev Console")
	contentHeight := height - 1
	if contentHeight < 0 {
		contentHeight = 0
	}

	start := len(lines) - contentHeight
	if start < 0 {
		start = 0
	}
	content := strings.Join(lines[start:], "\n")

	view := title
	if contentHeight > 0 {
		view = view + "\n" + content
	}

	return devConsoleStyle.Width(width).Height(height).Render(view)
}
