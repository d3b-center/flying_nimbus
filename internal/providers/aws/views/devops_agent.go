package views

import (
	"context"
	"fmt"
	"strings"

	"flying_nimbus/internal/app"
	aws "flying_nimbus/internal/providers/aws/backend"
	c "flying_nimbus/internal/providers/aws/views/components"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	overlay "github.com/rmhubbert/bubbletea-overlay"
)

// ── tea messages ──────────────────────────────────────────────────────────────

type agentSpacesLoadedMsg struct {
	spaces []aws.AgentSpace
	err    error
}

type agentChatResponseMsg struct {
	result *aws.DevOpsAgentChatResult
	err    error
}

// ssmKind distinguishes whether an instance lookup is for a shell or tunnel.
type ssmKind int

const (
	ssmKindShell ssmKind = iota
	ssmKindTunnel
)

// ssmInstanceResolvedMsg is produced when an EC2 instance lookup completes.
type ssmInstanceResolvedMsg struct {
	instance aws.Ec2Instance
	kind     ssmKind
	err      error
}

// ssmRDSTunnelResolvedMsg is produced when an RDS tunnel target has been
// resolved (RDS instance + bastion found).
type ssmRDSTunnelResolvedMsg struct {
	rds     aws.RDSInstance
	bastion aws.Ec2Instance
	err     error
}

// ssmExitedMsg is produced when the SSM subprocess exits and the TUI resumes.
type ssmExitedMsg struct {
	description string
	err         error
}

// listResultMsg is produced by the #list* commands with a pre-formatted
// string ready to append to chat history.
type listResultMsg struct {
	content string
	err     error
}

// ── view states ───────────────────────────────────────────────────────────────

type devOpsAgentViewState int

const (
	devOpsStateLoadingSpaces devOpsAgentViewState = iota
	devOpsStateSelectSpace
	devOpsStateChat
)

const devOpsInputRows = 3

// ── key bindings ──────────────────────────────────────────────────────────────

var devOpsSendKey = key.NewBinding(
	key.WithKeys("enter"),
	key.WithHelp("enter", "send"),
)

// ── styles ────────────────────────────────────────────────────────────────────

var (
	devOpsUserLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("62"))

	devOpsAgentLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("170"))

	devOpsSystemLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("241"))

	devOpsSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	devOpsInputPromptStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("62"))

	devOpsErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	devOpsDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

// ── chat history entry ────────────────────────────────────────────────────────

type devOpsMessage struct {
	role    string // "user" | "agent" | "system"
	content string
}

// ── view model ────────────────────────────────────────────────────────────────

// DevOpsAgentViewModel is the TUI for the AWS DevOps Agent chat screen.
type DevOpsAgentViewModel struct {
	app    *app.App
	window common.ContentWindowSizeMsg
	state  devOpsAgentViewState

	// Space selection
	spaceList list.Model
	spaces    []aws.AgentSpace

	// Chat session
	selectedSpace         aws.AgentSpace
	executionId           string
	history               []devOpsMessage
	viewport              viewport.Model
	input                 textinput.Model
	spinner               spinner.Model
	isSending             bool

	// Port-forward form (reuses the same InputForm component as ec2.go)
	inputForm             c.InputForm
	isInputFormActive     bool
	pendingTunnelInstance aws.Ec2Instance // EC2 direct port-forward
	pendingRDSInstance    aws.RDSInstance // RDS remote port-forward target
	pendingBastion        aws.Ec2Instance // bastion for RDS tunnels

	err error
}

func InitDevOpsAgentViewModel(appService *app.App, windowSize common.ContentWindowSizeMsg) DevOpsAgentViewModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))

	ti := textinput.New()
	ti.Placeholder = "Ask a question or type #shell / #tunnel..."
	ti.CharLimit = 2048
	ti.Width = max(1, windowSize.Width-4)

	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, windowSize.Width, windowSize.Height)
	l.Title = "Select an agent space"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	vpHeight := max(1, windowSize.Height-devOpsInputRows)
	vp := viewport.New(windowSize.Width, vpHeight)

	return DevOpsAgentViewModel{
		app:       appService,
		window:    windowSize,
		state:     devOpsStateLoadingSpaces,
		spaceList: l,
		viewport:  vp,
		input:     ti,
		spinner:   sp,
	}
}

// ── NimbusModel interface ─────────────────────────────────────────────────────

func (m DevOpsAgentViewModel) Title() string { return "DevOps Agent" }

func (m DevOpsAgentViewModel) Commands() common.Commands {
	if m.state == devOpsStateChat {
		return []key.Binding{devOpsSendKey}
	}
	return []key.Binding{}
}

func (m DevOpsAgentViewModel) InputRoutingStrategy() common.InputRoutingStrategy {
	if m.state == devOpsStateChat {
		return common.RouteFocusedFirst
	}
	return common.RouteGlobalFirst
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m DevOpsAgentViewModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadSpacesCmd())
}

func (m DevOpsAgentViewModel) loadSpacesCmd() tea.Cmd {
	return func() tea.Msg {
		if m.app == nil || m.app.AWS == nil || m.app.AWS.DevOpsAgent == nil {
			return agentSpacesLoadedMsg{err: fmt.Errorf("DevOps Agent service unavailable")}
		}
		spaces, err := m.app.AWS.DevOpsAgent.ListAgentSpaces(m.app.Context)
		return agentSpacesLoadedMsg{spaces: spaces, err: err}
	}
}

func (m DevOpsAgentViewModel) sendToAgentCmd(space aws.AgentSpace, executionId, message string) tea.Cmd {
	return func() tea.Msg {
		if m.app == nil || m.app.AWS == nil || m.app.AWS.DevOpsAgent == nil {
			return agentChatResponseMsg{err: fmt.Errorf("DevOps Agent service unavailable")}
		}
		result, err := m.app.AWS.DevOpsAgent.Chat(
			m.app.Context, space.AgentSpaceId, executionId, message,
		)
		return agentChatResponseMsg{result: result, err: err}
	}
}

// resolveInstanceCmd looks up an EC2 instance by ID or name tag for #shell.
func resolveInstanceCmd(ctx context.Context, ec2Svc *aws.Ec2Service, search string, kind ssmKind) tea.Cmd {
	return func() tea.Msg {
		instances, err := ec2Svc.ListInstances(ctx)
		if err != nil {
			return ssmInstanceResolvedMsg{err: fmt.Errorf("list instances: %w", err), kind: kind}
		}
		inst, err := findDevOpsInstance(instances, search)
		return ssmInstanceResolvedMsg{instance: inst, kind: kind, err: err}
	}
}

// resolveTunnelTargetCmd is used by #tunnel: tries EC2 first, falls back to
// RDS. For RDS it also locates the bastion in the same VPC.
func resolveTunnelTargetCmd(ctx context.Context, ec2Svc *aws.Ec2Service, rdsSvc *aws.RdsService, search string) tea.Cmd {
	return func() tea.Msg {
		// Try EC2 first.
		ec2Instances, err := ec2Svc.ListInstances(ctx)
		if err == nil {
			if inst, err := findDevOpsInstance(ec2Instances, search); err == nil {
				return ssmInstanceResolvedMsg{instance: inst, kind: ssmKindTunnel}
			}
		}

		// Fall back to RDS.
		rdsInstances, err := rdsSvc.ListDBInstances(ctx)
		if err != nil {
			return ssmRDSTunnelResolvedMsg{err: fmt.Errorf("no EC2 or RDS instance matching %q", search)}
		}
		rds, err := findRDSBySearch(rdsInstances, search)
		if err != nil {
			return ssmRDSTunnelResolvedMsg{err: fmt.Errorf("no EC2 or RDS instance matching %q", search)}
		}

		// Locate a bastion in the same VPC.
		bastion, err := ec2Svc.FindBastionHost(ctx, rds.VpcID)
		if err != nil {
			return ssmRDSTunnelResolvedMsg{
				err: fmt.Errorf("RDS instance %q found but no bastion in VPC %s: %w", rds.Id, rds.VpcID, err),
			}
		}

		return ssmRDSTunnelResolvedMsg{rds: rds, bastion: bastion}
	}
}

// findRDSBySearch matches by exact ID, case-insensitive ID, or partial ID.
func findRDSBySearch(instances []aws.RDSInstance, search string) (aws.RDSInstance, error) {
	lower := strings.ToLower(search)
	for _, i := range instances {
		if i.Id == search {
			return i, nil
		}
	}
	for _, i := range instances {
		if strings.EqualFold(i.Id, search) {
			return i, nil
		}
	}
	for _, i := range instances {
		if strings.Contains(strings.ToLower(i.Id), lower) {
			return i, nil
		}
	}
	return aws.RDSInstance{}, fmt.Errorf("no RDS instance matching %q", search)
}

// findDevOpsInstance matches by exact ID, exact name, or partial name (first match).
func findDevOpsInstance(instances []aws.Ec2Instance, search string) (aws.Ec2Instance, error) {
	lower := strings.ToLower(search)
	for _, i := range instances {
		if i.InstanceID == search {
			return i, nil
		}
	}
	for _, i := range instances {
		if strings.EqualFold(i.Name, search) {
			return i, nil
		}
	}
	for _, i := range instances {
		if strings.Contains(strings.ToLower(i.Name), lower) {
			return i, nil
		}
	}
	return aws.Ec2Instance{}, fmt.Errorf("no instance found matching %q", search)
}

// portForwardInputs returns the form fields for SSM port forwarding —
// identical to ec2.go's ssmPortForwardInputs.
func portForwardInputs() []c.InputField {
	return []c.InputField{
		{Label: LocalPortLabel, Placeholder: "8080", CharLimit: 5, Validator: aws.ValidatePort},
		{Label: RemotePortLabel, Placeholder: "8080", CharLimit: 5, Validator: aws.ValidatePort},
	}
}

// portForwardOnSubmit is called by the InputForm on submit — identical logic
// to ec2.go's ssmPortForwardOnSubmit, adapted for the chat model.
func (m DevOpsAgentViewModel) portForwardOnSubmit(values c.InputFormResult) tea.Cmd {
	inst := m.pendingTunnelInstance
	localPort := mustAtoi(values[LocalPortLabel])
	remotePort := mustAtoi(values[RemotePortLabel])
	config := aws.PortForwardConfig{LocalPort: localPort, RemotePort: remotePort}
	cmd := m.app.AWS.Ssm.BuildPortForwardCmd(inst.InstanceID, config)
	desc := fmt.Sprintf("tunnel %s (%s) localhost:%d → :%d", inst.Name, inst.InstanceID, localPort, remotePort)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return ssmExitedMsg{description: desc, err: err}
	})
}

// rdsPortForwardOnSubmit is used when tunnelling to an RDS instance through a
// bastion — calls BuildRemotePortForwardCmd with the RDS endpoint pre-filled.
func (m DevOpsAgentViewModel) rdsPortForwardOnSubmit(values c.InputFormResult) tea.Cmd {
	localPort := mustAtoi(values[LocalPortLabel])
	config := aws.PortForwardConfig{
		LocalPort:  localPort,
		RemotePort: int(m.pendingRDSInstance.Port),
		RemoteHost: m.pendingRDSInstance.Endpoint,
	}
	cmd := m.app.AWS.Ssm.BuildRemotePortForwardCmd(m.pendingBastion.InstanceID, config)
	desc := fmt.Sprintf(
		"RDS tunnel %s → localhost:%d (via bastion %s)",
		m.pendingRDSInstance.Id, localPort, m.pendingBastion.Name,
	)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return ssmExitedMsg{description: desc, err: err}
	})
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m DevOpsAgentViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case common.ContentWindowSizeMsg:
		m.window = msg
		m.updateLayout()
		return m, nil

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		if m.isSending {
			m.syncViewport()
		}
		return m, cmd

	case agentSpacesLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = devOpsStateSelectSpace
			return m, nil
		}
		m.spaces = msg.spaces
		if len(m.spaces) == 1 {
			return m.enterChat(m.spaces[0])
		}
		items := make([]list.Item, len(m.spaces))
		for i, sp := range m.spaces {
			items[i] = sp
		}
		m.spaceList.SetItems(items)
		m.spaceList.SetSize(m.window.Width, m.window.Height)
		m.state = devOpsStateSelectSpace
		return m, nil

	case agentChatResponseMsg:
		m.isSending = false
		if msg.err != nil {
			m.appendMsg("agent", fmt.Sprintf("[error: %v — please try again]", msg.err))
		} else if msg.result != nil {
			m.executionId = msg.result.ExecutionId
			m.appendMsg("agent", msg.result.Response)
		}
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	// ── SSM instance resolution ───────────────────────────────────────────────

	case ssmInstanceResolvedMsg:
		m.isSending = false
		if msg.err != nil {
			m.replaceLastMsg("system", fmt.Sprintf("Error: %v", msg.err))
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}

		switch msg.kind {
		case ssmKindShell:
			// Same as ec2.go ssmShell(): BuildSessionCmd + tea.ExecProcess.
			m.replaceLastMsg("system", fmt.Sprintf(
				"Starting shell on %s (%s) — press ctrl+c to end the session.",
				msg.instance.Name, msg.instance.InstanceID,
			))
			m.syncViewport()
			m.viewport.GotoBottom()
			ssmCmd := m.app.AWS.Ssm.BuildSessionCmd(msg.instance.InstanceID)
			desc := fmt.Sprintf("shell on %s (%s)", msg.instance.Name, msg.instance.InstanceID)
			return m, tea.ExecProcess(ssmCmd, func(err error) tea.Msg {
				return ssmExitedMsg{description: desc, err: err}
			})

		case ssmKindTunnel:
			// Same as ec2.go ssmPortForward(): open the InputForm overlay.
			m.pendingTunnelInstance = msg.instance
			m.replaceLastMsg("system", fmt.Sprintf(
				"Instance resolved: %s (%s). Enter port numbers below.",
				msg.instance.Name, msg.instance.InstanceID,
			))
			m.syncViewport()
			m.viewport.GotoBottom()
			m.isInputFormActive = true
			m.inputForm = c.NewInputForm(
				fmt.Sprintf("Port Forward: %s", msg.instance.Name),
				portForwardInputs(),
				m.portForwardOnSubmit,
			)
			return m, nil
		}

	case ssmRDSTunnelResolvedMsg:
		m.isSending = false
		if msg.err != nil {
			m.replaceLastMsg("system", fmt.Sprintf("Error: %v", msg.err))
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}
		m.pendingRDSInstance = msg.rds
		m.pendingBastion = msg.bastion
		m.replaceLastMsg("system", fmt.Sprintf(
			"RDS %s resolved (endpoint %s:%d, bastion %s). Enter local port below.",
			msg.rds.Id, msg.rds.Endpoint, msg.rds.Port, msg.bastion.Name,
		))
		m.syncViewport()
		m.viewport.GotoBottom()
		m.isInputFormActive = true
		m.inputForm = c.NewInputForm(
			fmt.Sprintf("RDS Tunnel: %s (:%d)", msg.rds.Id, msg.rds.Port),
			[]c.InputField{
				{Label: LocalPortLabel, Placeholder: fmt.Sprintf("%d", msg.rds.Port), CharLimit: 5, Validator: aws.ValidatePort},
			},
			m.rdsPortForwardOnSubmit,
		)
		return m, nil

	// ── InputForm messages (identical to ec2.go handling) ────────────────────

	case c.InputFormCancelMsg:
		m.isInputFormActive = false
		m.appendMsg("system", "Port forward cancelled.")
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	case c.InputFormSubmitMsg:
		m.isInputFormActive = false
		return m, msg.OnSubmit(msg.Values)

	// ── SSM process exit ──────────────────────────────────────────────────────

	case ssmExitedMsg:
		content := fmt.Sprintf("%s ended.", msg.description)
		if msg.err != nil {
			content = fmt.Sprintf("%s exited with error: %v", msg.description, msg.err)
		}
		m.appendMsg("system", content)
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	// ── List command results ───────────────────────────────────────────────────

	case listResultMsg:
		m.isSending = false
		if msg.err != nil {
			m.replaceLastMsg("system", fmt.Sprintf("Error: %v", msg.err))
		} else {
			m.replaceLastMsg("agent", msg.content)
		}
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	// ── Key handling ──────────────────────────────────────────────────────────

	case tea.KeyMsg:
		if key.Matches(msg, constants.Keymap.Back) {
			return m, func() tea.Msg { return common.BackMsg{} }
		}

		switch m.state {

		case devOpsStateSelectSpace:
			if key.Matches(msg, constants.Keymap.Enter) {
				if item, ok := m.spaceList.SelectedItem().(aws.AgentSpace); ok {
					return m.enterChat(item)
				}
				return m, nil
			}
			m.spaceList, cmd = m.spaceList.Update(msg)
			return m, cmd

		case devOpsStateChat:
			// InputForm takes over all keys when active — same as ec2.go.
			if m.isInputFormActive {
				m.inputForm, cmd = m.inputForm.Update(msg)
				return m, cmd
			}

			// Scroll bindings (arrow keys not consumed by single-line textinput).
			switch msg.String() {
			case "up":
				m.viewport.LineUp(3)
				return m, nil
			case "down":
				m.viewport.LineDown(3)
				return m, nil
			case "pgup", "ctrl+u":
				m.viewport.HalfViewUp()
				return m, nil
			case "pgdown", "ctrl+d":
				m.viewport.HalfViewDown()
				return m, nil
			}

			if key.Matches(msg, devOpsSendKey) && !m.isSending {
				text := strings.TrimSpace(m.input.Value())
				if text == "" {
					return m, nil
				}
				m.input.SetValue("")
				if strings.HasPrefix(text, "#") {
					return m.handleHashCommand(text)
				}
				return m.sendToAgent(text)
			}

			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	// Delegate non-key messages.
	switch m.state {
	case devOpsStateSelectSpace:
		m.spaceList, cmd = m.spaceList.Update(msg)
	case devOpsStateChat:
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

// ── Action helpers ────────────────────────────────────────────────────────────

func (m DevOpsAgentViewModel) enterChat(space aws.AgentSpace) (DevOpsAgentViewModel, tea.Cmd) {
	m.selectedSpace = space
	m.state = devOpsStateChat
	m.input.Focus()
	m.syncViewport()
	return m, nil
}

func (m DevOpsAgentViewModel) sendToAgent(text string) (DevOpsAgentViewModel, tea.Cmd) {
	m.appendMsg("user", text)
	m.isSending = true
	execId := m.executionId
	space := m.selectedSpace
	m.syncViewport()
	m.viewport.GotoBottom()
	return m, tea.Batch(m.spinner.Tick, m.sendToAgentCmd(space, execId, text))
}

// handleHashCommand dispatches #shell / #tunnel / #help commands.
func (m DevOpsAgentViewModel) handleHashCommand(input string) (DevOpsAgentViewModel, tea.Cmd) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return m, nil
	}
	cmdName := strings.ToLower(strings.TrimPrefix(parts[0], "#"))
	args := parts[1:]

	m.appendMsg("user", input)

	switch cmdName {
	case "shell":
		if len(args) == 0 {
			m.appendMsg("system", "Usage: #shell <instance-id-or-name>")
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}
		m.appendMsg("system", fmt.Sprintf("Resolving instance %q...", args[0]))
		m.isSending = true
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, tea.Batch(
			m.spinner.Tick,
			resolveInstanceCmd(m.app.Context, m.app.AWS.Ec2, args[0], ssmKindShell),
		)

	case "tunnel":
		if len(args) == 0 {
			m.appendMsg("system", "Usage: #tunnel <ec2-or-rds-name>")
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}
		m.appendMsg("system", fmt.Sprintf("Resolving tunnel target %q (trying EC2, then RDS)...", args[0]))
		m.isSending = true
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, tea.Batch(
			m.spinner.Tick,
			resolveTunnelTargetCmd(m.app.Context, m.app.AWS.Ec2, m.app.AWS.Rds, args[0]),
		)

	case "listec2instances":
		m.appendMsg("system", "Fetching EC2 instances...")
		m.isSending = true
		m.syncViewport()
		m.viewport.GotoBottom()
		ctx := m.app.Context
		ec2Svc := m.app.AWS.Ec2
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			instances, err := ec2Svc.ListInstances(ctx)
			if err != nil {
				return listResultMsg{err: err}
			}
			return listResultMsg{content: formatEC2List(instances)}
		})

	case "listrdsinstances":
		m.appendMsg("system", "Fetching RDS instances...")
		m.isSending = true
		m.syncViewport()
		m.viewport.GotoBottom()
		ctx := m.app.Context
		rdsSvc := m.app.AWS.Rds
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			instances, err := rdsSvc.ListDBInstances(ctx)
			if err != nil {
				return listResultMsg{err: err}
			}
			return listResultMsg{content: formatRDSList(instances)}
		})

	case "listsfn":
		m.appendMsg("system", "Fetching Step Functions...")
		m.isSending = true
		m.syncViewport()
		m.viewport.GotoBottom()
		ctx := m.app.Context
		sfnSvc := m.app.AWS.Sfn
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			machines, err := sfnSvc.ListStateMachines(ctx)
			if err != nil {
				return listResultMsg{err: err}
			}
			return listResultMsg{content: formatSFNList(machines)}
		})

	case "listsfnruns":
		if len(args) == 0 {
			m.appendMsg("system", "Usage: #listsfnruns <state-machine-name>")
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}
		name := strings.Join(args, " ")
		m.appendMsg("system", fmt.Sprintf("Fetching executions for %q...", name))
		m.isSending = true
		m.syncViewport()
		m.viewport.GotoBottom()
		ctx := m.app.Context
		sfnSvc := m.app.AWS.Sfn
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			machine, err := sfnSvc.FindStateMachineByName(ctx, name)
			if err != nil {
				return listResultMsg{err: err}
			}
			execs, err := sfnSvc.ListExecutions(ctx, machine.Arn, 20)
			if err != nil {
				return listResultMsg{err: err}
			}
			return listResultMsg{content: formatSFNExecutions(machine.Name, execs)}
		})

	case "help":
		m.appendMsg("system", devOpsHelpText())
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	default:
		m.appendMsg("system", fmt.Sprintf("Unknown command %q. Type #help for available commands.", cmdName))
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}
}

func (m *DevOpsAgentViewModel) appendMsg(role, content string) {
	m.history = append(m.history, devOpsMessage{role: role, content: content})
}

func (m *DevOpsAgentViewModel) replaceLastMsg(role, content string) {
	if len(m.history) > 0 {
		m.history[len(m.history)-1] = devOpsMessage{role: role, content: content}
		return
	}
	m.appendMsg(role, content)
}

func devOpsHelpText() string {
	return "Available commands:\n" +
		"  #shell <instance>              — open an SSM shell session\n" +
		"  #tunnel <ec2-or-rds-name>      — port forwarding; EC2 direct, RDS via bastion\n" +
		"  #listec2instances              — list EC2 instances (name + ID + state)\n" +
		"  #listrdsinstances              — list RDS instances (ID + engine + endpoint)\n" +
		"  #listsfn                       — list Step Functions state machines\n" +
		"  #listsfnruns <sfn-name>        — list recent executions for a state machine\n" +
		"  #help                          — show this message\n" +
		"\n" +
		"<instance> accepts an instance ID (i-…) or a name tag (partial match ok).\n" +
		"Scroll: ↑/↓ line  •  PgUp/PgDn or ctrl+u/ctrl+d half page"
}

// ── List formatters ───────────────────────────────────────────────────────────

func formatEC2List(instances []aws.Ec2Instance) string {
	if len(instances) == 0 {
		return "No EC2 instances found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("EC2 Instances (%d):\n", len(instances)))
	for _, i := range instances {
		name := i.Name
		if name == "" {
			name = "(unnamed)"
		}
		sb.WriteString(fmt.Sprintf("  • %-36s  %-21s  %s\n", name, i.InstanceID, i.State))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatRDSList(instances []aws.RDSInstance) string {
	if len(instances) == 0 {
		return "No RDS instances found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("RDS Instances (%d):\n", len(instances)))
	for _, i := range instances {
		engine := i.DbEngine + " " + i.DbVersion
		endpoint := i.Endpoint
		if i.Port > 0 {
			endpoint = fmt.Sprintf("%s:%d", i.Endpoint, i.Port)
		}
		sb.WriteString(fmt.Sprintf("  • %-36s  %-22s  %-10s  %s\n",
			i.Id, engine, i.Status, endpoint))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatSFNList(machines []aws.StateMachine) string {
	if len(machines) == 0 {
		return "No Step Functions state machines found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Step Functions (%d):\n", len(machines)))
	for _, m := range machines {
		sb.WriteString(fmt.Sprintf("  • %-48s  %s\n", m.Name, m.Type))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatSFNExecutions(machineName string, execs []aws.SfnExecution) string {
	if len(execs) == 0 {
		return fmt.Sprintf("No executions found for %q.", machineName)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Executions for %s (%d):\n", machineName, len(execs)))
	for _, e := range execs {
		started := e.StartDate.Format("2006-01-02 15:04:05")
		sb.WriteString(fmt.Sprintf("  • %-40s  %-12s  %s\n", e.Name, e.Status, started))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func mustAtoi(s string) int {
	v := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		v = v*10 + int(ch-'0')
	}
	return v
}

// ── Layout ────────────────────────────────────────────────────────────────────

func (m *DevOpsAgentViewModel) updateLayout() {
	w, h := m.window.Width, m.window.Height
	m.spaceList.SetSize(w, h)
	m.input.Width = max(1, w-4)
	m.viewport.Width = w
	m.viewport.Height = max(1, h-devOpsInputRows)
	m.syncViewport()
}

func (m *DevOpsAgentViewModel) syncViewport() {
	m.viewport.SetContent(m.renderHistory())
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m DevOpsAgentViewModel) View() string {
	switch m.state {

	case devOpsStateLoadingSpaces:
		return constants.DocStyle.Render(
			m.spinner.View() + "  Fetching DevOps Agent spaces...",
		)

	case devOpsStateSelectSpace:
		if m.err != nil {
			return constants.DocStyle.Render(
				devOpsErrorStyle.Render("Error: "+m.err.Error()) + "\n\n" +
					devOpsDimStyle.Render("Make sure AWS DevOps Agent is set up in your account and region."),
			)
		}
		if len(m.spaces) == 0 {
			return constants.DocStyle.Render(
				devOpsErrorStyle.Render("No agent spaces found.") + "\n\n" +
					devOpsDimStyle.Render("Create an agent space in the AWS DevOps Agent console first."),
			)
		}
		return m.spaceList.View()

	case devOpsStateChat:
		sep := devOpsSeparatorStyle.Render(strings.Repeat("─", max(0, m.window.Width)))
		inputLine := devOpsInputPromptStyle.Render("> ") + m.input.View()
		content := lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			sep,
			inputLine,
		)
		// Overlay the port-forward InputForm when active — same pattern as ec2.go.
		if m.isInputFormActive {
			return overlay.Composite(
				m.inputForm.View(),
				content,
				overlay.Center,
				overlay.Center,
				0, 0,
			)
		}
		return content
	}
	return ""
}

// renderHistory builds the viewport content from m.history.
func (m *DevOpsAgentViewModel) renderHistory() string {
	if len(m.history) == 0 && !m.isSending {
		return devOpsDimStyle.Render(
			"\n  Agent space: " + m.selectedSpace.Name +
				"\n\n  Ask anything, or type #help for special commands.\n" +
				"\n  Scroll: ↑/↓ · PgUp/PgDn · ctrl+u/ctrl+d",
		)
	}

	wrapWidth := m.viewport.Width - 4
	if wrapWidth < 20 {
		wrapWidth = 20
	}

	var sb strings.Builder
	for _, msg := range m.history {
		sb.WriteString("\n")
		switch msg.role {
		case "user":
			sb.WriteString(devOpsUserLabelStyle.Render("  You") + "\n")
			sb.WriteString(wordwrap.String("  "+msg.content, wrapWidth))
		case "agent":
			sb.WriteString(devOpsAgentLabelStyle.Render("  DevOps Agent") + "\n")
			sb.WriteString(wordwrap.String("  "+msg.content, wrapWidth))
		case "system":
			sb.WriteString(devOpsSystemLabelStyle.Render("  ─") + " ")
			sb.WriteString(devOpsDimStyle.Render(wordwrap.String(msg.content, wrapWidth-2)))
		}
		sb.WriteString("\n")
	}

	if m.isSending {
		sb.WriteString("\n")
		sb.WriteString(devOpsAgentLabelStyle.Render("  DevOps Agent") + "\n")
		sb.WriteString("  " + m.spinner.View() + "\n")
	}

	return sb.String()
}
