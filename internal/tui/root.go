package tui

import (
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/tui/constants"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// NavigateMsg tells the RootModel to push a new screen onto the stack.
type NavigateMsg struct {
	Model tea.Model
}

// BackMsg tells the RootModel to pop the current screen.
type BackMsg struct{}

func InitRoot() RootModel {
	stack := make([]tea.Model, 0, 3)

	// Rename to not conflict with Standard library Context
	appService := &app.AppServices{}

	stack = append(stack, NewProvidersModel(appService))

	return RootModel{
		appService: appService,
		stack:      stack,
	}

}

type RootModel struct {
	appService *app.AppServices
	// Might need for mutex locks
	stack []tea.Model
}

func (m RootModel) Init() tea.Cmd {
	return nil
}

func (m RootModel) View() string {
	current := m.stack[len(m.stack)-1]
	return current.View()
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		constants.WindowSize = msg
	case NavigateMsg:
		m.stack = append(m.stack, msg.Model)
		return m, msg.Model.Init()

	case BackMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, constants.Keymap.Back):
			return m, func() tea.Msg {
				return BackMsg{}
			}
		case key.Matches(msg, constants.Keymap.Quit):
			return m, tea.Quit
		}
	}

	// delegate everything else
	current := m.stack[len(m.stack)-1]
	next, cmd := current.Update(msg)
	m.stack[len(m.stack)-1] = next
	return m, cmd
}
