package views

import (
	"context"
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/providers/aws/views/components"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/bubbles/viewport"
)


type Ec2ViewModel struct {
	app *app.App
	list list.Model
	loader spinner.Model
	isLoading bool
	instanceDetail string
	detailsFocused bool
	detailViewport viewport.Model
	windowSize common.ContentWindowSizeMsg
	ready bool
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
	loader.Spinner = spinner.Points

	vp := viewport.New(0,0)

	return Ec2ViewModel{
		app: appService,
		list: l,
		loader: loader,
		isLoading: true,
		detailsFocused: false,
		detailViewport: vp,
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
	m.windowSize = common.ContentWindowSizeMsg{Width: w, Height: h}
	contentWidth := constants.WindowSize.Width - w
	contentHeight := constants.WindowSize.Height - h

	instanceListWidth := contentWidth / 3
	detailsWidth := contentWidth - instanceListWidth

	m.list.SetSize(instanceListWidth, contentHeight)
	if !m.ready {
		m.detailViewport = viewport.New(detailsWidth, contentHeight)
		m.ready = true
	} else {
		m.detailViewport.Width = detailsWidth
		m.detailViewport.Height = contentHeight
	}
	m.detailViewport.SetContent(m.instanceDetail)

	listStyle := instancesListStyle.
		Width(instanceListWidth).
		Height(contentHeight)
		
	detailStyle := instanceDetailStyle.
		Width(detailsWidth).
		Height(contentHeight)

	if m.detailsFocused {
		detailStyle = detailStyle.BorderForeground(lipgloss.Color("62"))  // Bright color
		listStyle = listStyle.BorderForeground(lipgloss.Color("240"))     // Dim color
	} else {
		listStyle = listStyle.BorderForeground(lipgloss.Color("62"))      // Bright color
		detailStyle = detailStyle.BorderForeground(lipgloss.Color("240")) // Dim color
	}

	left := listStyle.Render(m.list.View())
	right := detailStyle.Render(m.detailViewport.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Ec2ViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	prevIndex := m.list.Index()

	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		slog.Debug(fmt.Sprintf("Received WindowSizeMsg %v", msg))

		m.windowSize = msg
		m.instanceListWidth = msg.Width / 3
		m.detailsWidth = msg.Width - m.instanceListWidth

		m.list.SetSize(m.instanceListWidth, msg.Height)

		// Initialize or resize viewport
		if !m.ready {
			m.detailViewport = viewport.New(m.detailsWidth, msg.Height)
			m.ready = true
		} else {
			m.detailViewport.Width = m.detailsWidth
			m.detailViewport.Height = msg.Height
		}

	case ec2InstancesLoadedMsg:
		m.isLoading = false
		m.list.SetItems(msg)
		slog.Debug(fmt.Sprintf("Size of list %d", len(m.list.Items())))

		if !m.ready {
			m.detailViewport = viewport.New(m.detailsWidth, m.windowSize.Height)
			m.ready = true
		} else {
			m.detailViewport.Width = m.detailsWidth
			m.detailViewport.Height = m.windowSize.Height
		}

		m.instanceDetail = generateEc2InstanceDetail(m.list.SelectedItem())
		m.detailViewport.SetContent(m.instanceDetail)
		
	case spinner.TickMsg:
		m.loader, cmd = m.loader.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		// Allow scrolling in detail viewport
		switch msg.String() {
		case "right":
			slog.Debug("Focus on details view")
			m.detailsFocused = true
		case "left":
			slog.Debug("Focus on instances list")
			m.detailsFocused = false
		case "down":
			if m.detailsFocused {
				m.detailViewport.ViewDown()
			} else {
				m.list, cmd = m.list.Update(msg)
			}
		case "up":
			if m.detailsFocused {
				m.detailViewport.ViewUp()
			} else {
				m.list, cmd = m.list.Update(msg)
			}
		}
	default:
		m.list, cmd = m.list.Update(msg)
	}

	if m.list.Index() != prevIndex {
		slog.Debug("Selection changed, regenerating detail", "index", m.list.Index())
		m.instanceDetail = generateEc2InstanceDetail(m.list.SelectedItem())
		m.detailViewport.SetContent(m.instanceDetail)
		m.detailViewport.GotoTop()
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
		"",
		sectionHeaderStyle.Render("Security Groups"),
	}

	// Add security groups
	if len(instance.SecurityGroups) > 0 {
		for _, sg := range instance.SecurityGroups {
			rows = append(rows, "  • "+sg.Id)
		}
	} else {
		rows = append(rows, "  None")
	}

	rows = append(rows, "", sectionHeaderStyle.Render("EBS Volumes"))
	rows = append(rows, components.GenerateEbsVolumeRows(instance.Volumes)...)
	
	rows = append(rows, "", sectionHeaderStyle.Render("Tags"))
	rows = append(rows, components.GenerateTagRows(instance.Tags)...)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m Ec2ViewModel) resizeViewport(width int, height int) {
	if !m.ready {
		m.detailViewport = viewport.New(width, height)
		m.ready = true
	} else {
		m.detailViewport.Width = width
		m.detailViewport.Height = height
	}

	m.detailViewport.SetContent(m.instanceDetail)
}