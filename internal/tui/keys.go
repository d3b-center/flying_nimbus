package tui

import (
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type keymap struct {
	Enter            key.Binding
	Back             key.Binding
	ToggleDevConsole key.Binding
	ShowFullHelp     key.Binding
	CloseFullHelp    key.Binding
	Quit             key.Binding
	ForceQuit        key.Binding
}

// Keymap reusable key mappings shared across models
var DefaultKeymap = keymap{
	Enter: constants.Keymap.Enter,

	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),

	// Toggle help.
	ShowFullHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "more"),
	),
	CloseFullHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "close help"),
	),

	// Toggle Dev Console
	ToggleDevConsole: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "toggle dev console"),
	),

	// Quitting.
	Quit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	ForceQuit: key.NewBinding(key.WithKeys("ctrl+c")),
}

type nimbusKeyMap struct {
	Global  keymap
	Dynamic common.Commands
}

func (m nimbusKeyMap) ShortHelp() []key.Binding {
	kb := []key.Binding{
		constants.Keymap.CursorUp,
		constants.Keymap.CursorDown,
		m.Global.Enter,
		m.Global.Back,
		m.Global.ToggleDevConsole,
		m.Global.ShowFullHelp,
	}

	return kb
}

func (m nimbusKeyMap) FullHelp() [][]key.Binding {

	kb := [][]key.Binding{{
		constants.Keymap.CursorUp,
		constants.Keymap.CursorDown,
		m.Global.Enter,
		m.Global.Back,
		m.Global.ToggleDevConsole,
	}}

	return kb
}

func GenerateNimbusKeyMap(m tea.Model) nimbusKeyMap {
	var activeCmds []key.Binding
	if nimbus, ok := m.(common.NimbusModel); ok {
		activeCmds = nimbus.Commands()
	}

	return nimbusKeyMap{
		Global:  DefaultKeymap,
		Dynamic: activeCmds,
	}

}
