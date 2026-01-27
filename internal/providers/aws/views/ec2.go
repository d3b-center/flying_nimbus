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


type Ec2ViewModel struct {
	app *app.App
	list list.Model
	loader spinner.Model
	isLoading bool
	windowSize common.ContentWindowSizeMsg
	instanceListWidth int
	detailsWidth int
}

type (
	ec2InstancesLoadedMsg []list.Item
)

func InitEc2ViewModel(appService *app.App) Ec2ViewModel {
	slog.Debug("Initialize custom Ec2 view model")
	items := []list.Item{}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Instances"
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	loader := spinner.New()
	loader.Style = spinnerStyle
	loader.Spinner = spinner.Dot

	return Ec2ViewModel{
		app: appService,
		list: l,
	}
}

func fetchEc2InstancesCmd(ctx context.Context, ec2Service *aws.Ec2Service) tea.Cmd {
	return func() tea.Msg {
		instances, _ := ec2Service.ListInstances(ctx)
		return ec2InstancesLoadedMsg(ec2InstancesToItems(instances))
	}
}

func (m Ec2ViewModel) Init() tea.Cmd {
	slog.Debug("Initialize Ec2 BubbleTea Model")
	return tea.Batch(m.loader.Tick, fetchEc2InstancesCmd(m.app.Context, m.app.AWS.Ec2))
}

func (m Ec2ViewModel) View() string {
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
	right := instanceDetailStyle.Width(detailsWidth).Height(contentHeight).Render(generateEc2InstanceDetail(m.list.SelectedItem()))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Ec2ViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		slog.Debug(fmt.Sprintf("Received WindowSizeMsg %v", msg))

		m.windowSize = msg
		m.instanceListWidth = msg.Width / 3
		m.detailsWidth = msg.Width - m.instanceListWidth

		m.list.SetSize(m.instanceListWidth, msg.Height)
	case ec2InstancesLoadedMsg:
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

func ec2InstancesToItems(instances []aws.Ec2Instance) []list.Item {
	items := make([]list.Item, len(instances))
	for i, instance := range instances {
		items[i] = list.Item(instance)
	}
	return items
}

func generateEc2InstanceDetail(selectedItem list.Item) string {
	if selectedItem == nil {
		return "No Info"
	}

	instance, ok := selectedItem.(aws.Ec2Instance)
	if !ok {
		return "No Info"
	}

		rows := []string{
		headerStyle.Render("Instance Details"),
		"",
		sectionHeaderStyle.Render("General Info"),
		common.KV("Instance ID", instance.InstanceID),
		common.KV("Name", instance.Name),
		common.KV("Instance Type", instance.InstanceType),
		common.KV("State", instance.State),
		common.KV("IAM Profile", instance.IamInstanceProfile),
		common.KV("Launch Time", instance.LaunchTime),
		"",
		sectionHeaderStyle.Render("Network"),
		common.KV("Private IP", instance.PrivateIP),
		common.KV("Public IP", instance.PublicIP),
		common.KV("VPC", instance.VpcID),
		common.KV("Subnet", instance.SubnetID),
		
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}