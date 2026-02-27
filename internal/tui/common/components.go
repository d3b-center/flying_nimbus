package common

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func RenderModal(modalContent string, window tea.WindowSizeMsg) string {

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")). // Match your TitleBar color
		Padding(1, 2)

	uiWithModal := modalStyle.Render(modalContent)

	return lipgloss.Place(
		window.Width,
		window.Height,
		lipgloss.Center,
		lipgloss.Center,
		uiWithModal,
		lipgloss.WithWhitespaceChars("--"), // This uses your actual app as the "underlay"
		lipgloss.WithWhitespaceForeground(lipgloss.NoColor{}),
	)

}

func RenderErrorModal(
	title string,
	message []string,
	footer string,
	window tea.WindowSizeMsg,
) string {

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("1")) // red

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250"))

	footerStyle := lipgloss.NewStyle().
		Faint(true)

	content := []string{
		titleStyle.Render(title),
		"",
	}

	for _, line := range message {
		content = append(content, bodyStyle.Render(line))
	}

	if footer != "" {
		content = append(content, "", footerStyle.Render(footer))
	}

	modalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		content...,
	)

	return RenderModal(modalContent, window)
}
