package tui

import (
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	devConsoleRatio      = 0.25
	minDevConsoleHeight  = 3
	runtimeStatsInterval = 15 * time.Second
	minHelpBarHeight     = 1
	TitleBarInnerHeight  = 1
	TitleBarBorderHeight = 2 // top + bottom
	TitleBarHeight       = TitleBarBorderHeight + TitleBarInnerHeight
)

func InitRoot(appService *app.App) RootModel {
	slog.Debug("InitRoot ")
	stack := make([]tea.Model, 0, 3)

	if appService == nil {
		appService = &app.App{}
	}

	help := help.New()

	m := RootModel{
		appService:     appService,
		stack:          stack,
		showDevConsole: false,
		help:           help,
		isAuthError:    !appService.AWS.LoggedIn,
	}

	stack = append(stack, NewProvidersModel(appService, m.ContentWindowSize))
	m.stack = stack

	return m
}

type RootModel struct {
	appService *app.App
	// Might need for mutex locks
	stack             []tea.Model
	showDevConsole    bool
	WindowSize        tea.WindowSizeMsg
	ContentWindowSize common.ContentWindowSizeMsg
	devConsoleHeight  int
	help              help.Model
	isAuthError       bool
}

func (m RootModel) Init() tea.Cmd {
	if m.appService != nil && m.appService.Logger != nil {
		m.appService.Logger.Info("REST events stub active", slog.String("status", "pending"))
		m.logRuntimeStats()
	}
	return runtimeStatsCmd()
}

func (m RootModel) View() string {
	current := m.stack[len(m.stack)-1]
	content := current.View()

	if m.isAuthError {
		return m.renderAuthModal()
	}

	km := GenerateNimbusKeyMap(current)

	consoleView := m.renderDevConsole()

	help := m.renderHelpMinimal(km)

	main := lipgloss.JoinVertical(lipgloss.Left, m.renderTitleBar(current, m.WindowSize.Width), content, help, consoleView)

	if m.help.ShowAll {
		return m.renderHelpModal(km)
	}

	return main
}

func (m RootModel) renderTitleBar(teaModel tea.Model, width int) string {
	nimbus, ok := teaModel.(common.NimbusModel)
	if !ok {
		return ""
	}
	title := fmt.Sprintf(" ☁️ Flying Nimbus - %s", nimbus.Title())

	left := lipgloss.NewStyle().
		Bold(true).
		Render(title)

	right := lipgloss.NewStyle().
		Render(m.appService.AWS.Identity.WhoAmI())

	titleBar := lipgloss.JoinHorizontal(lipgloss.Top, left, lipgloss.NewStyle().Width(max(0, width-lipgloss.Width(left)-lipgloss.Width(right)-4)).Render(""),
		right,
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(width).
		Render(titleBar)

}

func (m RootModel) renderHelpMinimal(km nimbusKeyMap) string {

	isShowAll := m.help.ShowAll

	m.help.ShowAll = false

	m.help.Width = m.WindowSize.Width
	help := m.help.View(km)
	m.help.ShowAll = isShowAll

	helpStyle := lipgloss.NewStyle().Padding(1, 0, 0, 2)
	return helpStyle.Render(help)
}

func (m RootModel) renderHelpModal(km nimbusKeyMap) string {

	help := m.help.View(km)

	modalContent := lipgloss.JoinVertical(
		lipgloss.Center,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Render("HELP & COMMANDS"),
		"\n",
		help,
		"\n",
		lipgloss.NewStyle().Faint(true).Render("press ? to close"),
	)

	return common.RenderModal(modalContent, m.WindowSize)

}

func (m RootModel) renderDevConsole() string {

	consoleHeight := m.devConsoleHeight
	if !m.showDevConsole {
		consoleHeight = 0
	}

	// Generate Dev Console
	var lines []string
	if m.appService != nil && m.appService.LogBuffer != nil {
		lines = m.appService.LogBuffer.Lines()
	}
	return common.RenderDevConsole(lines, m.WindowSize.Width, consoleHeight)
}

func (m RootModel) renderAuthModal() string {

	lines := []string{
		"We were unable to retrieve AWS credentials.",
		"The required IAM role could not be assumed.",
		"",
		"Please verify that:",
		"  • You are logged in via AWS SSO",
		"  • AWS_PROFILE is configured correctly",
		"  • Your session has not expired",
		"",
	}

	return common.RenderErrorModal(
		"Authentication Required",
		lines,
		"Press q to exit",
		m.WindowSize,
	)
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	current := m.stack[len(m.stack)-1]

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case common.NavigateMsg:
		return m.handleNavigation(msg)
	case common.BackMsg:
		return m.handleBack()
	case runtimeStatsMsg:
		return m.handleRuntimeStats()
	case tea.KeyMsg:
		return m.handleKeyMsg(msg, current)
	}

	// Delegate to current model
	return m.delegateToCurrentModel(msg, current)
}

func (m RootModel) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.computeWindowSize(msg)
	return m, tea.Batch(func() tea.Msg {
		return m.ContentWindowSize
	})
}

func (m RootModel) handleNavigation(msg common.NavigateMsg) (tea.Model, tea.Cmd) {
	m.stack = append(m.stack, msg.Model)
	m.logNavigation("push", msg.Model)
	return m, tea.Batch(msg.Model.Init())
}

func (m RootModel) handleBack() (tea.Model, tea.Cmd) {
	if len(m.stack) > 1 {
		m.logNavigation("pop", m.stack[len(m.stack)-1])
		m.stack = m.stack[:len(m.stack)-1]
	}
	return m, nil
}

func (m RootModel) handleRuntimeStats() (tea.Model, tea.Cmd) {
	m.logRuntimeStats()
	return m, runtimeStatsCmd()
}

func (m RootModel) handleKeyMsg(msg tea.KeyMsg, current tea.Model) (tea.Model, tea.Cmd) {
	// Global help toggle - always handle this first
	if key.Matches(msg, DefaultKeymap.ShowFullHelp) {
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	}

	// When full help is shown or Auth Error, only allow toggling help off and quit
	if m.help.ShowAll || m.isAuthError {
		if key.Matches(msg, DefaultKeymap.Quit) {
			return m, tea.Quit
		}
		// Block all other input when help is shown
		return m, nil
	}

	// Handle quit
	if key.Matches(msg, DefaultKeymap.Quit) {
		return m, tea.Quit
	}

	// Get routing strategy from current model
	strategy := m.getInputRoutingStrategy(current)

	// Handle global keys if routing strategy is RouteGlobalFirst
	if strategy == common.RouteGlobalFirst {
		if handled, model, cmd := m.handleGlobalKeys(msg); handled {
			return model, cmd
		}
	}

	// Delegate to current model
	return m.delegateToCurrentModel(msg, current)
}

func (m RootModel) getInputRoutingStrategy(current tea.Model) common.InputRoutingStrategy {
	if nimbus, ok := current.(common.NimbusModel); ok {
		return nimbus.InputRoutingStrategy()
	}
	return common.RouteGlobalFirst
}

func (m RootModel) handleGlobalKeys(msg tea.KeyMsg) (handled bool, model tea.Model, cmd tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeymap.Back):
		return true, m, tea.Batch(func() tea.Msg {
			return common.BackMsg{}
		})
	case key.Matches(msg, DefaultKeymap.ToggleDevConsole):
		m.showDevConsole = !m.showDevConsole
		content, devHeight := computeLayout(
			tea.WindowSizeMsg(m.WindowSize),
			m.showDevConsole,
		)
		m.devConsoleHeight = devHeight
		m.ContentWindowSize = content
		return true, m, tea.Batch(func() tea.Msg {
			return content
		})
	}
	return false, m, nil
}

func (m RootModel) delegateToCurrentModel(msg tea.Msg, current tea.Model) (tea.Model, tea.Cmd) {
	next, cmd := current.Update(msg)
	m.stack[len(m.stack)-1] = next
	return m, tea.Batch(cmd)
}

func (m *RootModel) computeWindowSize(msg tea.WindowSizeMsg) {
	slog.Info("Root window resize",
		slog.Int("width", msg.Width),
		slog.Int("height", msg.Height),
	)
	wFrame, hFrame := constants.DocStyle.GetFrameSize()
	usableHeight := msg.Height - hFrame
	usableWidth := msg.Width - wFrame

	m.WindowSize = tea.WindowSizeMsg{Height: usableHeight, Width: usableWidth}

	content, devHeight := computeLayout(m.WindowSize, m.showDevConsole)

	m.ContentWindowSize = content
	m.devConsoleHeight = devHeight
}

type runtimeStatsMsg struct{}

// runtimeStatsCmd schedules periodic runtime stats logging.
func runtimeStatsCmd() tea.Cmd {
	return tea.Tick(runtimeStatsInterval, func(time.Time) tea.Msg {
		return runtimeStatsMsg{}
	})
}

// devConsoleHeight computes the fixed dev console height from total height.
func devConsoleHeight(fullHeight int, showDevConsole bool) int {
	if !showDevConsole {
		return 0
	}

	if fullHeight <= 0 {
		return 0
	}

	if devConsoleRatio >= 1.0 {
		slog.Error("Dev Console Height Ratio cannot be greater than 1")
	}

	height := int(float64(fullHeight) * devConsoleRatio)
	height = max(minDevConsoleHeight, height)

	return height
}

// logNavigation emits structured navigation events for the dev console.
func (m RootModel) logNavigation(action string, model tea.Model) {
	if m.appService == nil || m.appService.Logger == nil {
		return
	}

	m.appService.Logger.Info("navigation",
		slog.String("action", action),
		slog.String("screen", fmt.Sprintf("%T", model)),
	)
}

// logRuntimeStats captures memory and GC metrics for the dev console.
func (m RootModel) logRuntimeStats() {
	if m.appService == nil || m.appService.Logger == nil {
		return
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	allocMB := int64(mem.Alloc / 1024 / 1024)
	sysMB := int64(mem.Sys / 1024 / 1024)

	attrs := []slog.Attr{
		slog.Int64("alloc_mb", allocMB),
		slog.Int64("sys_mb", sysMB),
		slog.Uint64("num_gc", uint64(mem.NumGC)),
	}
	if mem.LastGC != 0 {
		lastGC := time.Unix(0, int64(mem.LastGC)).UTC().Format("15:04:05")
		attrs = append(attrs, slog.String("last_gc", lastGC))
	}

	m.appService.Logger.Info("runtime stats", attrsToAny(attrs)...)
}

// attrsToAny adapts slog attributes for variadic logging calls.
func attrsToAny(attrs []slog.Attr) []any {
	out := make([]any, len(attrs))
	for i, attr := range attrs {
		out[i] = attr
	}
	return out
}

// computeLayout calculates the effective window and content dimensions for the TUI.
//
// It accounts for:
//   - The outer document frame (from constants.DocStyle)
//   - An optional developer console at the bottom
//
// The returned values are:
//  1. tea.WindowSizeMsg representing the usable window size (frame removed)
//  2. common.ContentWindowSizeMsg representing the space available to child views
//  3. The computed developer console height
//
// RootModel uses this helper to centralize layout math so resizing logic remains
// consistent across window resize events and UI toggles.
func computeLayout(
	msg tea.WindowSizeMsg,
	showDevConsole bool,
) (common.ContentWindowSizeMsg, int) {

	usableHeight := msg.Height
	usableWidth := msg.Width

	devHeight := devConsoleHeight(usableHeight, showDevConsole)

	content := common.ContentWindowSizeMsg{
		Width:  usableWidth,
		Height: usableHeight - devHeight - TitleBarHeight - minHelpBarHeight,
	}

	return content, devHeight
}
