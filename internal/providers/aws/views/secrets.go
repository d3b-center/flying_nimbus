package views

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"flying_nimbus/internal/app"
	aws "flying_nimbus/internal/providers/aws/backend"
	c "flying_nimbus/internal/providers/aws/views/components"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
	overlay "github.com/rmhubbert/bubbletea-overlay"
)

const secretsListWidthRatio = 0.35

// secretSource identifies which backend a SecretItem came from.
type secretSource int

const (
	sourceSecretsManager secretSource = iota
	sourceParameterStore
)

// SecretItem is a unified list item wrapping either a Secret or a Parameter.
type SecretItem struct {
	source secretSource
	secret *aws.Secret
	param  *aws.Parameter
}

func (i SecretItem) Title() string {
	if i.source == sourceSecretsManager {
		return i.secret.Name
	}
	return i.param.Name
}

func (i SecretItem) Description() string {
	if i.source == sourceSecretsManager {
		return "[SM] " + i.secret.ARN
	}
	return "[PS] " + i.param.ARN
}

func (i SecretItem) FilterValue() string { return i.Title() }

// SecretsViewModel renders secrets from Secrets Manager and Parameter Store combined.
type SecretsViewModel struct {
	app                  *app.App
	list                 list.Model
	loader               spinner.Model
	isLoading            bool
	windowSize           common.ContentWindowSizeMsg
	listWidth            int
	detailsWidth         int
	inputRoutingStrategy common.InputRoutingStrategy
	viewport             viewport.Model
	isViewportFocused    bool
	actionMenu           c.ActionMenu
	isActionMenuActive   bool
	// pending counts how many fetch responses are still in flight
	pending int
	items   []list.Item
}

type secretsLoadedMsg struct {
	items  []list.Item
	source secretSource
}
type secretFieldsLoadedMsg map[string]string

// InitSecretsViewModel builds a SecretsViewModel with default layout.
func InitSecretsViewModel(appService *app.App, windowSize common.ContentWindowSizeMsg) SecretsViewModel {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "My Secrets"
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	loader := spinner.New()
	loader.Style = common.SpinnerStyle
	loader.Spinner = spinner.Dot

	vp := viewport.New(0, 0)

	m := SecretsViewModel{
		app:                  appService,
		list:                 l,
		loader:               loader,
		isLoading:            true,
		pending:              2,
		windowSize:           windowSize,
		inputRoutingStrategy: common.RouteGlobalFirst,
		viewport:             vp,
		isViewportFocused:    false,
	}
	m.updateLayout(windowSize)
	return m
}

func (m SecretsViewModel) Title() string {
	return "Secrets"
}

func (m SecretsViewModel) Commands() common.Commands {
	return []key.Binding{c.ForceRefresh, c.ToggleFocus, c.CopySecret}
}

func (m SecretsViewModel) InputRoutingStrategy() common.InputRoutingStrategy {
	return m.inputRoutingStrategy
}

func ownerName(identity *aws.CallerIdentity) string {
	if identity == nil {
		return ""
	}
	return fmt.Sprintf("assumed-role/%s/%s", identity.RoleName, identity.SessionName)
}

// fetchAllSecretsCmd fetches from both Secrets Manager and Parameter Store concurrently.
func fetchAllSecretsCmd(ctx context.Context, app *app.App, owner string) tea.Cmd {
	return tea.Batch(
		fetchSecretsManagerCmd(ctx, app.AWS.Secrets, owner),
		fetchParameterStoreCmd(ctx, app.AWS.ParameterStore, owner),
	)
}

func fetchSecretsManagerCmd(ctx context.Context, svc *aws.SecretsService, owner string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			slog.Warn("Secrets Manager service is nil; skipping fetch")
			return secretsLoadedMsg{source: sourceSecretsManager}
		}
		secrets, err := svc.ListSecretsByOwner(ctx, owner)
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to list secrets: %v", err))
			return secretsLoadedMsg{source: sourceSecretsManager}
		}
		items := make([]list.Item, len(secrets))
		for i, s := range secrets {
			sc := s
			items[i] = SecretItem{source: sourceSecretsManager, secret: &sc}
		}
		return secretsLoadedMsg{items: items, source: sourceSecretsManager}
	}
}

func fetchParameterStoreCmd(ctx context.Context, svc *aws.ParameterStoreService, owner string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			slog.Warn("Parameter Store service is nil; skipping fetch")
			return secretsLoadedMsg{source: sourceParameterStore}
		}
		params, err := svc.ListParametersByOwner(ctx, owner)
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to list parameters: %v", err))
			return secretsLoadedMsg{source: sourceParameterStore}
		}
		items := make([]list.Item, len(params))
		for i, p := range params {
			pc := p
			items[i] = SecretItem{source: sourceParameterStore, param: &pc}
		}
		return secretsLoadedMsg{items: items, source: sourceParameterStore}
	}
}

func fetchSecretFieldsCmd(ctx context.Context, app *app.App, item SecretItem) tea.Cmd {
	return func() tea.Msg {
		var fields map[string]string
		var err error
		if item.source == sourceSecretsManager {
			fields, err = app.AWS.Secrets.FetchSecretFields(ctx, item.secret.ARN)
		} else {
			fields, err = app.AWS.ParameterStore.FetchParameterFields(ctx, item.param.Name)
		}
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to fetch fields: %v", err))
			return secretFieldsLoadedMsg{}
		}
		return secretFieldsLoadedMsg(fields)
	}
}

func (m SecretsViewModel) Init() tea.Cmd {
	slog.Debug("Initialize Secrets Model")
	if m.app == nil || m.app.AWS == nil {
		slog.Warn("AWS service is not initialized; loading empty list")
		return tea.Batch(
			m.loader.Tick,
			func() tea.Msg { return secretsLoadedMsg{source: sourceSecretsManager} },
			func() tea.Msg { return secretsLoadedMsg{source: sourceParameterStore} },
		)
	}

	owner := ownerName(m.app.AWS.Identity)
	slog.Debug(fmt.Sprintf("Owner: %s", owner))

	return tea.Batch(
		m.loader.Tick,
		fetchAllSecretsCmd(m.app.Context, m.app, owner),
	)
}

func (m SecretsViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		m.windowSize = msg
		m.updateLayout(msg)

	case secretsLoadedMsg:
		m.items = append(m.items, msg.items...)
		m.pending--
		slog.Debug(fmt.Sprintf("Received %d items from source %d; pending=%d", len(msg.items), msg.source, m.pending))
		if m.pending <= 0 {
			m.isLoading = false
			m.list.SetItems(m.items)
			m.refreshViewport()
		}

	case secretFieldsLoadedMsg:
		m.isActionMenuActive = true
		m.actionMenu = m.buildCopyMenu(msg)
		m.updateInputRouting()
		return m, nil

	case c.ModalCancelMsg:
		m.isActionMenuActive = false
		m.updateInputRouting()
		return m, nil

	case c.ModalResponseMsg:
		m.isActionMenuActive = false
		m.updateInputRouting()
		if msg.Err != nil {
			slog.Error(fmt.Sprintf("Failed to copy field: %v", msg.Err))
		} else {
			slog.Debug("Field copied to clipboard")
		}
		return m, nil

	case spinner.TickMsg:
		m.loader, cmd = m.loader.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if m.isActionMenuActive {
			m.actionMenu, cmd = m.actionMenu.Update(msg)
			return m, cmd
		}

		switch {
		case key.Matches(msg, c.CopySecret):
			selected := m.list.SelectedItem()
			if selected == nil {
				return m, nil
			}
			item, ok := selected.(SecretItem)
			if !ok {
				return m, nil
			}
			return m, fetchSecretFieldsCmd(m.app.Context, m.app, item)

		case key.Matches(msg, c.ForceRefresh):
			m.isLoading = true
			m.items = nil
			m.pending = 2
			owner := ownerName(m.app.AWS.Identity)
			slog.Debug(fmt.Sprintf("Owner: %s", owner))
			return m, tea.Batch(
				m.loader.Tick,
				fetchAllSecretsCmd(m.app.Context, m.app, owner),
			)

		case key.Matches(msg, c.ToggleFocus):
			m.isViewportFocused = !m.isViewportFocused
			m.updateInputRouting()
			return m, nil
		}
	}

	if m.isViewportFocused {
		m.viewport, cmd = m.viewport.Update(msg)
	} else {
		prevSelected := m.list.Index()
		m.list, cmd = m.list.Update(msg)
		if m.list.Index() != prevSelected {
			m.refreshViewport()
		}
	}

	return m, cmd
}

func (m *SecretsViewModel) updateInputRouting() {
	if m.isActionMenuActive || m.isViewportFocused {
		m.inputRoutingStrategy = common.RouteFocusedFirst
	} else {
		m.inputRoutingStrategy = common.RouteGlobalFirst
	}
}

func (m SecretsViewModel) buildCopyMenu(fields secretFieldsLoadedMsg) c.ActionMenu {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	actions := make([]c.ActionItem, 0, len(keys))
	for _, k := range keys {
		fieldKey := k
		fieldValue := fields[k]
		actions = append(actions, c.ActionItem{
			Label: "Copy " + fieldKey,
			Action: func() tea.Cmd {
				return func() tea.Msg {
					return c.ModalResponseMsg{Err: clipboard.WriteAll(fieldValue)}
				}
			},
		})
	}
	return c.NewActionModal("Secret Value", actions)
}

func (m SecretsViewModel) View() string {
	if m.isLoading {
		return constants.DocStyle.Render(m.loader.View() + "\n")
	}

	listBorderColor := c.UnfocusedColor
	detailBorderColor := c.UnfocusedColor
	if m.isViewportFocused {
		detailBorderColor = c.FocusedColor
	} else {
		listBorderColor = c.FocusedColor
	}

	left := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(listBorderColor).
		Padding(0, 1).
		Width(m.listWidth).
		Height(m.windowSize.Height).
		Render(m.list.View())

	right := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(detailBorderColor).
		Padding(0, 1).
		Width(m.detailsWidth).
		Height(m.windowSize.Height).
		Render(m.viewport.View())

	base := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	if m.isActionMenuActive {
		return overlay.Composite(
			m.actionMenu.View(),
			base,
			overlay.Center,
			overlay.Center,
			0,
			0,
		)
	}

	return base
}

func (m *SecretsViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg
	m.listWidth = int(float64(msg.Width) * secretsListWidthRatio)
	m.detailsWidth = msg.Width - m.listWidth

	viewportWidth := m.detailsWidth - c.BorderWidth
	viewportHeight := msg.Height - c.BorderHeight
	if viewportHeight < 0 {
		viewportHeight = 0
	}

	m.list.SetSize(m.listWidth-c.BorderWidth, msg.Height-c.BorderHeight)
	m.viewport.Width = viewportWidth
	m.viewport.Height = viewportHeight
	m.refreshViewport()
}

func (m *SecretsViewModel) refreshViewport() {
	content := m.renderDetail()
	wrapped := wrap.String(wordwrap.String(content, m.viewport.Width), m.viewport.Width)
	m.viewport.SetContent(wrapped)
	m.viewport.GotoTop()
}

func (m SecretsViewModel) renderDetail() string {
	return generateSecretDetail(m.list.SelectedItem(), m.detailsWidth-c.BorderWidth)
}

func generateSecretDetail(selectedItem list.Item, width int) string {
	if selectedItem == nil {
		return "No secret selected."
	}

	item, ok := selectedItem.(SecretItem)
	if !ok {
		return "No Info"
	}

	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1)
	sectionHeaderStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		PaddingBottom(1)

	var rows []string

	if item.source == sourceSecretsManager {
		s := item.secret
		rows = []string{
			headerStyle.Render("Secrets Manager"),
			"",
			sectionHeaderStyle.Render("General Info"),
			common.KV("Name", s.Name),
			common.KV("ARN", s.ARN),
			common.KV("Description", s.Desc),
			common.KV("Last Changed", s.LastChanged),
			common.KV("Last Accessed", s.LastAccessed),
		}
		if len(s.Tags) > 0 {
			rows = append(rows, "", sectionHeaderStyle.Render("Tags"))
			rows = append(rows, c.GenerateTagRows(s.Tags)...)
		}
	} else {
		p := item.param
		rows = []string{
			headerStyle.Render("Parameter Store"),
			"",
			sectionHeaderStyle.Render("General Info"),
			common.KV("Name", p.Name),
			common.KV("ARN", p.ARN),
			common.KV("Type", p.Type),
			common.KV("Description", p.Desc),
			common.KV("Last Modified", p.LastModified),
		}
		if len(p.Tags) > 0 {
			rows = append(rows, "", sectionHeaderStyle.Render("Tags"))
			rows = append(rows, c.GenerateTagRows(p.Tags)...)
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
