package tui

import (
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/providers/aws"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type ProvidersModel struct {
	appService *app.AppServices
	list       list.Model
}

func NewProvidersModel(appService *app.AppServices) ProvidersModel {
	items := []list.Item{
		common.NewNavItem(
			"aws",
			"Amazon Web Services",
			func(appService *app.AppServices) tea.Model {
				return aws.NewAWSProviderModel(appService)
			},
		),
		common.NewNavItem(
			"azure",
			"Azure (not good)",
			func(appService *app.AppServices) tea.Model {
				return aws.NewAWSProviderModel(appService)
			},
		),
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select Provider!!"

	return ProvidersModel{
		appService: appService,
		list:       l,
	}
}

func (m ProvidersModel) Init() tea.Cmd {
	return nil
}

func (m ProvidersModel) View() string {
	return constants.DocStyle.Render(m.list.View() + "\n")
}

func (m ProvidersModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		constants.WindowSize = msg
		top, right, bottom, left := constants.DocStyle.GetMargin()
		m.list.SetSize(msg.Width-left-right, msg.Height-top-bottom-1)
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			item := m.list.SelectedItem().(common.NavItem)
			return m, func() tea.Msg {
				return NavigateMsg{
					Model: item.Model(m.appService),
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
