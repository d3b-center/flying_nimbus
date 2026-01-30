package views

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	spinnerStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Align(lipgloss.Center)
	instancesListStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
	instanceDetailStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
	headerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1)
	sectionHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("62")).
				PaddingBottom(1)
)
