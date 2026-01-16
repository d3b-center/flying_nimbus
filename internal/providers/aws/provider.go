package aws

import (
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type AwsProviderModel struct {
	appService *app.App
	menu       list.Model
}

func (m AwsProviderModel) Init() tea.Cmd {
	return nil
}

func (m AwsProviderModel) View() string {
	return constants.DocStyle.Render(m.menu.View() + "\n")
}

func (m AwsProviderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

func NewAWSProviderModel(appService *app.App) AwsProviderModel {
	items := []list.Item{
		common.NewNavItem(
			"EC2",
			"Manage EC2 Resources",
			func(appService *app.App) tea.Model {
				// NewAWSProviderModel Please Remove (dummy model)
				return NewAWSProviderModel(appService)
			},
		),
		common.NewNavItem(
			"RDS",
			"Manage RDS Resources",
			func(appService *app.App) tea.Model {
				// NewAWSProviderModel Please Remove (dummy model)
				return NewAWSProviderModel(appService)
			},
		),
		common.NewNavItem(
			"Service Catalog",
			"Service Catalog",
			func(appService *app.App) tea.Model {
				// NewAWSProviderModel Please Remove (dummy model)
				return NewAWSProviderModel(appService)
			},
		),
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select Capability"
	h, v := constants.DocStyle.GetFrameSize()
	l.SetSize(constants.WindowSize.Width-h, constants.WindowSize.Height-v)
	return AwsProviderModel{
		appService: appService,
		menu:       l,
	}
}
