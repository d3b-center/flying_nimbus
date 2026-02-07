package constants

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// WindowSize store the size of the terminal window
var DocStyle = lipgloss.NewStyle().Margin(2, 2)

const TitleBarInnerHeight = 1
const TitleBarBorderHeight = 2 // top + bottom
const TitleBarHeight = TitleBarBorderHeight + TitleBarInnerHeight

type keymap struct {
	Enter            key.Binding
	Back             key.Binding
	Quit             key.Binding
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
