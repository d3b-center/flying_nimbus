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
	spinnerStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Align(lipgloss.Center)
	instancesListStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
	instanceDetailStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
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
	l.Title = "Instances"
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	//h, v := constants.DocStyle.GetFrameSize()
	//l.SetSize((constants.WindowSize.Width-h)/3, constants.WindowSize.Height-v)

	loader := spinner.New()
	loader.Style = spinnerStyle
	loader.Spinner = spinner.Dot

	return RdsViewModel{
		app:       appService,
		loader:    loader,
		list:      l,
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

	w, h := constants.DocStyle.GetFrameSize()
	contentWidth := constants.WindowSize.Width - w
	contentHeight := constants.WindowSize.Height - h

	instanceListWidth := contentWidth / 3
	detailsWidth := contentWidth - instanceListWidth

	m.list.SetSize(instanceListWidth, contentHeight)

	left := instancesListStyle.Width(instanceListWidth).Height(contentHeight).Render(m.list.View())
	right := instanceDetailStyle.Width(detailsWidth).Height(contentHeight).Render(generateInstanceDetail(m.list.SelectedItem()))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m RdsViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	slog.Debug(fmt.Sprintf("%v", msg))
	var cmd tea.Cmd
	switch msg := msg.(type) {
	// TODO: This should be a custom WindowSizeMsg from Root
	case tea.WindowSizeMsg:
		slog.Debug(fmt.Sprintf("Received WindowSizeMsg %v", msg))
		constants.WindowSize = msg

		h, v := constants.DocStyle.GetFrameSize()

		contentWidth := msg.Width - h
		contentHeight := msg.Height - v

		m.list.SetSize(contentWidth, contentHeight)
	case rdsInstancesLoadedMsg:
		m.isLoading = false
		m.list.SetItems(msg)
		slog.Debug(fmt.Sprintf("Size of list %d", len(m.list.Items())))
	case spinner.TickMsg:
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

func generateInstanceDetail(selectedItem list.Item) string {
	// Skeleton detail sections
	if selectedItem == nil {
		return "No Info"
	}

	rds, ok := selectedItem.(aws.RDSInstance)
	if !ok {
		return "No Info"
	}

	rows := []string{
		"Instance Details",
		"\n",
		"General Info",
		fmt.Sprintf("DB Identifier:    %s", rds.Id),
		fmt.Sprintf("Engine:           %s", rds.DbEngine),
		fmt.Sprintf("Version:          %s", rds.DbVersion),
		fmt.Sprintf("Instance Class:   %s", rds.InstanceClass),
		fmt.Sprintf("Status:           %s", rds.Status),
		fmt.Sprintf("Total Storage:    %d", rds.AllocatedStorage),
		fmt.Sprintf("Endpoint:         %s", rds.Endpoint),
		fmt.Sprintf("Port:             %d", rds.Port),
		fmt.Sprintf("IsPublic:         %t", rds.IsPubliclyAccessible),
		"\n",
		"Network",
		fmt.Sprintf("VPC:			   %s", rds.VpcID),
		fmt.Sprintf("Subnets:          %s", ""),
		"\n",
		"Metrics",
		fmt.Sprintf("CPU Utilization:  %s", "Noop"),
		fmt.Sprintf("Free Storage:     %s", "Noop"),
		fmt.Sprintf("Connections:      %s", "Noop"),
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
