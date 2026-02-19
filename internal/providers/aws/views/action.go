package views

import (
	"flying_nimbus/internal/tui/common"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/bubbles/key"
)

const (
	// ActionSSMShell ModalAction = iota
	// ActionSSMPortForward
	// ActionSSMRemotePortForward
	// ActionStartInstance
	// ActionStopInstance

	leftKey key.Binding = key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("left/h", "left"),
	)
	rightKey key.Binding = key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("right/l", "right"),
	)
)

type ModalAction struct {
	label string
	action func
}

type ModalCancelMsg struct{}

type ModalModel struct {
	title string
	cursor int
	actions []ModalAction
}

func NewModal(title string, actions []ModalAction) ModalModel {
	if len(actions) == 0 {
		slog.Error("No actions given to modal!")
		actions = {}
	}

	return ModalModel{
		title: title,
		cursor: 0,
		actions: actions,
	}
}

func (m ModalModel) Commands() common.Commands {
	keys := make([]key.Binding, 2)

	keys.append(leftKey)
	keys.append(rightKey)

	return keys
}

func (m ModalModel) Update(msg tea.Msg) (ModalModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmd = handleKeypress(msg)
	}

	return m, cmd
}

func (m ModalModel) handleKeypress(msg tea.KeyMsg) tea.Cmd {
	if key.Matches(msg, rightKey) {
		if cursor == len(m.actions) - 1 {
			cursor = 0
		} else {
			cursor += 1
		}
	}
	if key.Matches(msg, leftKey) {
		if cursor == 0 {
			cursor = len(m.actions) - 1
		} else {
			cursor -= 1
		}
	}

	return nil
}