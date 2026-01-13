package tui

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

func StartTea() error {
	// Add logfile ()

	m := InitRoot()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal("Error running program:", err)
	}
	return nil
}
