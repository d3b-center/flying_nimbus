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
)

const instanceListWidthRatio = 0.25

// RdsViewModel is the Bubble Tea model for displaying RDS instances and their details.
type RdsViewModel struct {
	app               *app.App
	list              list.Model
	loader            spinner.Model
	isLoading         bool
	windowSize        common.ContentWindowSizeMsg
	instanceListWidth int
	detailsWidth      int
	sgs               map[string]*aws.SecurityGroup
}

// Messages returned from async commands.
type (
	rdsInstancesLoadedMsg   []list.Item
	securityGroupsLoadedMsg map[string]*aws.SecurityGroup
)

func InitRdsViewModel(appService *app.App) RdsViewModel {
	items := []list.Item{}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Instances"
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	windowSize := common.ContentWindowSizeMsg{
		Height: constants.WindowSize.Height,
		Width:  constants.WindowSize.Width,
	}

	loader := spinner.New()
	loader.Style = spinnerStyle
	loader.Spinner = spinner.Dot

	m := RdsViewModel{
		app:        appService,
		loader:     loader,
		list:       l,
		isLoading:  true,
		windowSize: windowSize,
	}
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

	left := instancesListStyle.Width(m.instanceListWidth).Height(m.windowSize.Height).Render(m.list.View())
	right := instanceDetailStyle.Width(m.detailsWidth).Height(m.windowSize.Height).Render(generateInstanceDetail(m.list.SelectedItem(), m.sgs))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m RdsViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		m.windowSize = msg
		m.updateLayout(msg)

	case rdsInstancesLoadedMsg:
		m.list.SetItems(msg)
		slog.Debug(fmt.Sprintf("📥 Rcv'd Rds Instances... # of Rds' %d", len(m.list.Items())))
		cmd = fetchSecurityGroupsCmd(m.app.Context, m.app.AWS.Sg, gatherSecurityGroupIds(msg))

	case securityGroupsLoadedMsg:
		slog.Debug(fmt.Sprintf("📥 Rcv'd SG Details' %d", len(msg)))
		m.isLoading = false
		m.sgs = msg

	case spinner.TickMsg:
		m.loader, cmd = m.loader.Update(msg)
	default:
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

// updateLayout recalculates pane widths based on the current window size.
func (m *RdsViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg

	m.instanceListWidth = int(float64(msg.Width) * instanceListWidthRatio)
	m.detailsWidth = msg.Width - m.instanceListWidth

	m.list.SetSize(m.instanceListWidth, msg.Height)
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

// generateInstanceDetail generates the detail panel for a selected RDS instance.
func generateInstanceDetail(selectedItem list.Item, sgs map[string]*aws.SecurityGroup) string {
	// Skeleton detail sections
	if selectedItem == nil {
		return "No Info"
	}

	rds, ok := selectedItem.(aws.RDSInstance)
	if !ok {
		return "No Info"
	}

	var (
		headerStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1)
		sectionHeaderStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("62")).
					PaddingBottom(1)
	)

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
		"",
	}

	rows = append(rows, subnetSection(rds.SubnetIds, sectionHeaderStyle)...)

	rows = append(rows, "")

	rows = append(rows, securityGroupRulesSection(rds.SecurityGroupIds, sgs, sectionHeaderStyle)...)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
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
