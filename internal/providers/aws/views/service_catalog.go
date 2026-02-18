package views

import (
	"context"
	"flying_nimbus/internal/app"
	aws "flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const serviceCatalogListWidthRatio = 0.25

// ServiceCatalogViewModel renders Service Catalog provisioned products.
type ServiceCatalogViewModel struct {
	app                  *app.App
	provisionedList      list.Model
	loader               spinner.Model
	isLoadingProvisioned bool
	windowSize           common.ContentWindowSizeMsg
	listWidth            int
	detailsWidth         int
}

type (
	provisionedProductsLoadedMsg []list.Item
)

// InitServiceCatalogViewModel builds a ServiceCatalogViewModel with default layout.
func InitServiceCatalogViewModel(appService *app.App, windowSize common.ContentWindowSizeMsg) ServiceCatalogViewModel {
	provisioned := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	provisioned.Title = "Provisioned Products"
	provisioned.SetShowTitle(true)
	provisioned.SetShowStatusBar(false)
	provisioned.SetShowHelp(false)

	loader := spinner.New()
	loader.Style = spinnerStyle
	loader.Spinner = spinner.Dot

	m := ServiceCatalogViewModel{
		app:                  appService,
		provisionedList:      provisioned,
		loader:               loader,
		isLoadingProvisioned: true,
		windowSize:           windowSize,
	}
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

func (m ServiceCatalogViewModel) Init() tea.Cmd {
	slog.Debug("Initialize Service Catalog Model")
	if m.app == nil || m.app.AWS == nil || m.app.AWS.ServiceCatalog == nil {
		slog.Warn("Service Catalog is not initialized; loading empty lists")
		return tea.Batch(
			m.loader.Tick,
			func() tea.Msg { return provisionedProductsLoadedMsg([]list.Item{}) },
		)
	}
	return tea.Batch(
		m.loader.Tick,
		fetchProvisionedProductsCmd(m.app.Context, m.app.AWS.ServiceCatalog),
	)
}

func (m ServiceCatalogViewModel) View() string {
	if m.isLoading() {
		return constants.DocStyle.Render(m.loader.View() + "\n")
	}

	left := instancesListStyle.Width(m.listWidth).Height(m.windowSize.Height).Render(m.provisionedList.View())
	right := instanceDetailStyle.Width(m.detailsWidth).Height(m.windowSize.Height).Render(m.renderDetail())

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m ServiceCatalogViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		m.windowSize = msg
		m.updateLayout(msg)
	case provisionedProductsLoadedMsg:
		m.isLoadingProvisioned = false
		m.provisionedList.SetItems(msg)
		slog.Debug(fmt.Sprintf("📥 Rcv'd Provisioned Products... # of Products %d", len(m.provisionedList.Items())))
	case spinner.TickMsg:
		m.loader, cmd = m.loader.Update(msg)
	}

	m.provisionedList, cmd = m.provisionedList.Update(msg)

	return m, cmd
}

func (m ServiceCatalogViewModel) Title() string {
	return "Service Catalog"
}

func (m ServiceCatalogViewModel) Commands() common.Commands {
	return make([]key.Binding, 0)
}

func (m ServiceCatalogViewModel) InputRoutingStrategy() common.InputRoutingStrategy {
	return common.RouteGlobalFirst
}

func (m *ServiceCatalogViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg
	m.listWidth = int(float64(msg.Width) * serviceCatalogListWidthRatio)
	m.detailsWidth = msg.Width - m.listWidth
	if msg.Height < 0 {
		msg.Height = 0
	}
	m.provisionedList.SetSize(m.listWidth, msg.Height)
}

func (m ServiceCatalogViewModel) renderDetail() string {
	return generateProvisionedProductDetail(m.provisionedList.SelectedItem())
}

func (m ServiceCatalogViewModel) isLoading() bool {
	return m.isLoadingProvisioned
}

func provisionedProductsToItems(items []aws.ProvisionedProduct) []list.Item {
	out := make([]list.Item, len(items))
	for i, item := range items {
		out[i] = list.Item(item)
	}
	return out
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
