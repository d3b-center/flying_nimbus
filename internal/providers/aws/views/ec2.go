package views

import (
	"context"
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const ec2InstanceListWidthRatio = 0.25

// Ec2ViewModel manages the EC2 instance list and details view.
type Ec2ViewModel struct {
	app                  *app.App
	list                 list.Model
	loader               spinner.Model
	isLoading            bool
	instanceDetail       string
	detailsFocused       bool
	detailViewport       viewport.Model
	windowSize           common.ContentWindowSizeMsg
	ready                bool
	instanceListWidth    int
	detailsWidth         int
	contentHeight        int
	inputRoutingStrategy common.InputRoutingStrategy

}

type (
	ec2InstancesLoadedMsg []list.Item
)

var (
	focusedColor   = lipgloss.Color("62")
	unfocusedColor = lipgloss.Color("240")
)

// Creates a new EC2 view model
func InitEc2ViewModel(appService *app.App, windowSize common.ContentWindowSizeMsg) Ec2ViewModel {
	slog.Debug("Initialize custom Ec2 view model")
	items := []list.Item{}

	l := list.New(items, list.NewDefaultDelegate(), windowSize.Height, 0)
	l.Title = "Instances"
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	loader := spinner.New()
	loader.Style = common.SpinnerStyle
	loader.Spinner = spinner.Points

	vp := viewport.New(0, 0)

	m := Ec2ViewModel{
		app:            appService,
		list:           l,
		loader:         loader,
		isLoading:      true,
		detailsFocused: false,
		detailViewport: vp,
		windowSize:     windowSize,
	}
	m.updateLayout(windowSize)

	return m
}

// fetchEc2InstancesCmd returns a command that fetches EC2 instances.
func fetchEc2InstancesCmd(ctx context.Context, ec2Service *aws.Ec2Service) tea.Cmd {
	return func() tea.Msg {
		instances, _ := ec2Service.ListInstances(ctx)
		return ec2InstancesLoadedMsg(ec2InstancesToItems(instances))
	}
}

func (m Ec2ViewModel) InputRoutingStrategy() common.InputRoutingStrategy {
	return m.inputRoutingStrategy
}

func (m Ec2ViewModel) Commands() common.Commands {
	return make([]key.Binding, 0)
}

func (m Ec2ViewModel) Title() string {
	return "EC2 Management"
}

// Init initializes the EC2 view model.
func (m Ec2ViewModel) Init() tea.Cmd {
	slog.Debug("Initialize Ec2 BubbleTea Model")
	return tea.Batch(m.loader.Tick, fetchEc2InstancesCmd(m.app.Context, m.app.AWS.Ec2))
}

// View renders the EC2 view.
func (m Ec2ViewModel) View() string {
	if m.isLoading {
		return constants.DocStyle.Render(m.loader.View() + "\n")
	}

	listStyle := common.InstancesListStyle.
		MaxHeight(m.contentHeight)

	detailStyle := common.InstanceDetailStyle.
		MaxHeight(m.contentHeight)

	if m.detailsFocused {
		detailStyle = detailStyle.BorderForeground(focusedColor)
		listStyle = listStyle.BorderForeground(unfocusedColor)
	} else {
		listStyle = listStyle.BorderForeground(focusedColor)
		detailStyle = detailStyle.BorderForeground(unfocusedColor)
	}

	left := listStyle.Render(m.list.View())
	right := detailStyle.Render(m.detailViewport.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// Update handles messages and updates the model state.
func (m Ec2ViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	prevIndex := m.list.Index()

	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		slog.Debug(fmt.Sprintf("Received WindowSizeMsg %v", msg))
		m.updateLayout(msg)

	case ec2InstancesLoadedMsg:
		m.isLoading = false
		m.list.SetItems(msg)
		slog.Debug(fmt.Sprintf("Size of list %d", len(m.list.Items())))

		if len(msg) > 0 {
			m.updateInstanceDetails()
			m.updateLayout(m.windowSize)
		}

	case spinner.TickMsg:
		m.loader, cmd = m.loader.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		cmd = m.handleKeypress(msg)
	default:
		m.list, cmd = m.list.Update(msg)
	}

	if m.list.Index() != prevIndex {
		slog.Debug("Selection changed, regenerating detail", "index", m.list.Index())
		m.updateInstanceDetails()
	}

	filterState := m.list.FilterState()
	if filterState == list.Filtering {
		m.inputRoutingStrategy = common.RouteFocusedFirst
	} else {
		m.inputRoutingStrategy = common.RouteGlobalFirst
	}
	return m, cmd
}

// ec2InstancesToItems converts EC2 instances to list items.
func ec2InstancesToItems(instances []aws.Ec2Instance) []list.Item {
	items := make([]list.Item, len(instances))
	for i, instance := range instances {
		items[i] = list.Item(instance)
	}
	return items
}

// generateEc2InstanceDetail generates formatted details for an EC2 instance.
func generateEc2InstanceDetail(selectedItem list.Item) string {
	if selectedItem == nil {
		return "No Info"
	}

	instance, ok := selectedItem.(aws.Ec2Instance)
	if !ok {
		return "No Info"
	}

	rows := []string{
		common.HeaderStyle.Render("Instance Details"),
		"",
		common.SectionHeaderStyle.Render("General Info"),
		common.KV("Instance ID", instance.InstanceID),
		common.KV("Name", instance.Name),
		common.KV("Instance Type", instance.InstanceType),
		common.KV("State", instance.State),
		common.KV("IAM Profile", instance.IamInstanceProfile),
		common.KV("Launch Time", instance.LaunchTime),
		"",
		common.SectionHeaderStyle.Render("Network"),
		common.KV("Private IP", instance.PrivateIP),
		common.KV("Public IP", instance.PublicIP),
		common.KV("VPC", instance.VpcID),
		common.KV("Subnet", instance.SubnetID),
		"",
		common.SectionHeaderStyle.Render("Security Groups"),
	}

	// Add security groups
	if len(instance.SecurityGroupIds) > 0 {
		for _, id := range instance.SecurityGroupIds {
			rows = append(rows, "  • "+id)
		}
	} else {
		rows = append(rows, "  None")
	}

	rows = append(rows, "", common.SectionHeaderStyle.Render("EBS Volumes"))
	rows = append(rows, GenerateEbsVolumeRows(instance.Volumes)...)

	rows = append(rows, "", common.SectionHeaderStyle.Render("Tags"))
	rows = append(rows, GenerateTagRows(instance.Tags)...)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// resizeViewport updates the viewport dimensions.
func (m *Ec2ViewModel) resizeViewport(width int, height int) {
	if !m.ready {
		m.detailViewport = viewport.New(width, height)
		m.ready = true
	} else {
		m.detailViewport.Width = width
		m.detailViewport.Height = height
	}
}

// updateLayout recalculates and applies layout dimensions.
func (m *Ec2ViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg

	usableWidth := msg.Width - BorderWidth
	usableHeight := msg.Height - BorderHeight

	m.instanceListWidth = int(float64(usableWidth) * ec2InstanceListWidthRatio)
	m.detailsWidth = usableWidth - m.instanceListWidth

	m.contentHeight = usableHeight

	if !m.ready {
		m.detailViewport = viewport.New(m.instanceListWidth, m.contentHeight)
		m.ready = true
	} else {
		m.detailViewport.Width = m.detailsWidth
		m.detailViewport.Height = m.contentHeight
		m.resizeViewport(m.detailViewport.Width, m.detailViewport.Height)
	}

	m.list.SetSize(m.instanceListWidth, usableHeight)
	m.detailViewport.SetContent(m.instanceDetail)
}

// updateInstanceDetails regenerates and displays instance details.
func (m *Ec2ViewModel) updateInstanceDetails() {
	m.instanceDetail = generateEc2InstanceDetail(m.list.SelectedItem())
	m.detailViewport.SetContent(m.instanceDetail)
	m.detailViewport.GotoTop()
}

// handleKeypress processes keyboard input.
func (m *Ec2ViewModel) handleKeypress(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	switch msg.String() {
	case "right":
		slog.Debug("Focus on details view")
		m.detailsFocused = true
	case "left":
		slog.Debug("Focus on instances list")
		m.detailsFocused = false
	case "down":
		if m.detailsFocused {
			m.detailViewport.PageDown()
		} else {
			m.list, cmd = m.list.Update(msg)
		}
	case "up":
		if m.detailsFocused {
			m.detailViewport.PageUp()
		} else {
			m.list, cmd = m.list.Update(msg)
		}
	}
	case "enter":
		m.openActionMenu

	return cmd
}

func (m *Ec2ViewModel) openActionMenu() {
	
}
