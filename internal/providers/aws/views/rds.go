package views

import (
	"context"
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type RdsViewModel struct {
	app               *app.App
	list              list.Model
	loader            spinner.Model
	isLoading         bool
	windowSize        common.ContentWindowSizeMsg
	instanceListWidth int
	detailsWidth      int
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

	windowSize := common.ContentWindowSizeMsg{
		Height: constants.WindowSize.Height,
		Width:  constants.WindowSize.Width,
	}
	instanceListWidth := windowSize.Width / 3
	detailsWidth := windowSize.Width - instanceListWidth

	l.SetSize((constants.WindowSize.Width)/3, constants.WindowSize.Height)

	loader := spinner.New()
	loader.Style = spinnerStyle
	loader.Spinner = spinner.Dot

	return RdsViewModel{
		app:               appService,
		loader:            loader,
		list:              l,
		isLoading:         true,
		windowSize:        windowSize,
		instanceListWidth: instanceListWidth,
		detailsWidth:      detailsWidth,
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
	slog.Debug(fmt.Sprintf("List Height %d", m.list.Height()))
	slog.Debug(fmt.Sprintf("Height %d", m.windowSize.Height))

	left := instancesListStyle.Width(m.instanceListWidth).Height(m.windowSize.Height).Render(m.list.View())
	right := instanceDetailStyle.Width(m.detailsWidth).Height(m.windowSize.Height).Render(generateRdsInstanceDetail(m.list.SelectedItem()))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m RdsViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		slog.Debug(fmt.Sprintf("Received WindowSizeMsg %v", msg))

		m.windowSize = msg
		m.instanceListWidth = msg.Width / 3
		m.detailsWidth = msg.Width - m.instanceListWidth

		m.list.SetSize(m.instanceListWidth, msg.Height)
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

func generateRdsInstanceDetail(selectedItem list.Item) string {
	// Skeleton detail sections
	if selectedItem == nil {
		return "No Info"
	}

	rds, ok := selectedItem.(aws.RDSInstance)
	if !ok {
		return "No Info"
	}

	rows := []string{
		headerStyle.Render("Instance Details"),
		"",
		sectionHeaderStyle.Render("General Info"),
		common.KV("DB Identifier", rds.Id),
		common.KV("Engine", rds.DbEngine),
		common.KV("Version", rds.DbVersion),
		common.KV("Instance Class", rds.InstanceClass),
		common.KV("Status", rds.Status),
		common.KV("Total Storage", fmt.Sprintf("%d GiB", rds.AllocatedStorage)),
		common.KV("Endpoint", rds.Endpoint),
		common.KV("Port", fmt.Sprintf("%d", rds.Port)),
		common.KV("Is Public", fmt.Sprintf("%t", rds.IsPubliclyAccessible)),
		"",
		sectionHeaderStyle.Render("Network"),
		common.KV("VPC", rds.VpcID),
		common.KV("Subnets", ""),
		"",
		sectionHeaderStyle.Render("Metrics"),
		common.KV("CPU Utilization", "Noop"),
		common.KV("Free Storage", "Noop"),
		common.KV("Connections", "Noop"),
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
