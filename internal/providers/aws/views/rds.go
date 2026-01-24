package views

import (
	"context"
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))
)

type RdsViewModel struct {
	app       *app.App
	list      list.Model
	loader    spinner.Model
	isLoading bool
}

type (
	rdsInstancesLoadedMsg []list.Item
)

func InitRdsViewModel(appService *app.App) RdsViewModel {
	items := []list.Item{}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Rds"
	h, v := constants.DocStyle.GetFrameSize()
	l.SetSize(constants.WindowSize.Width-h, constants.WindowSize.Height-v)

	loader := spinner.New()
	loader.Style = spinnerStyle
	loader.Spinner = spinner.Dot

	return RdsViewModel{
		app:       appService,
		loader:    loader,
		isLoading: true,
	}
}

func fetchRdsInstancesCmd(ctx context.Context, rdsService *aws.RdsService) tea.Cmd {
	return func() tea.Msg {
		dbs, _ := rdsService.ListDBInstances(ctx)
		return rdsInstancesLoadedMsg(dbInstancesToItems(dbs))
	}
}

func (m RdsViewModel) Init() tea.Cmd {
	slog.Debug("Initialize Rds Model")
	return tea.Batch(m.loader.Tick, fetchRdsInstancesCmd(m.app.Context, m.app.AWS.Rds))
}

func (m RdsViewModel) View() string {
	if m.isLoading {
		return constants.DocStyle.Render(m.loader.View() + "\n")
	}
	return constants.DocStyle.Render(m.list.View() + "\n")
}

func (m RdsViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	slog.Debug(fmt.Sprintf("%v", msg))
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case rdsInstancesLoadedMsg:
		m.isLoading = false
		m.list = toTeaList(msg)
		slog.Debug(fmt.Sprintf("Size of list %d", len(m.list.Items())))
	case spinner.TickMsg:
		slog.Debug("Spinning")
		m.loader, cmd = m.loader.Update(msg)
	default:
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func dbInstancesToItems(dbs []aws.RDSInstance) []list.Item {
	items := make([]list.Item, len(dbs))
	for i, db := range dbs {
		items[i] = list.Item(db)
	}
	return items
}

func toTeaList(rdsInstances rdsInstancesLoadedMsg) list.Model {
	h, v := constants.DocStyle.GetFrameSize()
	//list.SetSize(constants.WindowSize.Width-h, constants.WindowSize.Height-v)
	list := list.New(rdsInstances, list.NewDefaultDelegate(), constants.WindowSize.Width-h, constants.WindowSize.Height-v)

	list.Title = "Instances"
	list.SetShowStatusBar(false)

	return list
}
