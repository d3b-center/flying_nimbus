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
	instanceListWidth    int
	detailsWidth         int
	contentHeight        int
	inputRoutingStrategy common.InputRoutingStrategy
}

type (
	ec2InstancesLoadedMsg []list.Item
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

	vp := viewport.New(windowSize.Width, windowSize.Height)

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
	return []key.Binding{toggleFocus}
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
		MaxHeight(m.windowSize.Height)

	detailStyle := common.InstanceDetailStyle.
		MaxHeight(m.windowSize.Height)

	if m.detailsFocused {
		detailStyle = detailStyle.BorderForeground(focusedColor)
		listStyle = listStyle.BorderForeground(unfocusedColor)
	} else {
		listStyle = listStyle.BorderForeground(focusedColor)
		detailStyle = detailStyle.BorderForeground(unfocusedColor)
	}

	m.detailViewport.Style = detailStyle

	left := listStyle.Render(m.list.View())
	right := m.detailViewport.View()

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// Update handles messages and updates the model state.
func (m Ec2ViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevIndex := m.list.Index()
	var cmds []tea.Cmd

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
		newLoader, cmd := m.loader.Update(msg)
		m.loader = newLoader
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		if key.Matches(msg, forceRefresh) {
			m.isLoading = true
			cmd := fetchRdsInstancesCmd(m.app.Context, m.app.AWS.Rds)
			cmds = append(cmds, cmd)
		}
		cmd := m.handleKeypress(msg)
		cmds = append(cmds, cmd)
	default:
		newList, cmd := m.list.Update(msg)
		m.list = newList
		cmds = append(cmds, cmd)
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
	return m, tea.Batch(cmds...)
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

// updateLayout recalculates and applies layout dimensions.
func (m *Ec2ViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg

	usableWidth := msg.Width - BorderWidth
	usableHeight := msg.Height - BorderHeight

	m.instanceListWidth = int(float64(usableWidth) * ec2InstanceListWidthRatio)
	m.detailsWidth = usableWidth - m.instanceListWidth

	m.contentHeight = usableHeight

	m.detailViewport.Width = m.detailsWidth
	m.detailViewport.Height = msg.Height

	m.list.SetSize(m.instanceListWidth, usableHeight)
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
	if key.Matches(msg, toggleFocus) {
		m.detailsFocused = !m.detailsFocused
		return nil
	}

	if m.detailsFocused {
		m.detailViewport, cmd = m.detailViewport.Update(msg)
	} else {
		m.list, cmd = m.list.Update(msg)
	}
	return cmd
}
