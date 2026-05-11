package views

import (
	"fmt"
	"strings"
	"time"

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

// resourceNamesMsg is produced by background fetches that populate the
// autocomplete name caches for #ls suggestions.
type resourceNamesMsg struct {
	kind  string   // "ec2" | "rds" | "sfn" | "emr" | "ecs-clusters" | "ecs-services" | "ecs-tasks"
	names []string // nil signals fetch failure (cache stays nil → retry on next keystroke)
	key   string   // for compound caches: cluster name (services) or "cluster/service" (tasks)
}

// ── View states & constants ───────────────────────────────────────────────────

type devOpsAgentViewState int

const (
	devOpsStateLoadingSpaces devOpsAgentViewState = iota
	devOpsStateSelectSpace
	devOpsStateChat
)

// devOpsInputRows is the fixed row budget below the viewport.
const devOpsInputRows = 3

// devOpsAgentCostPerSecond is the AWS DevOps Agent billing rate in USD.
const devOpsAgentCostPerSecond = 0.00083

// ── Key bindings ──────────────────────────────────────────────────────────────

var devOpsSendKey = key.NewBinding(
	key.WithKeys("enter"),
	key.WithHelp("enter", "send"),
)

// ── Styles ────────────────────────────────────────────────────────────────────

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

	devOpsCostStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true)
)

// ── Chat message ──────────────────────────────────────────────────────────────

type devOpsMessage struct {
	role    string // "user" | "agent" | "system" | "cost"
	content string
}

// ── View model ────────────────────────────────────────────────────────────────

// DevOpsAgentViewModel is the TUI for the AWS DevOps Agent chat screen.
type DevOpsAgentViewModel struct {
	app    *app.App
	window common.ContentWindowSizeMsg
	state  devOpsAgentViewState

	// Space selection
	spaceList list.Model
	spaces    []aws.AgentSpace

	// Chat session
	selectedSpace aws.AgentSpace
	executionId   string
	history       []devOpsMessage
	viewport      viewport.Model
	input         textinput.Model
	spinner       spinner.Model
	isSending     bool

	// Usage tracking
	queryStartTime time.Time
	totalDuration  time.Duration
	queryCount     int

	// Port-forward form (reuses the same InputForm component as ec2.go)
	inputForm             c.InputForm
	isInputFormActive     bool
	pendingTunnelInstance aws.Ec2Instance // EC2 direct port-forward
	pendingRDSInstance    aws.RDSInstance // RDS remote port-forward target
	pendingBastion        aws.Ec2Instance // bastion for RDS tunnels

	// agentUnavailable is set when ListAgentSpaces fails or returns nothing.
	// #-commands still work; plain-text questions return a notice instead.
	agentUnavailable bool

	// suggestions holds the current autocomplete matches for the typed # prefix.
	suggestions []string

	// Command history — up to 5 most recently sent commands, newest first.
	// cmdHistoryIdx is -1 when not navigating; ≥0 while navigating.
	// inputSnapshot preserves whatever the user was typing when they started.
	cmdHistory    []string
	cmdHistoryIdx int
	inputSnapshot string

	// Resource name caches for autocomplete. nil = not yet fetched.
	// An empty non-nil slice means the fetch succeeded but found nothing.
	cachedEC2Names []string
	cachedRDSNames []string
	cachedSFNNames []string
	cachedEMRNames []string
	// ECS caches — compound keys: clusterName for services, "cluster/service" for tasks.
	cachedECSClusters []string
	cachedECSServices map[string][]string // cluster → []serviceName
	cachedECSTasks    map[string][]string // "cluster/service" → []taskID

	err error
}

// InitDevOpsAgentViewModel creates the initial DevOps Agent view.
func InitDevOpsAgentViewModel(appService *app.App, windowSize common.ContentWindowSizeMsg) DevOpsAgentViewModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))

	ti := textinput.New()
	ti.Placeholder = "Ask a question or type #help for commands..."
	ti.CharLimit = 2048
	ti.Width = max(1, windowSize.Width-4)
	ti.ShowSuggestions = true
	ti.SetSuggestions(allHashCommands)

	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, windowSize.Width, windowSize.Height)
	l.Title = "Select an agent space"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	vpHeight := max(1, windowSize.Height-devOpsInputRows)
	vp := viewport.New(windowSize.Width, vpHeight)

	return DevOpsAgentViewModel{
		app:           appService,
		window:        windowSize,
		state:         devOpsStateLoadingSpaces,
		spaceList:     l,
		viewport:      vp,
		input:         ti,
		spinner:       sp,
		cmdHistoryIdx: -1,
	}
}

// ── NimbusModel interface ─────────────────────────────────────────────────────

func (m DevOpsAgentViewModel) Title() string {
	if m.agentUnavailable {
		return "DevOps Agent (commands only)"
	}
	return "DevOps Agent"
}

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
		if msg.err != nil || len(msg.spaces) == 0 {
			m.agentUnavailable = true
			m.state = devOpsStateChat
			m.input.Focus()
			m.syncViewport()
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
		elapsed := time.Since(m.queryStartTime)
		m.totalDuration += elapsed
		m.queryCount++
		if msg.err != nil {
			m.appendMsg("agent", fmt.Sprintf("[error: %v — please try again]", msg.err))
		} else if msg.result != nil {
			m.executionId = msg.result.ExecutionId
			m.appendMsg("agent", msg.result.Response)
		}
		cost := elapsed.Seconds() * devOpsAgentCostPerSecond
		m.appendMsg("cost", fmt.Sprintf("%.1fs · $%.5f", elapsed.Seconds(), cost))
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

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

	case c.InputFormCancelMsg:
		m.isInputFormActive = false
		m.appendMsg("system", "Port forward cancelled.")
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	case c.InputFormSubmitMsg:
		m.isInputFormActive = false
		return m, msg.OnSubmit(msg.Values)

	case ssmExitedMsg:
		content := fmt.Sprintf("%s ended.", msg.description)
		if msg.err != nil {
			content = fmt.Sprintf("%s exited with error: %v", msg.description, msg.err)
		}
		m.appendMsg("system", content)
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

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

	case resourceNamesMsg:
		switch msg.kind {
		case "ec2":
			m.cachedEC2Names = msg.names
		case "rds":
			m.cachedRDSNames = msg.names
		case "sfn":
			m.cachedSFNNames = msg.names
		case "emr":
			m.cachedEMRNames = msg.names
		case "ecs-clusters":
			m.cachedECSClusters = msg.names
		case "ecs-services":
			if m.cachedECSServices == nil {
				m.cachedECSServices = make(map[string][]string)
			}
			m.cachedECSServices[msg.key] = msg.names
		case "ecs-tasks":
			if m.cachedECSTasks == nil {
				m.cachedECSTasks = make(map[string][]string)
			}
			m.cachedECSTasks[msg.key] = msg.names
		}
		m.refreshSuggestions()
		return m, nil

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
			if m.isInputFormActive {
				m.inputForm, cmd = m.inputForm.Update(msg)
				return m, cmd
			}

			switch msg.String() {
			case "up":
				inputEmpty := strings.TrimSpace(m.input.Value()) == ""
				if inputEmpty || (m.cmdHistoryIdx == -1 && len(m.cmdHistory) == 0) {
					// Empty input (or no history at all) → scroll viewport.
					m.viewport.ScrollUp(3)
					return m, nil
				}
				// Non-empty input: enter / advance history navigation.
				if m.cmdHistoryIdx == -1 {
					m.inputSnapshot = m.input.Value()
					m.cmdHistoryIdx = 0
				} else if m.cmdHistoryIdx < len(m.cmdHistory)-1 {
					m.cmdHistoryIdx++
				}
				m.input.SetValue(m.cmdHistory[m.cmdHistoryIdx])
				m.input.CursorEnd()
				m.refreshSuggestions()
				return m, nil

			case "down":
				if m.cmdHistoryIdx == -1 {
					// Not in history navigation mode → scroll viewport.
					m.viewport.ScrollDown(3)
					return m, nil
				}
				if m.cmdHistoryIdx > 0 {
					m.cmdHistoryIdx--
					m.input.SetValue(m.cmdHistory[m.cmdHistoryIdx])
				} else {
					m.cmdHistoryIdx = -1
					m.input.SetValue(m.inputSnapshot)
				}
				m.input.CursorEnd()
				m.refreshSuggestions()
				return m, nil

			case "pgup", "ctrl+u":
				m.viewport.HalfPageUp()
				return m, nil
			case "pgdown", "ctrl+d":
				m.viewport.HalfPageDown()
				return m, nil
			}

			if key.Matches(msg, devOpsSendKey) && !m.isSending {
				text := strings.TrimSpace(m.input.Value())
				if text == "" {
					return m, nil
				}
				m.pushCmdHistory(text)
				m.cmdHistoryIdx = -1
				m.inputSnapshot = ""
				m.input.SetValue("")
				m.refreshSuggestions()
				if strings.HasPrefix(text, "#") {
					return m.handleHashCommand(text)
				}
				return m.sendToAgent(text)
			}

			// A printable character typed while navigating history exits
			// history mode so the user can edit without clobbering the snapshot.
			if msg.Type == tea.KeyRunes && m.cmdHistoryIdx != -1 {
				m.cmdHistoryIdx = -1
				m.inputSnapshot = ""
			}

			m.input, cmd = m.input.Update(msg)
			m.refreshSuggestions()
			return m, tea.Batch(cmd, m.fetchResourceNamesIfNeeded())
		}
	}

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
	// Expand any embedded #ls resource references so the agent gets concrete
	// identifiers rather than the shorthand syntax.
	expanded := expandResourceRefs(text)

	m.appendMsg("user", text)
	if expanded != text {
		m.appendMsg("system", "→ "+expanded)
	}

	if m.agentUnavailable {
		m.appendMsg("agent",
			"AWS DevOps Agent is not available in this account or region.\n"+
				"You can still use all #list*, #shell, and #tunnel commands — type #help for the full list.")
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil
	}
	m.isSending = true
	m.queryStartTime = time.Now()
	execId := m.executionId
	space := m.selectedSpace
	m.syncViewport()
	m.viewport.GotoBottom()
	return m, tea.Batch(m.spinner.Tick, m.sendToAgentCmd(space, execId, expanded))
}

// fetchResourceNamesIfNeeded fires a background API call to populate a
// resource-name cache the first time the user types a prefix that needs it.
// Returns nil if no fetch is required (cache already populated or wrong prefix).
func (m *DevOpsAgentViewModel) fetchResourceNamesIfNeeded() tea.Cmd {
	if m.app == nil || m.app.AWS == nil {
		return nil
	}
	val := strings.ToLower(m.input.Value())
	lsPart := lsPartFrom(val) // "#ls ..." tail, empty if no #ls present

	switch {
	case strings.HasPrefix(lsPart, "#ls ec2 ") && m.cachedEC2Names == nil:
		return fetchResourceNamesCmd(m.app, "ec2")
	case strings.HasPrefix(val, "#shell ") && m.cachedEC2Names == nil:
		return fetchResourceNamesCmd(m.app, "ec2")
	case strings.HasPrefix(lsPart, "#ls rds ") && m.cachedRDSNames == nil:
		return fetchResourceNamesCmd(m.app, "rds")
	case strings.HasPrefix(lsPart, "#ls sfn ") && m.cachedSFNNames == nil:
		return fetchResourceNamesCmd(m.app, "sfn")
	case strings.HasPrefix(lsPart, "#ls emr ") && m.cachedEMRNames == nil:
		return fetchResourceNamesCmd(m.app, "emr")
	case (strings.HasPrefix(lsPart, "#ls ecs ") || strings.HasPrefix(val, "#logs ecs ")) &&
		m.cachedECSClusters == nil:
		return fetchResourceNamesCmd(m.app, "ecs-clusters")
	}

	// ECS services — fetch when cluster name is complete.
	for _, p := range []string{"#ls ecs ", "#logs ecs "} {
		src := lsPart
		if p == "#logs ecs " {
			src = val
		}
		if strings.HasPrefix(src, p) {
			rest := src[len(p):]
			if spaceIdx := strings.Index(rest, " "); spaceIdx >= 0 {
				cluster := rest[:spaceIdx]
				if cluster != "" {
					if m.cachedECSServices == nil || m.cachedECSServices[cluster] == nil {
						return fetchECSServicesCmd(m.app, cluster)
					}
				}
			}
		}
	}

	// ECS task IDs — fetch for both #logs ecs and #ls ecs when the full
	// cluster/service context is known and the "task" keyword has been typed.
	for _, ecsPrefix := range []string{"#logs ecs ", "#ls ecs "} {
		src := val
		if ecsPrefix == "#ls ecs " {
			src = lsPartFrom(val)
		}
		if !strings.HasPrefix(src, ecsPrefix) {
			continue
		}
		rest := src[len(ecsPrefix):]
		if svcIdx := strings.Index(rest, " service "); svcIdx >= 0 {
			cluster := rest[:svcIdx]
			afterSvc := rest[svcIdx+len(" service "):]
			if taskIdx := strings.Index(afterSvc, " task "); taskIdx >= 0 {
				service := afterSvc[:taskIdx]
				cacheKey := cluster + "/" + service
				if m.cachedECSTasks == nil || m.cachedECSTasks[cacheKey] == nil {
					return fetchECSTasksCmd(m.app, cluster, service, cacheKey)
				}
			}
		}
	}

	return nil
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

// pushCmdHistory prepends cmd to the command history, keeping at most 5
// entries. Consecutive duplicate commands are not stored.
func (m *DevOpsAgentViewModel) pushCmdHistory(cmd string) {
	if cmd == "" {
		return
	}
	if len(m.cmdHistory) > 0 && m.cmdHistory[0] == cmd {
		return // don't duplicate the most recent entry
	}
	m.cmdHistory = append([]string{cmd}, m.cmdHistory...)
	if len(m.cmdHistory) > 5 {
		m.cmdHistory = m.cmdHistory[:5]
	}
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
		var sep string
		if len(m.suggestions) > 0 {
			sep = m.renderSuggestionBar()
		} else {
			sep = m.renderSeparator()
		}
		inputLine := devOpsInputPromptStyle.Render("> ") + m.input.View()
		content := lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			sep,
			inputLine,
		)
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

// renderSuggestionBar replaces the separator while # autocomplete is active.
func (m DevOpsAgentViewModel) renderSuggestionBar() string {
	typed := m.input.Value()
	tabHint := devOpsDimStyle.Render(" ↹  ")
	availWidth := m.window.Width - lipgloss.Width(tabHint) - 2

	// For mid-sentence #ls references, only show the #ls tail in the bar so
	// the long prose prefix doesn't overwhelm the available width.
	displayTyped := typed
	lowerTyped := strings.ToLower(typed)
	if idx := strings.LastIndex(lowerTyped, "#ls "); idx > 0 {
		displayTyped = typed[idx:]
	}

	var parts []string
	usedWidth := 0

	for i, cmd := range m.suggestions {
		// Strip the prose prefix from the displayed suggestion too.
		displayCmd := cmd
		if lowerCmd := strings.ToLower(cmd); strings.LastIndex(lowerCmd, "#ls ") > 0 {
			displayCmd = cmd[strings.LastIndex(lowerCmd, "#ls "):]
		}

		completion := displayCmd[len(displayTyped):]
		var rendered string
		if i == 0 {
			rendered = devOpsDimStyle.Render(displayTyped) +
				devOpsUserLabelStyle.Render(completion)
		} else {
			rendered = devOpsDimStyle.Render(displayCmd)
		}
		w := lipgloss.Width(rendered)
		sepW := lipgloss.Width(devOpsDimStyle.Render("  "))
		if usedWidth+w+sepW > availWidth && len(parts) > 0 {
			parts = append(parts, devOpsDimStyle.Render("…"))
			break
		}
		parts = append(parts, rendered)
		usedWidth += w + sepW
	}

	return tabHint + strings.Join(parts, devOpsDimStyle.Render("  "))
}

// renderSeparator draws the horizontal rule with an optional right-aligned
// session cost summary.
func (m DevOpsAgentViewModel) renderSeparator() string {
	const lineChar = "─"
	if m.queryCount == 0 {
		return devOpsSeparatorStyle.Render(strings.Repeat(lineChar, max(0, m.window.Width)))
	}
	totalCost := m.totalDuration.Seconds() * devOpsAgentCostPerSecond
	noun := "query"
	if m.queryCount != 1 {
		noun = "queries"
	}
	stats := fmt.Sprintf(" %d %s · %.1fs · $%.5f ", m.queryCount, noun, m.totalDuration.Seconds(), totalCost)
	statsWidth := lipgloss.Width(stats)
	lineWidth := max(0, m.window.Width-statsWidth)
	return devOpsSeparatorStyle.Render(strings.Repeat(lineChar, lineWidth)) +
		devOpsCostStyle.Render(stats)
}

// renderHistory builds the viewport content from m.history.
func (m *DevOpsAgentViewModel) renderHistory() string {
	if len(m.history) == 0 && !m.isSending {
		if m.agentUnavailable {
			return devOpsErrorStyle.Render("\n  AWS DevOps Agent is not available in this account or region.\n") +
				devOpsDimStyle.Render(
					"\n  All #list*, #shell, and #tunnel commands are still available.\n"+
						"  Type #help for the full list.\n"+
						"\n  Scroll: ↑/↓ · PgUp/PgDn · ctrl+u/ctrl+d",
				)
		}
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
		case "cost":
			sb.WriteString(devOpsCostStyle.Render("  " + msg.content))
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
