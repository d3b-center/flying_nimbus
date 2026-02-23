package constants

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// WindowSize store the size of the terminal window
var DocStyle = lipgloss.NewStyle().Margin(1, 1)

type keymap struct {
	Enter      key.Binding
	Back       key.Binding
	CursorUp   key.Binding
	CursorDown key.Binding
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
	CursorUp: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	CursorDown: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
}
