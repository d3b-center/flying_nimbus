package views

import (
	"context"
	"flying_nimbus/internal/app"
	aws "flying_nimbus/internal/providers/aws/backend"
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
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
	"github.com/rmhubbert/bubbletea-overlay"
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

// RdsViewModel is the Bubble Tea model for displaying RDS instances and their details.
type RdsViewModel struct {
	app                  *app.App
	list                 list.Model
	loader               spinner.Model
	isLoading            bool
	windowSize           common.ContentWindowSizeMsg
	instanceListWidth    int
	detailsWidth         int
	sgs                  map[string]*aws.SecurityGroup
	inputRoutingStrategy common.InputRoutingStrategy
	viewport             viewport.Model
	isViewportFocused    bool
	actionMenu           c.ActionMenu
	isActionMenuActive   bool
	inputForm            c.InputForm
	isInputFormActive    bool
}

func (m RdsViewModel) InputRoutingStrategy() common.InputRoutingStrategy {
	return m.inputRoutingStrategy
}

func (m RdsViewModel) Title() string {
	return "RDS Management"
}

func (m RdsViewModel) Commands() common.Commands {
	return []key.Binding{c.ForceRefresh, c.ToggleFocus}
}

// Messages returned from async commands.
type (
	rdsInstancesLoadedMsg   []list.Item
	securityGroupsLoadedMsg map[string]*aws.SecurityGroup
)

func InitRdsViewModel(appService *app.App, windowSize common.ContentWindowSizeMsg) RdsViewModel {
	items := []list.Item{}

	l := list.New(items, list.NewDefaultDelegate(), windowSize.Height, 0)
	l.Title = "Instances"
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	loader := spinner.New()
	loader.Style = common.SpinnerStyle
	loader.Spinner = spinner.Dot

	v := viewport.New(windowSize.Width, windowSize.Height)

	m := RdsViewModel{
		app:                  appService,
		loader:               loader,
		list:                 l,
		isLoading:            true,
		inputRoutingStrategy: common.RouteGlobalFirst,
		viewport:             v,
		isViewportFocused:    false,
	}
	slog.Debug(fmt.Sprintf("Window Size Init %v", windowSize))
	m.updateLayout(windowSize)

	return m
}

// fetchRdsInstancesCmd returns a command that fetches all RDS instances asynchronously.
func fetchRdsInstancesCmd(ctx context.Context, rdsService *aws.RdsService) tea.Cmd {
	return func() tea.Msg {
		dbs, _ := rdsService.ListDBInstances(ctx)
		return rdsInstancesLoadedMsg(dbInstancesToItems(dbs))
	}
}

// fetchSecurityGroupsCmd returns a command that fetches the details of specified security groups.
func fetchSecurityGroupsCmd(ctx context.Context, sgService *aws.SgService, sgIds []string) tea.Cmd {
	slog.Debug(fmt.Sprintf("📤 Sending cmd fetchSecurityGroups SGIds #%d", len(sgIds)))
	return func() tea.Msg {
		sgs, _ := sgService.DescribeSecurityGroupRules(ctx, sgIds)
		return securityGroupsLoadedMsg(sgs)
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

	if m.isViewportFocused {
		instanceDetailStyle = instanceDetailStyle.BorderForeground(c.FocusedColor)
		instancesListStyle = instancesListStyle.BorderForeground(c.UnfocusedColor)
	} else {
		instanceDetailStyle = instanceDetailStyle.BorderForeground(c.UnfocusedColor)
		instancesListStyle = instancesListStyle.BorderForeground(c.FocusedColor)
	}

	instanceDetailStyle.MaxHeight(m.windowSize.Height)

	left := instancesListStyle.MaxHeight(m.windowSize.Height).Render(m.list.View())

	m.viewport.Style = instanceDetailStyle
	right := m.viewport.View()
	instances := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return m.handleOverlays(instances)
}

// Handle any modals over the instance list
func (m RdsViewModel) handleOverlays(instances string) string {
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

func (m RdsViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	prevIndex := m.list.Index()

	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		m.windowSize = msg
		m.updateLayout(msg)

	case rdsInstancesLoadedMsg:
		m.list.SetItems(msg)
		slog.Debug(fmt.Sprintf("📥 Rcv'd Rds Instances... # of Rds' %d", len(m.list.Items())))
		cmd := fetchSecurityGroupsCmd(m.app.Context, m.app.AWS.Sg, gatherSecurityGroupIds(msg))
		cmds = append(cmds, cmd)

	case securityGroupsLoadedMsg:
		slog.Debug(fmt.Sprintf("📥 Rcv'd SG Details' %d", len(msg)))
		m.isLoading = false
		m.sgs = msg
		m.updateInstanceDetails()

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

		// Must be in Update function since ActionMenu callback doesn't return model
		instance := m.list.SelectedItem().(aws.RDSInstance)
		m.inputForm = c.NewInputForm(
			fmt.Sprintf("Port Forward: %s", instance.Id),
			m.portForwardInputs(),
			m.portForwardOnSubmit,
		)
		return m, nil

	case c.InputFormSubmitMsg:
		m.isInputFormActive = false
		return m, msg.OnSubmit(msg.Values)

	case c.InputFormCancelMsg:
		m.isInputFormActive = false
		m.isActionMenuActive = true
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() != list.Filtering && key.Matches(msg, c.ForceRefresh) {
			m.isLoading = true
			cmd := fetchRdsInstancesCmd(m.app.Context, m.app.AWS.Rds)
			cmds = append(cmds, cmd)
		}
		cmd := m.handleKeyMsg(msg)
		cmds = append(cmds, cmd)

	case spinner.TickMsg:
		newLoader, cmd := m.loader.Update(msg)
		m.loader = newLoader
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

func (m *RdsViewModel) updateInputRouting() {
	m.inputRoutingStrategy = common.RouteGlobalFirst

	filterState := m.list.FilterState()
	if filterState == list.Filtering || m.isActionMenuActive || m.isInputFormActive {
		m.inputRoutingStrategy = common.RouteFocusedFirst
	}
}

func (m *RdsViewModel) updateInstanceDetails() {

	if m.isLoading {
		return
	}
	m.viewport.SetContent(generateRdsInstanceDetail(m.detailsWidth, m.list.SelectedItem(), m.sgs))
	m.viewport.GotoTop()
}

func (m *RdsViewModel) handleKeyMsg(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	if m.isInputFormActive {
		m.inputForm, cmd = m.inputForm.Update(msg)
		return cmd
	}

	if m.isActionMenuActive {
		m.actionMenu, cmd = m.actionMenu.Update(msg)
		return cmd
	}

	if key.Matches(msg, c.ToggleFocus) {
		m.isViewportFocused = !m.isViewportFocused
		return nil
	}

	if key.Matches(msg, constants.Keymap.Enter) {
		m.isActionMenuActive = true
		m.buildActions()
		return nil
	}

	if m.isViewportFocused {
		m.viewport, cmd = m.viewport.Update(msg)
	} else {
		m.list, cmd = m.list.Update(msg)
	}

	return cmd
}

// updateLayout recalculates pane widths based on the current window size.
func (m *RdsViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg

	usableHeight := m.windowSize.Height - c.BorderHeight
	usableWidth := m.windowSize.Width - c.BorderWidth

	m.instanceListWidth = int(float64(usableWidth) * c.InstanceListWidthRatio)
	m.detailsWidth = usableWidth - m.instanceListWidth

	m.list.SetSize(m.instanceListWidth, usableHeight)

	m.viewport.Height = m.windowSize.Height
	m.viewport.Width = m.detailsWidth
}

func (m *RdsViewModel) buildActions() {
	m.actionMenu = c.NewActionModal("RDS Actions", []c.ActionItem{
		{Label: "Port Forward", Action: m.portForward},
	})
}

func (m RdsViewModel) portForward() tea.Cmd {
	selectedItem := m.list.SelectedItem()
	rdsInstance, _ := selectedItem.(aws.RDSInstance)
	if rdsInstance.Status != "available" {
		return nil
	}

	m.isInputFormActive = true
	m.isActionMenuActive = false

	return func() tea.Msg {
		return c.InputFormOpenMsg{}
	}
}

func (m RdsViewModel) portForwardOnSubmit(values c.InputFormResult) tea.Cmd {
	rdsHostname, bastionId, err := m.getPortForwardInstanceInfo()
	if err != nil {
		return func() tea.Msg {
			return c.ModalResponseMsg{err}
		}
	}

	localPort, _ := strconv.Atoi(values[LocalPortLabel])
	remotePort, _ := strconv.Atoi(values[RemotePortLabel])

	config := aws.PortForwardConfig{
		LocalPort:  localPort,
		RemotePort: remotePort,
		RemoteHost: rdsHostname,
	}

	slog.Info("Starting port forwarding session", "hostname", rdsHostname, "bastion ID", bastionId)
	cmd := m.app.AWS.Ssm.BuildRemotePortForwardCmd(bastionId, config)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return SsmSessionFinishedMsg{err}
	})
}

// Return the RDS instance's hostname, the bastion EC2 instance's ID, and any errors
func (m RdsViewModel) getPortForwardInstanceInfo() (string, string, error) {
	selectedItem := m.list.SelectedItem()
	rdsInstance, ok := selectedItem.(aws.RDSInstance)
	if !ok {
		return "", "", fmt.Errorf("SSM Error: Selected item is not instance")
	}

	bastion, err := m.app.AWS.Ec2.FindBastionHost(m.app.Context, rdsInstance.VpcID)
	if err != nil {
		return "", "", fmt.Errorf("Error getting bastion host: %w", err)
	}

	return rdsInstance.Endpoint, bastion.InstanceID, nil
}

func (m RdsViewModel) portForwardInputs() []c.InputField {
	return []c.InputField{
		{Label: LocalPortLabel, Placeholder: "10001", CharLimit: 5, Validator: aws.ValidatePort},
		{Label: RemotePortLabel, Placeholder: "5432", CharLimit: 5, Validator: aws.ValidatePort},
	}
}

// dbInstancesToItems converts a slice of RDS instances to list.Items.
func dbInstancesToItems(dbs []aws.RDSInstance) []list.Item {
	items := make([]list.Item, len(dbs))
	for i, db := range dbs {
		items[i] = list.Item(db)
	}
	return items
}

// gatherSecurityGroupIds extracts unique SG IDs from a list of RDS instances.
func gatherSecurityGroupIds(items []list.Item) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)

	for _, rawItem := range items {
		rds, ok := rawItem.(aws.RDSInstance)
		if !ok {
			slog.Error("failed to convert to aws.RDSInstance")
			continue
		}

		for _, sgID := range rds.SecurityGroupIds {
			if _, exists := seen[sgID]; exists {
				continue
			}
			seen[sgID] = struct{}{}
			out = append(out, sgID)
		}
	}

	return out
}

// generateRdsInstanceDetail generates the detail panel for a selected RDS instance.
func generateRdsInstanceDetail(width int, selectedItem list.Item, sgs map[string]*aws.SecurityGroup) string {
	// Skeleton detail sections
	if selectedItem == nil {
		return "No Info"
	}

	rds, ok := selectedItem.(aws.RDSInstance)
	if !ok {
		return "No Info"
	}

	rows := []string{
		common.HeaderStyle.Render("Instance Details"),
		"",
		common.SectionHeaderStyle.Render("General Info"),
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
		common.SectionHeaderStyle.Render("Network"),
		common.KV("VPC", rds.VpcID),
		"",
	}

	rows = append(rows, subnetSection(rds.SubnetIds, common.SectionHeaderStyle)...)

	rows = append(rows, "")

	rows = append(rows, securityGroupRulesSection(rds.SecurityGroupIds, sgs, common.SectionHeaderStyle)...)

	return wrap.String(wordwrap.String(lipgloss.JoinVertical(lipgloss.Left, rows...), width), width)
}

func formatSubnetIds(subnetIds []string) []string {
	rows := make([]string, 0, len(subnetIds))
	for _, id := range subnetIds {
		rows = append(rows, fmt.Sprintf("  ‣ %s", id))
	}
	return rows
}

func subnetSection(
	subnetIds []string,
	headerStyle lipgloss.Style,
) []string {

	return common.RenderSection(
		"Subnets",
		headerStyle,
		func() []string {
			return formatSubnetIds(subnetIds)
		},
	)
}

// securityGroupRulesSection generates the Security Group Rules section for the details panel.
func securityGroupRulesSection(
	sgIds []string,
	securityGroups map[string]*aws.SecurityGroup,
	headerStyle lipgloss.Style,
) []string {

	return common.RenderSection(
		"Security Group Rules",
		headerStyle,
		func() []string {
			rules := flattenSecurityGroupRules(sgIds, securityGroups)

			if len(rules) == 0 {
				return nil
			}

			rows := make([]string, 0, len(rules))
			for _, rule := range rules {
				rows = append(rows, formatSecurityGroupRule(rule))
			}
			return rows
		},
	)
}

// formatSecurityGroupRule returns a formatted string representation of a security group rule.
func formatSecurityGroupRule(r aws.SecurityGroupRule) string {
	trafficDirection := "INGRESS"
	if r.IsEgress {
		trafficDirection = "EGRESS"
	}

	port := "all"
	if r.FromPort != 0 || r.ToPort != 0 {
		if r.FromPort == r.ToPort {
			port = fmt.Sprintf("%d", r.FromPort)
		} else {
			port = fmt.Sprintf("%d-%d", r.FromPort, r.ToPort)
		}
	}

	target := r.CidrIpv4
	if target == "" && r.ReferencedSGId != "" {
		target = "" + r.ReferencedSGId
	}
	if target == "" {
		target = "-"
	}

	desc := r.Description
	if desc == "" {
		desc = "-"
	}

	ipProtocol := "all"
	if r.IpProtocol != "-1" {
		ipProtocol = r.IpProtocol
	}

	return fmt.Sprintf(
		"  ‣ %-7s %-5s %-8s → %-18s %s",
		trafficDirection,
		ipProtocol,
		port,
		target,
		desc,
	)
}

// flattenSecurityGroupRules returns all rules for the given SG IDs.
func flattenSecurityGroupRules(
	sgIds []string,
	securityGroups map[string]*aws.SecurityGroup,
) []aws.SecurityGroupRule {

	var out []aws.SecurityGroupRule

	for _, sgID := range sgIds {
		sg, ok := securityGroups[sgID]
		if !ok {
			continue
		}
		out = append(out, sg.Rules...)
	}

	return out
}
