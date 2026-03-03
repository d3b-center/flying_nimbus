package components

import (
	"flying_nimbus/internal/tui/constants"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

type ActionItem struct {
	Label  string
	Action func() tea.Cmd // Implementer supplies function to call
}

type ModalCancelMsg struct{}
type ModalResponseMsg struct{ Err error }

type ActionMenu struct {
	title   string
	cursor  int
	actions []ActionItem
}

func NewActionModal(title string, actions []ActionItem) ActionMenu {
	if len(actions) == 0 {
		slog.Error("No actions given to modal!")
	}

	return ActionMenu{
		title:   title,
		cursor:  0,
		actions: actions,
	}
}

func (m ActionMenu) Init() tea.Cmd {
	slog.Debug("Initializing action modal")
	return nil //Do nothing
}

func (m ActionMenu) Update(msg tea.Msg) (ActionMenu, tea.Cmd) {
	var cmd tea.Cmd

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		cmd = m.handleKeypress(keyMsg)
	}

	return m, cmd
}

func (m ActionMenu) View() string {
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

func (m *ActionMenu) handleKeypress(msg tea.KeyMsg) tea.Cmd {
	slog.Debug("Action Modal Keypress", "key", msg)
	var cmd tea.Cmd
	switch {
	case key.Matches(msg, RightKey), key.Matches(msg, LeftKey):
		m.moveCursor(msg)
	case key.Matches(msg, constants.Keymap.Enter):
		selected := m.actions[m.cursor]
		cmd = selected.Action()
	case key.Matches(msg, constants.Keymap.Back):
		cmd = func() tea.Msg {
			return ModalCancelMsg{}
		}
	}

	return cmd
}

func (m *ActionMenu) moveCursor(msg tea.KeyMsg) {
	switch {
	case key.Matches(msg, RightKey):
		m.cursor = (m.cursor + 1) % len(m.actions)
	case key.Matches(msg, LeftKey):
		m.cursor = (m.cursor - 1 + len(m.actions)) % len(m.actions)
	}
}
