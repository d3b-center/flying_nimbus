package tui

import (
	"flying_nimbus/internal/app"
	tea "github.com/charmbracelet/bubbletea"
	"log/slog"
)

func StartTea(app *app.App) error {

	m := InitRoot()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		app.Logger.Error("Failed to Kickoff BubbleTea", slog.Any("error", err))
	}
	return nil
}
