package aws

import (
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/providers/aws/views"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type AwsProviderModel struct {
	app    *app.App
	menu   list.Model
	window common.ContentWindowSizeMsg
}

func (m AwsProviderModel) Title() string {
	return "AWS"
}

func (m AwsProviderModel) Commands() common.Commands {
	return make([]key.Binding, 0)
}

func (m AwsProviderModel) Init() tea.Cmd {
	return nil
}

func (m AwsProviderModel) View() string {
	m.menu.SetSize(m.window.Width, m.window.Height)
	return m.menu.View() + "\n"
}

func (m AwsProviderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		slog.Debug(fmt.Sprintf("Window Size ProvidersInit %v", msg))
		m.window = msg
		top, right, bottom, left := constants.DocStyle.GetMargin()
		m.menu.SetSize(msg.Width-left-right, msg.Height-top-bottom-1)
	}

	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, constants.Keymap.Enter):
			item := m.menu.SelectedItem().(common.NavItem)
			return m, func() tea.Msg {
				return common.NavigateMsg{
					Model: item.Model(m.app, m.window),
				}
			}
		}
	}
	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

func NewAWSProviderModel(appService *app.App, windowSize common.ContentWindowSizeMsg) AwsProviderModel {
	items := []list.Item{
		common.NewNavItem(
			"EC2",
			"Manage EC2 Resources",
			func(appService *app.App, windowSize common.ContentWindowSizeMsg) common.NimbusModel {
				// NewAWSProviderModel Please Remove (dummy model)
				return NewAWSProviderModel(appService, windowSize)
			},
		),
		common.NewNavItem(
			"RDS",
			"Manage RDS Resources",
			func(appService *app.App, windowSize common.ContentWindowSizeMsg) common.NimbusModel {
				return views.InitRdsViewModel(appService, windowSize)
			},
		),
		common.NewNavItem(
			"Service Catalog",
			"Service Catalog",
			func(appService *app.App, windowSize common.ContentWindowSizeMsg) common.NimbusModel {
				// NewAWSProviderModel Please Remove (dummy model)
				return NewAWSProviderModel(appService, windowSize)
			},
		),
	}

	l := list.New(items, list.NewDefaultDelegate(), windowSize.Width, windowSize.Height)
	l.Title = "Select Capability"
	l.SetShowHelp(false)
	return AwsProviderModel{
		app:    appService,
		menu:   l,
		window: windowSize,
	}
}
