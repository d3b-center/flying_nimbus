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

	headerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1)
)

type Ec2ViewModel struct {
	app *app.App
	list list.Model
	loader spinner.Model
	isLoading bool
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
	right := instanceDetailStyle.Width(detailsWidth).Height(contentHeight).Render(generateInstanceDetail(m.list.SelectedItem()))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Ec2ViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func generateInstanceDetail(selectedItem list.Item) string {
	if selectedItem == nil {
		return "No Info"
	}

	instance, ok := selectedItem.(aws.Ec2Instance)
	if !ok {
		return "No Info"
	}

	var (
		sectionHeaderStyle = lipgloss.NewStyle().
									Bold(true).
									Foreground(lipgloss.Color("62")).
									PaddingBottom(1)
		labelStyle = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#EE6FF8"))
	)

		rows := []string{
		headerStyle.Render("Instance Details"),
		"",
		sectionHeaderStyle.Render("General Info"),
		common.KV("Instance ID", instance.InstanceID, common.WithLabelStyle(labelStyle)),
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