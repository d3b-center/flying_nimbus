package tui

import (
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/providers/aws"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"github.com/charmbracelet/bubbles/key"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type ProvidersModel struct {
	app  *app.App
	list list.Model
}

func (m ProvidersModel) Title() string {
	return "Providers"
}

func (m ProvidersModel) Commands() common.Commands {
	return make(map[string]string)
}

func NewProvidersModel(application *app.App) ProvidersModel {
	items := []list.Item{
		common.NewNavItem(
			"aws",
			"Amazon Web Services",
			func(application *app.App) common.NimbusModel {
				return aws.NewAWSProviderModel(application)
			},
		),
		common.NewNavItem(
			"azure",
			"Azure (not good)",
			func(a *app.App) common.NimbusModel {
				return aws.NewAWSProviderModel(a)
			},
		),
	}

	l := list.New(items, list.NewDefaultDelegate(), constants.WindowSize.Width, constants.WindowSize.Height)
	l.Title = "Select Provider"

	return ProvidersModel{
		app:  application,
		list: l,
	}
}

func (m ProvidersModel) Init() tea.Cmd {
	return nil
}

func (m ProvidersModel) View() string {
	m.list.SetSize(constants.WindowSize.Width, constants.WindowSize.Height)
	return m.list.View() + "\n"
}

func (m ProvidersModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, constants.Keymap.Enter):
			item := m.list.SelectedItem().(common.NavItem)
			return m, func() tea.Msg {
				return common.NavigateMsg{
					Model: item.Model(m.app),
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
