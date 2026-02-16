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
