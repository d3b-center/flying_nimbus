package constants

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WindowSize store the size of the terminal window
var WindowSize tea.WindowSizeMsg
var DocStyle = lipgloss.NewStyle().Margin(0, 2)

type keymap struct {
	Enter         key.Binding
	Back          key.Binding
	Quit          key.Binding
	ToggleDevConsole key.Binding
}

// Keymap reusable key mappings shared across models
var Keymap = keymap{
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c", "q"),
		key.WithHelp("ctrl+c/q", "quit"),
	),
	ToggleDevConsole: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "toggle dev console"),
	),
}
