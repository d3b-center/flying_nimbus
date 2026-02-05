package tui

import (
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	devConsoleRatio      = 0.25
	minDevConsoleHeight  = 3
	runtimeStatsInterval = 5 * time.Second
)

func InitRoot(appService *app.App) RootModel {
	stack := make([]tea.Model, 0, 3)

	// Rename to not conflict with Standard library Context
	if appService == nil {
		appService = &app.App{}
	}
	stack = append(stack, NewProvidersModel(appService))

	return RootModel{
		appService:     appService,
		stack:          stack,
		showDevConsole: false,
	}

}

type RootModel struct {
	appService *app.App
	// Might need for mutex locks
	stack             []tea.Model
	showDevConsole    bool
	WindowSize        tea.WindowSizeMsg
	ContentWindowSize common.ContentWindowSizeMsg
	devConsoleHeight  int
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

	if constants.WindowSize.Width == 0 || constants.WindowSize.Height == 0 {
		return content
	}

	consoleHeight := m.devConsoleHeight
	if !m.showDevConsole {
		consoleHeight = 0
	}

	var lines []string
	if m.appService != nil && m.appService.LogBuffer != nil {
		lines = m.appService.LogBuffer.Lines()
	}
	consoleView := common.RenderDevConsole(lines, constants.WindowSize.Width, consoleHeight)

	return lipgloss.JoinVertical(lipgloss.Top, renderTitleBar(current, m.WindowSize.Width), content, consoleView)
}

func renderTitleBar(m tea.Model, width int) string {
	nimbus, ok := m.(common.NimbusModel)
	if !ok {
		return ""
	}
	title := fmt.Sprintf(" ☁️ Flying Nimbus - %s", nimbus.Title())

	left := lipgloss.NewStyle().
		Bold(true).
		Render(title)

	right := lipgloss.NewStyle().
		Render("prod / us-east-1")

	titleBar := lipgloss.JoinHorizontal(lipgloss.Left, left, lipgloss.NewStyle().Width(max(0, width-lipgloss.Width(left)-lipgloss.Width(right)-4)).Render(""),
		right,
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		//Padding(0, 1).
		Height(constants.TitleBarHeight).
		Width(width).
		Render(titleBar)

}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		slog.Info("window resize",
			slog.Int("width", msg.Width),
			slog.Int("height", msg.Height),
		)

		window, content, devHeight := computeLayout(msg, m.showDevConsole)

		constants.WindowSize = tea.WindowSizeMsg{
			Height: content.Height,
			Width:  content.Width,
		}
		m.WindowSize = window
		m.ContentWindowSize = content
		m.devConsoleHeight = devHeight
		return m, func() tea.Msg {
			return content
		}

	case common.NavigateMsg:
		m.stack = append(m.stack, msg.Model)
		m.logNavigation("push", msg.Model)
		return m, tea.Batch(msg.Model.Init())

	case common.BackMsg:
		if len(m.stack) > 1 {
			m.logNavigation("pop", m.stack[len(m.stack)-1])
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil
	case runtimeStatsMsg:
		m.logRuntimeStats()
		return m, runtimeStatsCmd()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, constants.Keymap.Back):
			return m, func() tea.Msg {
				return common.BackMsg{}
			}
		case key.Matches(msg, constants.Keymap.Quit):
			return m, tea.Quit
		case key.Matches(msg, constants.Keymap.ToggleDevConsole):
			m.showDevConsole = !m.showDevConsole

			_, content, devHeight := computeLayout(
				tea.WindowSizeMsg(m.WindowSize),
				m.showDevConsole,
			)

			m.devConsoleHeight = devHeight
			m.ContentWindowSize = content

			constants.WindowSize = tea.WindowSizeMsg{
				Height: content.Height,
				Width:  content.Width,
			}
			return m, tea.Batch(func() tea.Msg {
				return content
			})
		}
	}

	// delegate everything else
	current := m.stack[len(m.stack)-1]
	delegateMsg := msg
	next, cmd := current.Update(delegateMsg)
	m.stack[len(m.stack)-1] = next
	return m, tea.Batch(cmd)
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
) (tea.WindowSizeMsg, common.ContentWindowSizeMsg, int) {
	hFrame, wFrame := constants.DocStyle.GetFrameSize()

	usableHeight := msg.Height - hFrame
	usableWidth := msg.Width - wFrame

	devHeight := devConsoleHeight(usableHeight, showDevConsole)

	content := common.ContentWindowSizeMsg{
		Width:  usableWidth,
		Height: usableHeight - devHeight - constants.TitleBarHeight,
	}

	window := tea.WindowSizeMsg{
		Width:  usableWidth,
		Height: usableHeight,
	}

	return window, content, devHeight
}
