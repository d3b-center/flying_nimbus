package views

import (
	"context"
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/providers/aws/backend"
	c "flying_nimbus/internal/providers/aws/views/components"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rmhubbert/bubbletea-overlay"
)

type (
	ec2InstancesLoadedMsg   []list.Item
	SsmSessionFinishedMsg   struct{ err error }
	instanceActionStatusMsg struct {
		Err error
	}
	InstanceState string
)

const (
	StateRunning    InstanceState = "running"
	StateStopped    InstanceState = "stopped"
	LocalPortLabel  string        = "Local Port"
	RemotePortLabel string        = "Remote Port"
)

// Ec2ViewModel manages the EC2 instance list and details view.
type Ec2ViewModel struct {
	app                     *app.App
	list                    list.Model
	loader                  spinner.Model
	isLoading               bool
	instanceDetail          string
	isDetailViewportFocused bool
	detailViewport          viewport.Model
	windowSize              common.ContentWindowSizeMsg
	instanceListWidth       int
	detailsWidth            int
	contentHeight           int
	inputRoutingStrategy    common.InputRoutingStrategy
	actionMenu              c.ActionMenu
	isActionMenuActive      bool
	inputForm               c.InputForm
	isInputFormActive       bool
}

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
		app:                     appService,
		list:                    l,
		loader:                  loader,
		isLoading:               true,
		isDetailViewportFocused: false,
		detailViewport:          vp,
		windowSize:              windowSize,
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

func startInstanceCmd(ctx context.Context, ec2Service *aws.Ec2Service, instanceId string) tea.Cmd {
	return func() tea.Msg {
		err := ec2Service.StartInstance(ctx, instanceId)
		return instanceActionStatusMsg{Err: err}
	}
}

func stopInstanceCmd(ctx context.Context, ec2Service *aws.Ec2Service, instanceId string) tea.Cmd {
	return func() tea.Msg {
		err := ec2Service.StopInstance(ctx, instanceId)
		return instanceActionStatusMsg{Err: err}
	}
}

func (m Ec2ViewModel) InputRoutingStrategy() common.InputRoutingStrategy {
	return m.inputRoutingStrategy
}

func (m Ec2ViewModel) Commands() common.Commands {
	return []key.Binding{c.ToggleFocus, c.ForceRefresh}
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

	if m.isDetailViewportFocused {
		detailStyle = detailStyle.BorderForeground(c.FocusedColor)
		listStyle = listStyle.BorderForeground(c.UnfocusedColor)
	} else {
		listStyle = listStyle.BorderForeground(c.FocusedColor)
		detailStyle = detailStyle.BorderForeground(c.UnfocusedColor)
	}

	m.detailViewport.Style = detailStyle

	left := listStyle.Render(m.list.View())
	right := m.detailViewport.View()
	instances := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return m.handleOverlays(instances)
}

// Handle any modals over the instance list
func (m Ec2ViewModel) handleOverlays(instances string) string {
	if m.isActionMenuActive {
		return overlay.Composite(
			m.actionMenu.View(),
			instances,
			overlay.Center,
			overlay.Center,
			0,
			0,
		)
	}

	if m.isInputFormActive {
		return overlay.Composite(
			m.inputForm.View(),
			instances,
			overlay.Center,
			overlay.Center,
			0,
			0,
		)
	}

	return instances
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

	case c.ModalCancelMsg:
		m.isActionMenuActive = false
		return m, nil

	case c.ModalResponseMsg:
		m.isActionMenuActive = false
		if msg.Err != nil {
			slog.Error("Error with modal action", "error", msg.Err)
		}
		return m, nil

	case c.InputFormOpenMsg:
		m.isInputFormActive = true
		m.isActionMenuActive = false
		m.openPortForwardInputForm()
		return m, nil

	case c.InputFormSubmitMsg:
		m.isInputFormActive = false
		return m, msg.OnSubmit(msg.Values)

	case c.InputFormCancelMsg:
		m.isInputFormActive = false
		m.isActionMenuActive = true
		return m, nil

	case instanceActionStatusMsg:
		m.isActionMenuActive = false
		m.isLoading = true
		return m, fetchEc2InstancesCmd(m.app.Context, m.app.AWS.Ec2)

	case tea.KeyMsg:
		if m.list.FilterState() != list.Filtering && key.Matches(msg, c.ForceRefresh) {
			m.isLoading = true
			cmd := fetchEc2InstancesCmd(m.app.Context, m.app.AWS.Ec2)
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

	m.updateInputRouting()

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
	rows = append(rows, c.GenerateEbsVolumeRows(instance.VolumeIds)...)

	rows = append(rows, "", common.SectionHeaderStyle.Render("Tags"))
	rows = append(rows, c.GenerateTagRows(instance.Tags)...)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// updateLayout recalculates and applies layout dimensions.
func (m *Ec2ViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg

	usableWidth := msg.Width - c.BorderWidth
	usableHeight := msg.Height - c.BorderHeight

	m.instanceListWidth = int(float64(usableWidth) * c.InstanceListWidthRatio)
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

func (m *Ec2ViewModel) updateInputRouting() {
	m.inputRoutingStrategy = common.RouteGlobalFirst

	filterState := m.list.FilterState()
	if filterState == list.Filtering || m.isActionMenuActive || m.isInputFormActive {
		m.inputRoutingStrategy = common.RouteFocusedFirst
	}
}

// handleKeypress processes keyboard input.
func (m *Ec2ViewModel) handleKeypress(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	if m.isInputFormActive {
		m.inputForm, cmd = m.inputForm.Update(msg)
		return cmd
	}

	if m.isActionMenuActive {
		m.actionMenu, cmd = m.actionMenu.Update(msg)
		return cmd
	}

	if key.Matches(msg, constants.Keymap.Enter) {
		m.isActionMenuActive = true
		// Must generate actions on modal popup to get latest model since
		// Init, Update, and View cannot take pointer to model
		m.buildActions()
		return nil
	}

	if key.Matches(msg, c.ForceRefresh) {
		m.isLoading = true
		return fetchEc2InstancesCmd(m.app.Context, m.app.AWS.Ec2)
	}

	if key.Matches(msg, c.ToggleFocus) {
		m.isDetailViewportFocused = !m.isDetailViewportFocused
		return nil
	}

	if m.isDetailViewportFocused {
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return cmd
	} else {
		m.list, cmd = m.list.Update(msg)
		return cmd
	}
}

func (m *Ec2ViewModel) buildActions() {
	m.actionMenu = c.NewActionModal("EC2 Actions", []c.ActionItem{
		{Label: "Shell", Action: m.ssmShell},
		{Label: "Port Forward", Action: m.ssmPortForward},
		{Label: "Start/Stop", Action: m.handleStartStop},
	})
}

func (m Ec2ViewModel) validateSsmInstance() (aws.Ec2Instance, error) {
	var err error
	errHeader := "Failed to open SSM Shell"

	if m.list.SelectedItem() == nil {
		err = fmt.Errorf("%s: Selected item is nil", errHeader)
	}

	selectedItem := m.list.SelectedItem()
	instance, ok := selectedItem.(aws.Ec2Instance)
	if !ok {
		err = fmt.Errorf("%s: Selected item is not instance", errHeader)
	}

	if instance.State != "running" {
		err = fmt.Errorf("%s: Selected instance is not running", errHeader)
	}

	return instance, err
}

func (m Ec2ViewModel) ssmShell() tea.Cmd {
	instance, err := m.validateSsmInstance()

	if err != nil {
		return func() tea.Msg {
			return c.ModalResponseMsg{Err: err}
		}
	}

	command := m.app.AWS.Ssm.BuildSessionCmd(instance.InstanceID)
	return tea.ExecProcess(command, func(err error) tea.Msg {
		return c.ModalResponseMsg{Err: err}
	})

}

// openPortForwardInputForm sets the input form for SSM port forward from the selected instance.
// Called from Update on InputFormOpenMsg because the ActionMenu callback does not return the model.
func (m *Ec2ViewModel) openPortForwardInputForm() {
	instance := m.list.SelectedItem().(aws.Ec2Instance)
	m.inputForm = c.NewInputForm(
		fmt.Sprintf("Port Forward: %s", instance.Name),
		m.ssmPortForwardInputs(),
		m.ssmPortForwardOnSubmit,
	)
}

// ActionMenu callback
func (m *Ec2ViewModel) ssmPortForward() tea.Cmd {
	_, err := m.validateSsmInstance()
	if err != nil {
		return func() tea.Msg {
			return c.ModalResponseMsg{Err: err}
		}
	}

	m.isInputFormActive = true
	m.isActionMenuActive = false

	return func() tea.Msg {
		return c.InputFormOpenMsg{}
	}
}

func (m Ec2ViewModel) ssmPortForwardInputs() []c.InputField {
	return []c.InputField{
		{Label: LocalPortLabel, Placeholder: "8080", CharLimit: 5, Validator: aws.ValidatePort},
		{Label: RemotePortLabel, Placeholder: "8080", CharLimit: 5, Validator: aws.ValidatePort},
	}
}

// InputForm callback
func (m Ec2ViewModel) ssmPortForwardOnSubmit(values c.InputFormResult) tea.Cmd {
	instance, err := m.validateSsmInstance()
	if err != nil {
		return func() tea.Msg {
			return c.ModalResponseMsg{Err: err}
		}
	}

	localPort, _ := strconv.Atoi(values[LocalPortLabel])
	remotePort, _ := strconv.Atoi(values[RemotePortLabel])

	config := aws.PortForwardConfig{
		LocalPort:  localPort,
		RemotePort: remotePort,
	}

	cmd := m.app.AWS.Ssm.BuildPortForwardCmd(instance.InstanceID, config)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return SsmSessionFinishedMsg{err}
	})
}

func (m *Ec2ViewModel) handleStartStop() tea.Cmd {
	if m.isLoading {
		return nil
	}

	instance, ok := m.list.SelectedItem().(aws.Ec2Instance)
	if !ok {
		return nil
	}

	if instance.State == string(StateRunning) {
		return stopInstanceCmd(m.app.Context, m.app.AWS.Ec2, instance.InstanceID)
	} else if instance.State == string(StateStopped) {
		return startInstanceCmd(m.app.Context, m.app.AWS.Ec2, instance.InstanceID)
	}
	return nil
}
