package views

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"flying_nimbus/internal/app"
	aws "flying_nimbus/internal/providers/aws/backend"
	c "flying_nimbus/internal/providers/aws/views/components"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	overlay "github.com/rmhubbert/bubbletea-overlay"
)

const serviceCatalogListWidthRatio = 0.25

var (
	keySwitchToProvisioned = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view provisioned"))
	keySwitchToCatalog     = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "catalog"))
	keyRefresh             = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
	keyLaunch              = key.NewBinding(key.WithKeys("p", "enter"), key.WithHelp("p/enter", "launch"))
)

// ServiceCatalogViewModel renders Service Catalog: catalog products and provisioned products (two modes).
type ServiceCatalogViewModel struct {
	app                  *app.App
	list                 list.Model
	loader               spinner.Model
	catalogMode          bool // true = catalog products, false = provisioned products
	isLoadingCatalog     bool
	isLoadingProvisioned bool
	catalogItems         []list.Item
	provisionedItems     []list.Item
	windowSize           common.ContentWindowSizeMsg
	listWidth            int
	detailsWidth         int
	inputRoutingStrategy common.InputRoutingStrategy

	// Launch flow state.
	launchState     int
	launchProduct   aws.Product
	launchArtifacts []aws.ProvisioningArtifact
	launchArtifact  aws.ProvisioningArtifact
	launchParams    []aws.ProvisioningParameter
	artifactList    list.Model
	launchSpinner   spinner.Model

	// Shared components: action menu and input form (provision form or SSM port forward).
	actionMenu         c.ActionMenu
	isActionMenuActive bool
	inputForm          c.InputForm
	isInputFormActive  bool
}

// Launch flow states.
const (
	launchIdle = iota
	launchLoadingArtifacts
	launchSelectingVersion
	launchLoadingParams
	launchShowingForm
	launchProvisioning
)

type (
	provisionedProductsLoadedMsg []list.Item
	catalogProductsLoadedMsg     []list.Item
	artifactsLoadedMsg           struct{ artifacts []aws.ProvisioningArtifact }
	paramsLoadedMsg              struct{ params []aws.ProvisioningParameter }
	provisionDoneMsg             struct {
		recordID string
		err      error
	}
)

// artifactItem adapts ProvisioningArtifact for list.Item.
type artifactItem struct{ aws.ProvisioningArtifact }

func (a artifactItem) Title() string {
	if a.Name != "" {
		return a.Name
	}
	return a.ID
}

func (a artifactItem) Description() string {
	if a.ProvisioningArtifact.Description != "" {
		return a.ProvisioningArtifact.Description
	}
	return a.ProvisioningArtifact.CreatedTime.Format("2006-01-02")
}

func (a artifactItem) FilterValue() string { return a.Name }

// InitServiceCatalogViewModel builds a ServiceCatalogViewModel with default layout (catalog mode by default).
func InitServiceCatalogViewModel(appService *app.App, windowSize common.ContentWindowSizeMsg) ServiceCatalogViewModel {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	loader := spinner.New()
	loader.Style = spinnerStyle
	loader.Spinner = spinner.Dot

	m := ServiceCatalogViewModel{
		app:                  appService,
		list:                 l,
		loader:               loader,
		catalogMode:          true,
		isLoadingCatalog:     true,
		isLoadingProvisioned: false,
		windowSize:           windowSize,
	}
	m.setListTitle()
	m.updateLayout(windowSize)

	return m
}

func fetchProvisionedProductsCmd(ctx context.Context, svc *aws.ServiceCatalogService) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			slog.Warn("Service Catalog service is nil; skipping provisioned fetch")
			return provisionedProductsLoadedMsg([]list.Item{})
		}
		if ctx == nil {
			ctx = context.Background()
		}
		products, _ := svc.ListProvisionedProducts(ctx)
		return provisionedProductsLoadedMsg(provisionedProductsToItems(products))
	}
}

func fetchCatalogProductsCmd(ctx context.Context, svc *aws.ServiceCatalogService) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			slog.Warn("Service Catalog service is nil; skipping catalog fetch")
			return catalogProductsLoadedMsg([]list.Item{})
		}
		if ctx == nil {
			ctx = context.Background()
		}
		products, _ := svc.ListCatalogProducts(ctx)
		return catalogProductsLoadedMsg(catalogProductsToItems(products))
	}
}

func catalogProductsToItems(products []aws.Product) []list.Item {
	out := make([]list.Item, len(products))
	for i, p := range products {
		out[i] = list.Item(p)
	}
	return out
}

func (m *ServiceCatalogViewModel) setListTitle() {
	if m.catalogMode {
		m.list.Title = "Catalog Products"
	} else {
		m.list.Title = "Provisioned Products"
	}
}

func fetchArtifactsCmd(ctx context.Context, svc *aws.ServiceCatalogService, productID string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil || productID == "" {
			return artifactsLoadedMsg{artifacts: nil}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		artifacts, err := svc.ListProvisioningArtifacts(ctx, productID)
		if err != nil {
			slog.Warn("ListProvisioningArtifacts failed", "err", err)
			return artifactsLoadedMsg{artifacts: nil}
		}
		return artifactsLoadedMsg{artifacts: artifacts}
	}
}

func fetchParamsCmd(ctx context.Context, svc *aws.ServiceCatalogService, productID, artifactID string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil || productID == "" || artifactID == "" {
			return paramsLoadedMsg{params: nil}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		params, err := svc.DescribeProvisioningParameters(ctx, productID, artifactID)
		if err != nil {
			slog.Warn("DescribeProvisioningParameters failed", "err", err)
			return paramsLoadedMsg{params: nil}
		}
		return paramsLoadedMsg{params: params}
	}
}

func provisionProductCmd(ctx context.Context, svc *aws.ServiceCatalogService, productID, artifactID, name string, params map[string]string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return provisionDoneMsg{err: fmt.Errorf("service not initialized")}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		recordID, err := svc.ProvisionProduct(ctx, productID, artifactID, name, params)
		if err != nil {
			return provisionDoneMsg{recordID: "", err: err}
		}
		return provisionDoneMsg{recordID: recordID, err: nil}
	}
}

func artifactsToItems(artifacts []aws.ProvisioningArtifact) []list.Item {
	out := make([]list.Item, len(artifacts))
	for i, a := range artifacts {
		out[i] = artifactItem{a}
	}
	return out
}

func newArtifactList(artifacts []aws.ProvisioningArtifact, productName string) list.Model {
	l := list.New(artifactsToItems(artifacts), list.NewDefaultDelegate(), 20, 10)
	l.Title = "Select version - " + productName
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	return l
}

func (m ServiceCatalogViewModel) Init() tea.Cmd {
	slog.Debug("Initialize Service Catalog Model")
	if m.app == nil || m.app.AWS == nil || m.app.AWS.ServiceCatalog == nil {
		slog.Warn("Service Catalog is not initialized; loading empty lists")
		if m.catalogMode {
			return tea.Batch(m.loader.Tick, func() tea.Msg { return catalogProductsLoadedMsg([]list.Item{}) })
		}
		return tea.Batch(m.loader.Tick, func() tea.Msg { return provisionedProductsLoadedMsg([]list.Item{}) })
	}
	if m.catalogMode {
		return tea.Batch(m.loader.Tick, fetchCatalogProductsCmd(m.app.Context, m.app.AWS.ServiceCatalog))
	}
	return tea.Batch(m.loader.Tick, fetchProvisionedProductsCmd(m.app.Context, m.app.AWS.ServiceCatalog))
}

func (m ServiceCatalogViewModel) View() string {
	if m.isLoading() {
		return constants.DocStyle.Render(m.loader.View() + "\n")
	}

	left := instancesListStyle.Width(m.listWidth).MaxHeight(m.windowSize.Height).Render(m.list.View())
	right := instanceDetailStyle.Width(m.detailsWidth).MaxHeight(m.windowSize.Height).Render(m.renderDetail())
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	if content, ok := m.handleOverlay(); ok {
		return overlay.Composite(content, main, overlay.Center, overlay.Center, 0, 0)
	}
	return main
}

func (m ServiceCatalogViewModel) handleOverlay() (string, bool) {
	if m.launchState != launchIdle {
		return m.renderLaunchOverlay(), true
	}
	if m.isActionMenuActive {
		return m.actionMenu.View(), true
	}
	if m.isInputFormActive {
		return m.inputForm.View(), true
	}
	return "", false
}

func (m ServiceCatalogViewModel) renderLaunchOverlay() string {
	switch m.launchState {
	case launchLoadingArtifacts, launchLoadingParams, launchProvisioning:
		title := "Loading..."
		if m.launchState == launchProvisioning {
			title = "Provisioning..."
		}
		return c.ModalOverlayStyle.Render(lipgloss.JoinVertical(lipgloss.Center, c.ModalTitleStyle.Render(title), m.launchSpinner.View()))
	case launchSelectingVersion:
		return c.ModalOverlayStyle.Render(m.artifactList.View())
	case launchShowingForm:
		return m.inputForm.View()
	default:
		return ""
	}
}

func (m *ServiceCatalogViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		m.windowSize = msg
		m.updateLayout(msg)
	case catalogProductsLoadedMsg:
		m.isLoadingCatalog = false
		m.catalogItems = msg
		if m.catalogMode {
			m.list.SetItems(msg)
		}
		slog.Debug(fmt.Sprintf("📥 Rcv'd Catalog Products... # %d", len(msg)))
	case provisionedProductsLoadedMsg:
		m.isLoadingProvisioned = false
		m.provisionedItems = msg
		if !m.catalogMode {
			m.list.SetItems(msg)
		}
		slog.Debug(fmt.Sprintf("📥 Rcv'd Provisioned Products... # %d", len(msg)))
	case spinner.TickMsg:
		m.loader, cmd = m.loader.Update(msg)
	case artifactsLoadedMsg:
		m.launchState = launchIdle
		if len(msg.artifacts) == 0 {
			cmd = tea.Batch(cmd, func() tea.Msg { return c.ModalResponseMsg{Err: fmt.Errorf("no versions available")} })
			break
		}
		m.launchArtifacts = msg.artifacts
		m.artifactList = newArtifactList(msg.artifacts, m.launchProduct.ProductName)
		m.launchState = launchSelectingVersion
	case paramsLoadedMsg:
		if msg.params == nil {
			m.launchState = launchIdle
			cmd = tea.Batch(cmd, func() tea.Msg { return c.ModalResponseMsg{Err: fmt.Errorf("failed to load parameters")} })
			break
		}
		m.launchParams = msg.params
		fields := m.provisionFormFields()
		m.inputForm = c.NewInputForm("Provision: "+m.launchProduct.ProductName, fields, m.provisionOnSubmit)
		m.isInputFormActive = true
		m.launchState = launchShowingForm
	case provisionDoneMsg:
		m.launchState = launchIdle
		if msg.err != nil {
			cmd = tea.Batch(cmd, func() tea.Msg { return c.ModalResponseMsg{Err: msg.err} })
		} else {
			slog.Info("Provisioning started", "recordID", msg.recordID)
			cmd = tea.Batch(cmd, func() tea.Msg { return c.ModalResponseMsg{Err: nil} })
			// Refresh provisioned list when we switch to it
			m.provisionedItems = nil
		}
	case c.ModalCancelMsg:
		m.launchState = launchIdle
		m.isActionMenuActive = false
		m.isInputFormActive = false
		return m, cmd
	case c.InputFormSubmitMsg:
		m.isInputFormActive = false
		return m, msg.OnSubmit(msg.Values)
	case c.InputFormCancelMsg:
		m.isInputFormActive = false
		if m.launchState == launchShowingForm {
			m.launchState = launchIdle
		} else {
			m.isActionMenuActive = true
		}
		return m, nil
	case c.InputFormOpenMsg:
		m.isInputFormActive = true
		m.isActionMenuActive = false
		prod := m.list.SelectedItem().(aws.ProvisionedProduct)
		m.inputForm = c.NewInputForm(
			"Port Forward: "+prod.Name,
			m.ssmPortForwardInputs(),
			m.ssmPortForwardOnSubmit,
		)
		return m, nil
	case c.ModalResponseMsg:
		if m.launchState != launchIdle {
			m.launchState = launchIdle
		}
		m.isActionMenuActive = false
		if msg.Err != nil {
			slog.Error("Service catalog action error", "error", msg.Err)
		} else if !m.catalogMode && m.app != nil && m.app.AWS != nil && m.app.AWS.ServiceCatalog != nil {
			cmd = tea.Batch(cmd, fetchProvisionedProductsCmd(m.app.Context, m.app.AWS.ServiceCatalog))
		}
		return m, cmd
	}

	if dm, dCmd, handled := m.delegateToSubmodel(msg); handled {
		return dm, dCmd
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if newM, keyCmd, handled := m.handleKeyWhenNotLoading(keyMsg); handled {
			return newM, keyCmd
		}
	}

	m.list, cmd = m.list.Update(msg)
	m.updateInputRouting()

	return m, cmd
}

func (m *ServiceCatalogViewModel) delegateToSubmodel(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	var cmd tea.Cmd
	// Provisioned product action menu consumes input when active
	if m.isActionMenuActive {
		m.actionMenu, cmd = m.actionMenu.Update(msg)
		m.updateInputRouting()
		return m, cmd, true
	}

	// Port forward form (provisioned product) consumes input when active
	if m.isInputFormActive && m.launchState == launchIdle {
		m.inputForm, cmd = m.inputForm.Update(msg)
		m.updateInputRouting()
		return m, cmd, true
	}

	// Launch flow: version selector or form consumes input
	if m.launchState == launchSelectingVersion {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if cmd, handled := m.handleArtifactListKey(keyMsg); handled {
				return m, cmd, true
			}
		}
		m.artifactList, cmd = m.artifactList.Update(msg)
		m.updateInputRouting()
		return m, cmd, true
	}
	if m.launchState == launchShowingForm && m.isInputFormActive {
		m.inputForm, cmd = m.inputForm.Update(msg)
		m.updateInputRouting()
		return m, cmd, true
	}
	if m.launchState == launchLoadingArtifacts || m.launchState == launchLoadingParams || m.launchState == launchProvisioning {
		m.launchSpinner, cmd = m.launchSpinner.Update(msg)
		m.updateInputRouting()
		return m, cmd, true
	}

	return nil, nil, false
}

func (m *ServiceCatalogViewModel) updateInputRouting() {
	m.inputRoutingStrategy = common.RouteGlobalFirst
	if m.list.FilterState() == list.Filtering ||
		m.launchState != launchIdle ||
		m.isActionMenuActive ||
		m.isInputFormActive {
		m.inputRoutingStrategy = common.RouteFocusedFirst
	}
}

func (m *ServiceCatalogViewModel) handleArtifactListKey(keyMsg tea.KeyMsg) (tea.Cmd, bool) {
	if key.Matches(keyMsg, constants.Keymap.Back) {
		m.launchState = launchIdle
		return nil, true
	}
	if key.Matches(keyMsg, constants.Keymap.Enter) {
		sel := m.artifactList.SelectedItem()
		if sel != nil {
			if a, ok := sel.(artifactItem); ok {
				m.launchArtifact = a.ProvisioningArtifact
				m.launchState = launchLoadingParams
				m.launchSpinner = spinner.New()
				m.launchSpinner.Style = spinnerStyle
				m.launchSpinner.Spinner = spinner.Dot
				return tea.Batch(m.launchSpinner.Tick, fetchParamsCmd(m.app.Context, m.app.AWS.ServiceCatalog, m.launchProduct.ProductID, m.launchArtifact.ID)), true
			}
		}
	}
	return nil, false
}

// openProvisionedProductActionMenu opens the action menu for the selected provisioned product.
// Returns true if the selection was a provisioned product and the menu was opened.
func (m *ServiceCatalogViewModel) openProvisionedProductActionMenu() bool {
	sel := m.list.SelectedItem()
	if _, ok := sel.(aws.ProvisionedProduct); !ok {
		return false
	}
	m.isActionMenuActive = true
	m.buildProvisionedActions()
	m.updateInputRouting()
	return true
}

// startLaunchFromCatalog starts the launch (provision) flow for the selected catalog product.
// Returns (m, cmd, true) when the selection is a product and the flow was started.
func (m *ServiceCatalogViewModel) startLaunchFromCatalog() (tea.Model, tea.Cmd, bool) {
	sel := m.list.SelectedItem()
	prod, ok := sel.(aws.Product)
	if !ok {
		return m, nil, false
	}
	m.launchState = launchLoadingArtifacts
	m.launchProduct = prod
	m.launchSpinner = spinner.New()
	m.launchSpinner.Style = spinnerStyle
	m.launchSpinner.Spinner = spinner.Dot
	cmd := tea.Batch(m.launchSpinner.Tick, fetchArtifactsCmd(m.app.Context, m.app.AWS.ServiceCatalog, prod.ProductID))
	return m, cmd, true
}

// switchToProvisionedView switches list to provisioned products and fetches if empty.
func (m *ServiceCatalogViewModel) switchToProvisionedView() tea.Cmd {
	m.catalogMode = false
	m.setListTitle()
	m.list.SetItems(m.provisionedItems)
	if len(m.provisionedItems) == 0 {
		m.isLoadingProvisioned = true
		return tea.Batch(m.loader.Tick, fetchProvisionedProductsCmd(m.app.Context, m.app.AWS.ServiceCatalog))
	}
	return m.loader.Tick
}

// switchToCatalogView switches list to catalog products and fetches if empty.
func (m *ServiceCatalogViewModel) switchToCatalogView() tea.Cmd {
	m.catalogMode = true
	m.setListTitle()
	m.list.SetItems(m.catalogItems)
	if len(m.catalogItems) == 0 {
		m.isLoadingCatalog = true
		return tea.Batch(m.loader.Tick, fetchCatalogProductsCmd(m.app.Context, m.app.AWS.ServiceCatalog))
	}
	return m.loader.Tick
}

// refreshCurrentList sets loading state and returns the fetch command for the current mode.
func (m *ServiceCatalogViewModel) refreshCurrentList() tea.Cmd {
	if m.catalogMode {
		m.isLoadingCatalog = true
		return tea.Batch(m.loader.Tick, fetchCatalogProductsCmd(m.app.Context, m.app.AWS.ServiceCatalog))
	}
	m.isLoadingProvisioned = true
	return tea.Batch(m.loader.Tick, fetchProvisionedProductsCmd(m.app.Context, m.app.AWS.ServiceCatalog))
}

// handleKeyWhenNotLoading handles key messages when not loading. Caller must ensure !m.isLoading().
// Returns (model, cmd, true) when the key was handled so Update can return early.
// Key handling: v = view provisioned, c = catalog, r = refresh, Enter = launch (catalog) or actions (provisioned)
func (m *ServiceCatalogViewModel) handleKeyWhenNotLoading(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.isLoading() {
		return nil, nil, false
	}

	if m.catalogMode {
		if m.launchState == launchIdle {
			if key.Matches(keyMsg, keyLaunch) {
				if newM, cmd, ok := m.startLaunchFromCatalog(); ok {
					return newM, cmd, true
				}
			}
		}
		if key.Matches(keyMsg, keySwitchToProvisioned) {
			return m, m.switchToProvisionedView(), true
		}
	} else {
		if m.launchState == launchIdle && !m.isActionMenuActive {
			if key.Matches(keyMsg, constants.Keymap.Enter) {
				if m.openProvisionedProductActionMenu() {
					return m, nil, true
				}
			}
		}
		if key.Matches(keyMsg, keySwitchToCatalog) {
			return m, m.switchToCatalogView(), true
		}
	}
	if key.Matches(keyMsg, keyRefresh) {
		return m, m.refreshCurrentList(), true
	}
	return nil, nil, false
}

func (m *ServiceCatalogViewModel) buildProvisionedActions() {
	m.actionMenu = c.NewActionModal("Provisioned Product", []c.ActionItem{
		{Label: "Shell", Action: m.ssmShell},
		{Label: "Port Forward", Action: m.ssmPortForward},
		{Label: "Start", Action: m.startProvisioned},
		{Label: "Stop", Action: m.stopProvisioned},
		{Label: "Terminate", Action: m.terminateProvisioned},
	})
}

func (m *ServiceCatalogViewModel) startProvisioned() tea.Cmd {
	return runStartStopProvisioned(m.app, m.list.SelectedItem(), true)
}

func (m *ServiceCatalogViewModel) stopProvisioned() tea.Cmd {
	return runStartStopProvisioned(m.app, m.list.SelectedItem(), false)
}

func (m *ServiceCatalogViewModel) terminateProvisioned() tea.Cmd {
	return runTerminateProvisioned(m.app, m.list.SelectedItem())
}

// provisionFormFields builds InputField slice for the provision form (name + catalog params).
// Fields use Placeholder to show defaults; main's InputField has no Value/Width.
func (m *ServiceCatalogViewModel) provisionFormFields() []c.InputField {
	fields := []c.InputField{
		{
			Label:       "Provisioned product name",
			Placeholder: m.launchProduct.ProductName,
			CharLimit:   128,
		},
	}
	for _, p := range m.launchParams {
		field := c.InputField{
			Label:     p.Key,
			CharLimit: 128,
			Required:  p.Required,
		}
		if len(p.AllowedValues) > 0 {
			field.Options = p.AllowedValues
		} else if p.DefaultValue != "" {
			field.Value = p.DefaultValue
		}
		fields = append(fields, field)
	}
	return fields
}

// provisionOnSubmit validates form values and runs provision; used as InputForm onSubmit callback.
func (m *ServiceCatalogViewModel) provisionOnSubmit(values c.InputFormResult) tea.Cmd {
	name := values["Provisioned product name"]
	if name == "" {
		return func() tea.Msg { return c.ModalResponseMsg{Err: fmt.Errorf("provisioned product name is required")} }
	}
	params := make(map[string]string)
	for _, p := range m.launchParams {
		if v, ok := values[p.Key]; ok {
			params[p.Key] = v
		}
	}
	m.launchState = launchProvisioning
	m.launchSpinner = spinner.New()
	m.launchSpinner.Style = spinnerStyle
	m.launchSpinner.Spinner = spinner.Dot
	return tea.Batch(
		m.launchSpinner.Tick,
		provisionProductCmd(m.app.Context, m.app.AWS.ServiceCatalog, m.launchProduct.ProductID, m.launchArtifact.ID, name, params),
	)
}

func (m *ServiceCatalogViewModel) validateSsmProvisionedProduct() (aws.ProvisionedProduct, string, error) {
	sel := m.list.SelectedItem()
	if sel == nil {
		return aws.ProvisionedProduct{}, "", fmt.Errorf("no provisioned product selected")
	}
	prod, ok := sel.(aws.ProvisionedProduct)
	if !ok {
		return aws.ProvisionedProduct{}, "", fmt.Errorf("selected item is not a provisioned product")
	}
	recordID := prod.LastSuccessfulRecordID
	if recordID == "" {
		return aws.ProvisionedProduct{}, "", fmt.Errorf("no provisioning record for this product")
	}
	return prod, recordID, nil
}

func (m *ServiceCatalogViewModel) ssmShell() tea.Cmd {
	prod, recordID, err := m.validateSsmProvisionedProduct()
	if err != nil {
		return func() tea.Msg { return c.ModalResponseMsg{Err: err} }
	}
	instanceIDs, err := m.app.AWS.ServiceCatalog.InstanceIDsForProvisionedProduct(m.app.Context, recordID)
	if err != nil {
		return func() tea.Msg { return c.ModalResponseMsg{Err: err} }
	}
	if len(instanceIDs) == 0 {
		return func() tea.Msg { return c.ModalResponseMsg{Err: fmt.Errorf("no EC2 instances found for this product")} }
	}
	instanceID := instanceIDs[0]
	if len(instanceIDs) > 1 {
		slog.Debug("Multiple instances for provisioned product, using first", "instanceID", instanceID, "product", prod.Name)
	}
	command := m.app.AWS.Ssm.BuildSessionCmd(instanceID)
	return tea.ExecProcess(command, func(err error) tea.Msg {
		return c.ModalResponseMsg{Err: err}
	})
}

func (m *ServiceCatalogViewModel) ssmPortForward() tea.Cmd {
	_, _, err := m.validateSsmProvisionedProduct()
	if err != nil {
		return func() tea.Msg { return c.ModalResponseMsg{Err: err} }
	}
	m.isInputFormActive = true
	m.isActionMenuActive = false
	return func() tea.Msg { return c.InputFormOpenMsg{} }
}

func (m *ServiceCatalogViewModel) ssmPortForwardInputs() []c.InputField {
	return []c.InputField{
		{Label: "Local Port", Placeholder: "8080", CharLimit: 5, Validator: aws.ValidatePort},
		{Label: "Remote Port", Placeholder: "8080", CharLimit: 5, Validator: aws.ValidatePort},
	}
}

func (m *ServiceCatalogViewModel) ssmPortForwardOnSubmit(values c.InputFormResult) tea.Cmd {
	_, recordID, err := m.validateSsmProvisionedProduct()
	if err != nil {
		return func() tea.Msg { return c.ModalResponseMsg{Err: err} }
	}
	if err := aws.ValidatePort(values["Local Port"]); err != nil {
		return func() tea.Msg { return c.ModalResponseMsg{Err: fmt.Errorf("invalid local port: %w", err)} }
	}
	if err := aws.ValidatePort(values["Remote Port"]); err != nil {
		return func() tea.Msg { return c.ModalResponseMsg{Err: fmt.Errorf("invalid remote port: %w", err)} }
	}
	localPort, _ := strconv.Atoi(values["Local Port"])
	remotePort, _ := strconv.Atoi(values["Remote Port"])
	instanceIDs, err := m.app.AWS.ServiceCatalog.InstanceIDsForProvisionedProduct(m.app.Context, recordID)
	if err != nil {
		return func() tea.Msg { return c.ModalResponseMsg{Err: err} }
	}
	if len(instanceIDs) == 0 {
		return func() tea.Msg { return c.ModalResponseMsg{Err: fmt.Errorf("no EC2 instances found for this product")} }
	}
	instanceID := instanceIDs[0]
	config := aws.PortForwardConfig{LocalPort: localPort, RemotePort: remotePort}
	cmd := m.app.AWS.Ssm.BuildPortForwardCmd(instanceID, config)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return c.ModalResponseMsg{Err: err}
	})
}

func runTerminateProvisioned(app *app.App, selected list.Item) tea.Cmd {
	return func() tea.Msg {
		if app == nil || app.AWS == nil || app.AWS.ServiceCatalog == nil {
			return c.ModalResponseMsg{Err: fmt.Errorf("service catalog not initialized")}
		}
		prod, ok := selected.(aws.ProvisionedProduct)
		if !ok || selected == nil {
			return c.ModalResponseMsg{Err: fmt.Errorf("no provisioned product selected")}
		}
		ctx := app.Context
		if ctx == nil {
			ctx = context.Background()
		}
		err := app.AWS.ServiceCatalog.TerminateProvisionedProduct(ctx, prod.ID)
		if err != nil {
			return c.ModalResponseMsg{Err: err}
		}
		return c.ModalResponseMsg{Err: nil}
	}
}

func runStartStopProvisioned(app *app.App, selected list.Item, start bool) tea.Cmd {
	return func() tea.Msg {
		if app == nil || app.AWS == nil || app.AWS.ServiceCatalog == nil {
			return c.ModalResponseMsg{Err: fmt.Errorf("service catalog not initialized")}
		}
		prod, ok := selected.(aws.ProvisionedProduct)
		if !ok || selected == nil {
			return c.ModalResponseMsg{Err: fmt.Errorf("no provisioned product selected")}
		}
		recordID := prod.LastSuccessfulRecordID
		if recordID == "" {
			return c.ModalResponseMsg{Err: fmt.Errorf("no provisioning record for this product")}
		}
		ctx := app.Context
		if ctx == nil {
			ctx = context.Background()
		}
		var err error
		if start {
			err = app.AWS.ServiceCatalog.StartProvisionedProduct(ctx, prod.ID, recordID)
		} else {
			err = app.AWS.ServiceCatalog.StopProvisionedProduct(ctx, prod.ID, recordID)
		}
		if err != nil {
			return c.ModalResponseMsg{Err: err}
		}
		return c.ModalResponseMsg{Err: nil}
	}
}

func (m ServiceCatalogViewModel) Title() string {
	return "Service Catalog"
}

func (m ServiceCatalogViewModel) Commands() common.Commands {
	bindings := []key.Binding{keySwitchToProvisioned, keySwitchToCatalog, keyRefresh}
	if m.catalogMode {
		bindings = append(bindings, keyLaunch)
	}
	return bindings
}

func (m ServiceCatalogViewModel) InputRoutingStrategy() common.InputRoutingStrategy {
	return m.inputRoutingStrategy
}

func (m *ServiceCatalogViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg
	m.listWidth = int(float64(msg.Width) * serviceCatalogListWidthRatio)
	m.detailsWidth = msg.Width - m.listWidth
	if msg.Height < 0 {
		msg.Height = 0
	}
	m.list.SetSize(m.listWidth, msg.Height)
}

func (m ServiceCatalogViewModel) renderDetail() string {
	if m.catalogMode {
		return generateCatalogProductDetail(m.list.SelectedItem())
	}
	return generateProvisionedProductDetail(m.list.SelectedItem())
}

func (m ServiceCatalogViewModel) isLoading() bool {
	if m.catalogMode {
		return m.isLoadingCatalog
	}
	return m.isLoadingProvisioned
}

func provisionedProductsToItems(items []aws.ProvisionedProduct) []list.Item {
	out := make([]list.Item, len(items))
	for i, item := range items {
		out[i] = list.Item(item)
	}
	return out
}

func generateCatalogProductDetail(selectedItem list.Item) string {
	if selectedItem == nil {
		return "Select a product to view details.\n\np / Enter: Launch (provision)\nv: View provisioned products\nr: Refresh"
	}
	product, ok := selectedItem.(aws.Product)
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

	rows := []string{
		headerStyle.Render("Catalog Product Details"),
		"",
		sectionHeaderStyle.Render("General"),
		common.KV("Name", product.ProductName),
		common.KV("Product ID", product.ProductID),
		common.KV("Type", product.ProductType),
		common.KV("Owner", product.Owner),
		common.KV("Distributor", formatOptionalValue(product.Distributor)),
		"",
		sectionHeaderStyle.Render("Description"),
		formatOptionalValue(product.ShortDescription),
		"",
		sectionHeaderStyle.Render("Support"),
		formatOptionalValue(product.SupportDescription),
		"",
		sectionHeaderStyle.Render("Actions"),
		"  p / Enter  Launch (provision) this product",
		"  v          View provisioned products",
		"  r          Refresh list",
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func generateProvisionedProductDetail(selectedItem list.Item) string {
	if selectedItem == nil {
		return "No Info"
	}

	product, ok := selectedItem.(aws.ProvisionedProduct)
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

	rows := []string{
		headerStyle.Render("Provisioned Product Details"),
		"",
		sectionHeaderStyle.Render("General Info"),
		common.KV("Name", product.Name),
		common.KV("Provisioned ID", product.ID),
		common.KV("Product ID", product.ProductID),
		common.KV("Type", product.Type),
		common.KV("Status", product.Status),
		common.KV("Status Message", formatOptionalValue(product.StatusMessage)),
		common.KV("Created", formatTime(product.CreatedTime)),
		common.KV("ARN", product.Arn),
		"",
		sectionHeaderStyle.Render("Provisioning"),
		common.KV("Artifact ID", product.ProvisioningArtifactID),
		common.KV("Last Record ID", product.LastRecordID),
		common.KV("Last Success Record", product.LastSuccessfulRecordID),
		"",
		sectionHeaderStyle.Render("Actions"),
		"  Enter  Start / Stop (underlying EC2)",
		"  r      Refresh list",
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func formatOptionalValue(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
