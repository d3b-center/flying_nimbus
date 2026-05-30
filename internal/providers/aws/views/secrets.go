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
	overlay "github.com/rmhubbert/bubbletea-overlay"
)

// secretSource identifies which backend a SecretItem came from.
type (
	secretSource int
	sourceFilter int
)

const (
	filterAll sourceFilter = iota
	filterSecretsManagerOnly
	filterParameterStoreOnly
)

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

func (i SecretItem) FilterValue() string {
	prefix := "[SM]"
	if i.source == sourceParameterStore {
		prefix = "[PS]"
	}
	return prefix + " " + i.Title()
}

// SecretsViewModel renders secrets from Secrets Manager and Parameter Store combined.
type SecretsViewModel struct {
	app                     *app.App
	list                    list.Model
	loader                  spinner.Model
	isLoading               bool
	windowSize              common.ContentWindowSizeMsg
	secretsDetail           string
	isDetailViewportFocused bool
	detailViewport          viewport.Model
	secretsListWidth        int
	secretsDetailsWidth     int
	contentHeight           int
	inputRoutingStrategy    common.InputRoutingStrategy
	actionMenu              c.ActionMenu
	isActionMenuActive      bool
	isInputFormActive       bool
	inputForm               c.InputForm
	pending                 int
	items                   []list.Item
	showAllSecrets          bool
	currentFilter           sourceFilter
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
		detailViewport:       vp,
		inputRoutingStrategy: common.RouteGlobalFirst,
		currentFilter:        filterAll,
	}
	m.updateLayout(windowSize)
	return m
}

func (m SecretsViewModel) Title() string {
	return "Secrets"
}

func (m SecretsViewModel) Commands() common.Commands {
	return []key.Binding{c.ForceRefresh, c.ToggleFocus, c.CopySecret, c.ToggleView, c.ToggleSource}
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
func fetchAllSecretsCmd(ctx context.Context, app *app.App, owner string, showAll bool) tea.Cmd {
	return tea.Batch(
		fetchSecretsManagerCmd(ctx, app.AWS.Secrets, owner, showAll),
		fetchParameterStoreCmd(ctx, app.AWS.ParameterStore, owner, showAll),
	)
}

func fetchSecretsManagerCmd(ctx context.Context, svc *aws.SecretsService, owner string, showAll bool) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			slog.Warn("Secrets Manager service is nil; skipping fetch")
			return secretsLoadedMsg{source: sourceSecretsManager}
		}
		var secrets []aws.Secret
		var err error
		if showAll {
			secrets, err = svc.ListAllSecrets(ctx)
		} else {
			secrets, err = svc.ListSecretsByOwner(ctx, owner)
		}
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

func fetchParameterStoreCmd(ctx context.Context, svc *aws.ParameterStoreService, owner string, showAll bool) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			slog.Warn("Parameter Store service is nil; skipping fetch")
			return secretsLoadedMsg{source: sourceParameterStore}
		}
		var params []aws.Parameter
		var err error
		if showAll {
			params, err = svc.ListAllParameters(ctx)
		} else {
			params, err = svc.ListParametersByOwner(ctx, owner)
		}
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
		fetchAllSecretsCmd(m.app.Context, m.app, owner, m.showAllSecrets),
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
			m.updateTitle()
		}

	case secretFieldsLoadedMsg:
		m.updateSecretDetails()
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
		cmd = m.handleKeypress(msg)
		return m, cmd
	}

	return m, cmd
}

func (m *SecretsViewModel) updateTitle() {
	ownerPart := "My Secrets"
	if m.showAllSecrets {
		ownerPart = "All Secrets"
	}

	sourcePart := ""
	switch m.currentFilter {
	case filterSecretsManagerOnly:
		sourcePart = " (SM only)"
	case filterParameterStoreOnly:
		sourcePart = " (PS only)"
	}

	m.list.Title = ownerPart + sourcePart
}

func (m *SecretsViewModel) updateInputRouting() {
	if m.isActionMenuActive || m.isDetailViewportFocused {
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

func (m SecretsViewModel) handleOverlays(instances string) string {
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

func (m *SecretsViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg

	usableWidth := msg.Width - c.BorderWidth
	usableHeight := msg.Height - c.BorderHeight

	m.secretsListWidth = int(float64(usableWidth) * c.SecretsListWidthRatio)
	m.secretsDetailsWidth = msg.Width - int(float64(m.secretsListWidth)*c.SecretsListWidthRatio)

	m.contentHeight = usableHeight
	// TODO: Hack-y way to adjust the detailViewPort width. Not sure why it is so wonky. This needs to be fixed, but the fix could be redesigning how we calculate widths. We have the same issue in ServiceCatalog
	m.detailViewport.Width = m.secretsDetailsWidth - 28*c.BorderWidth
	m.detailViewport.Height = msg.Height

	m.list.SetSize(m.secretsDetailsWidth, usableHeight)
}

func (m *SecretsViewModel) updateSecretDetails() {
	m.secretsDetail = generateSecretDetails(m.list.SelectedItem())
	m.detailViewport.SetContent(m.secretsDetail)
	m.detailViewport.GotoTop()
}

func generateSecretDetails(selectedItem list.Item) string {
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

// handleKeypress processes keyboard input.
func (m *SecretsViewModel) handleKeypress(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	if m.isActionMenuActive {
		m.actionMenu, cmd = m.actionMenu.Update(msg)
		return cmd
	}

	switch {
	case key.Matches(msg, c.CopySecret):
		selected := m.list.SelectedItem()
		if selected == nil {
			return nil
		}
		item, ok := selected.(SecretItem)
		if !ok {
			return nil
		}
		return fetchSecretFieldsCmd(m.app.Context, m.app, item)

	case key.Matches(msg, c.ForceRefresh):
		m.isLoading = true
		m.items = nil
		m.pending = 2
		owner := ownerName(m.app.AWS.Identity)
		slog.Debug(fmt.Sprintf("Owner: %s", owner))
		return tea.Batch(
			m.loader.Tick,
			fetchAllSecretsCmd(m.app.Context, m.app, owner, m.showAllSecrets),
		)

	case key.Matches(msg, c.ToggleFocus):
		m.isDetailViewportFocused = !m.isDetailViewportFocused
		m.updateInputRouting()
		return nil

	case key.Matches(msg, c.ToggleSource):
		switch m.currentFilter {
		case filterAll:
			m.currentFilter = filterSecretsManagerOnly
			m.list.SetFilterText("[SM]")
		case filterSecretsManagerOnly:
			m.currentFilter = filterParameterStoreOnly
			m.list.SetFilterText("[PS]")
		case filterParameterStoreOnly:
			m.currentFilter = filterAll
			m.list.ResetFilter()
		}
		m.updateTitle()
		return nil

	case key.Matches(msg, c.ToggleView):
		m.showAllSecrets = !m.showAllSecrets
		m.isLoading = true
		m.items = nil
		m.pending = 2
		owner := ownerName(m.app.AWS.Identity)
		m.updateTitle()
		return tea.Batch(
			m.loader.Tick,
			fetchAllSecretsCmd(m.app.Context, m.app, owner, m.showAllSecrets),
		)
	}

	if m.isDetailViewportFocused {
		m.detailViewport, cmd = m.detailViewport.Update(msg)
	} else {
		m.list, cmd = m.list.Update(msg)
		m.updateSecretDetails()
	}
	return cmd
}
