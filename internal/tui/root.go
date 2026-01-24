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

// NavigateMsg tells the RootModel to push a new screen onto the stack.
type NavigateMsg struct {
	Model tea.Model
}

// BackMsg tells the RootModel to pop the current screen.
type BackMsg struct{}

func InitRoot(appService *app.App) RootModel {
	stack := make([]tea.Model, 0, 3)

	// Rename to not conflict with Standard library Context
	if appService == nil {
		appService = &app.App{}
	}
	stack = append(stack, NewProvidersModel(appService))

	return RootModel{
		appService: appService,
		stack:      stack,
	}

}

type RootModel struct {
	appService *app.App
	// Might need for mutex locks
	stack          []tea.Model
	showDevConsole bool
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

	if !m.showDevConsole {
		return content
	}
	if constants.WindowSize.Width == 0 || constants.WindowSize.Height == 0 {
		return content
	}

	contentHeight := contentHeight(constants.WindowSize.Height, true)
	consoleHeight := devConsoleHeight(constants.WindowSize.Height)
	if consoleHeight <= 0 {
		return content
	}

	var lines []string
	if m.appService != nil && m.appService.LogBuffer != nil {
		lines = m.appService.LogBuffer.Lines()
	}
	consoleView := common.RenderDevConsole(lines, constants.WindowSize.Width, consoleHeight)

	contentView := lipgloss.NewStyle().
		Width(constants.WindowSize.Width).
		Height(contentHeight).
		Render(content)

	return lipgloss.JoinVertical(lipgloss.Top, contentView, consoleView)
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		constants.WindowSize = msg
	case common.NavigateMsg:
		m.stack = append(m.stack, msg.Model)
		m.logNavigation("push", msg.Model)
		return m, tea.Batch(msg.Model.Init(), m.syncWindowSize())

	case common.BackMsg:
		if len(m.stack) > 1 {
			m.logNavigation("pop", m.stack[len(m.stack)-1])
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, m.syncWindowSize()
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
			return m, m.syncWindowSize()
		}
	}

	// delegate everything else
	current := m.stack[len(m.stack)-1]
	delegateMsg := msg
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		if m.showDevConsole {
			size.Height = contentHeight(size.Height, true)
		}
		delegateMsg = size
	}
	next, cmd := current.Update(delegateMsg)
	m.stack[len(m.stack)-1] = next
	return m, cmd
}

type runtimeStatsMsg struct{}

// runtimeStatsCmd schedules periodic runtime stats logging.
func runtimeStatsCmd() tea.Cmd {
	return tea.Tick(runtimeStatsInterval, func(time.Time) tea.Msg {
		return runtimeStatsMsg{}
	})
}

// devConsoleHeight computes the fixed dev console height from total height.
func devConsoleHeight(fullHeight int) int {
	if fullHeight <= 0 {
		return 0
	}
	height := int(float64(fullHeight) * devConsoleRatio)
	if height < minDevConsoleHeight {
		height = minDevConsoleHeight
	}
	if height >= fullHeight {
		height = fullHeight - 1
	}
	if height < 0 {
		height = 0
	}
	return height
}

// contentHeight returns remaining height for the main view.
func contentHeight(fullHeight int, showDevConsole bool) int {
	if !showDevConsole {
		return fullHeight
	}
	return fullHeight - devConsoleHeight(fullHeight)
}

// syncWindowSize replays the latest terminal size to the active model.
func (m RootModel) syncWindowSize() tea.Cmd {
	if constants.WindowSize.Width == 0 || constants.WindowSize.Height == 0 {
		return nil
	}

	return func() tea.Msg {
		return constants.WindowSize
	}
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
