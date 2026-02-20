package views

import (
	"flying_nimbus/internal/tui/common"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/bubbles/key"
	"flying_nimbus/internal/tui/constants"
	"log/slog"
)

var (
	modalOverlayStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	modalTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62"))

	modalActiveStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("170")).
		Padding(0, 2)

	modalInactiveStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("241")).
		Padding(0, 2)

	modalHelpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
)

type ModalAction struct {
	Label string
	Action any // Implementer makes an enum of actions
}

type ModalSelectMsg struct {
	Action any
}

type ModalCancelMsg struct{}

type ActionModel struct {
	title string
	cursor int
	inputRoutingStrategy common.InputRoutingStrategy
	actions []ModalAction
}

func NewActionModal(title string, actions []ModalAction) ActionModel {
	if len(actions) == 0 {
		slog.Error("No actions given to modal!")
	}

	return ActionModel{
		title: title,
		cursor: 0,
		actions: actions,
		inputRoutingStrategy: common.RouteFocusedFirst,
	}
}

func (m ActionModel) Commands() common.Commands {
	return []key.Binding{
		leftKey,
		rightKey,
		constants.Keymap.Enter,
		constants.Keymap.Back,
	}
}

func (m ActionModel) InputRoutingStrategy() common.InputRoutingStrategy {
	return m.inputRoutingStrategy
}

func (m ActionModel) Init() tea.Cmd {
	slog.Debug("Initializing action modal")
	return nil //Do nothing
}

func (m ActionModel) Update(msg tea.Msg) (ActionModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmd = m.handleKeypress(msg)
	}

	return m, cmd
}

func (m ActionModel) View() string {
	var buttons []string

	for i, action := range m.actions {
		style := modalInactiveStyle
		if i == m.cursor {
			style = modalActiveStyle
		}
		buttons = append(buttons, style.Render(action.Label))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Center, buttons...)

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		modalTitleStyle.Render(m.title),
		row,
		modalHelpStyle.Render("←/→: navigate • enter: select • esc: cancel"),
	)

	return modalOverlayStyle.Render(content)
}

func (m *ActionModel) handleKeypress(msg tea.KeyMsg) tea.Cmd {
	slog.Debug("Action Modal Keypress", "key", msg)
	var cmd tea.Cmd
	if key.Matches(msg, rightKey) {
		m.cursor = (m.cursor + 1) % len(m.actions)
	}
	if key.Matches(msg, leftKey) {
		// Add len of actions to avoid negatives
		m.cursor = (m.cursor - 1 + len(m.actions)) % len(m.actions)
	}
	if key.Matches(msg, constants.Keymap.Enter) {
		selected := m.actions[m.cursor]
		cmd = func() tea.Msg {
			return ModalSelectMsg{selected}
		}
	}
	if key.Matches(msg, constants.Keymap.Back) {
		cmd = func() tea.Msg {
			return ModalCancelMsg{}
		}
	}

	return cmd
}