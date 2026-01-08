// Package awsview creates views for aws services
package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awsService "flying_nimbus/cmd/services/aws"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type ServiceCatalogView struct {
	Layout       tview.Primitive
	pages        *tview.Pages
	app          *tview.Application
	scService    *awsService.ServiceCatalogService
	productList  *tview.List
	detailsPanel *tview.TextView
	products     []awsService.Product
}

func NewServiceCatalogView(pages *tview.Pages, app *tview.Application) *ServiceCatalogView {
	scv := &ServiceCatalogView{
		pages: pages,
		app:   app,
	}

	ctx := context.Background()
	var err error

	scv.scService, err = awsService.NewServiceCatalogService(ctx)
	if err != nil {
		scv.buildErrorLayout(err)
		return scv
	}

	scv.buildLayout()
	scv.loadProducts()

	return scv
}

func (scv *ServiceCatalogView) buildLayout() {
	scv.productList = tview.NewList().
		ShowSecondaryText(true)

	scv.productList.SetBorder(true).SetTitle("Service Catalog Products (Loading...)")

	scv.detailsPanel = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)

	scv.detailsPanel.SetBorder(true).SetTitle("Product Details")
	scv.detailsPanel.SetText("[yellow]Loading products...[white]\n\nPlease wait...")

	scv.productList.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if index >= 0 && index < len(scv.products) {
			scv.showProductDetails(index)
		}
	})

	flex := tview.NewFlex().
		AddItem(scv.productList, 0, 1, true).
		AddItem(scv.detailsPanel, 0, 2, false)

	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'b':
			scv.goBack()
			return nil
		case 'r':
			scv.refreshProducts()
			return nil
		case 'p':
			currentIndex := scv.productList.GetCurrentItem()
			if currentIndex >= 0 && currentIndex < len(scv.products) {
				scv.provisionProduct(currentIndex)
			}
			return nil
		case 'v':
			scv.viewProvisionedProducts()
			return nil
		}

		if event.Key() == tcell.KeyF5 {
			scv.refreshProducts()
			return nil
		}

		return event
	})

	scv.Layout = flex
}

func (scv *ServiceCatalogView) loadProducts() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		products, err := scv.scService.ListProducts(ctx)

		scv.app.QueueUpdateDraw(func() {
			if err != nil {
				scv.productList.SetTitle("Service Catalog Products (Error)")
				scv.detailsPanel.Clear()
				scv.detailsPanel.SetText(fmt.Sprintf("[red]Error loading products:[white]\n\n%v", err))
				return
			}

			scv.products = products
			scv.populateProductList()
		})
	}()
}

func (scv *ServiceCatalogView) populateProductList() {
	scv.productList.Clear()

	if len(scv.products) == 0 {
		scv.productList.AddItem("No products found", "No Service Catalog products available", 0, nil)
		scv.productList.SetTitle("Service Catalog Products (0)")
		scv.detailsPanel.Clear()
		scv.detailsPanel.SetText("[yellow]No products available[white]\n\nYou don't have access to any Service Catalog products.")
		return
	}

	for i, product := range scv.products {
		description := product.ShortDescription
		if len(description) > 60 {
			description = description[:57] + "..."
		}

		idx := i
		scv.productList.AddItem(
			product.ProductName,
			description,
			0,
			func() {
				scv.provisionProduct(idx)
			},
		)
	}

	scv.productList.AddItem("", "", 0, nil)
	scv.productList.AddItem("View Provisioned Products", "See all provisioned products", 'v', scv.viewProvisionedProducts)
	scv.productList.AddItem("Back to AWS Menu", "Return to AWS services", 'b', scv.goBack)

	scv.productList.SetTitle(fmt.Sprintf("Service Catalog Products (%d)", len(scv.products)))

	if len(scv.products) > 0 {
		scv.productList.SetCurrentItem(0)
		scv.showProductDetails(0)
	}
}

func (scv *ServiceCatalogView) showProductDetails(index int) {
	if index < 0 || index >= len(scv.products) {
		scv.detailsPanel.Clear()
		scv.detailsPanel.SetText("Select a product to view details")
		return
	}

	product := scv.products[index]

	scv.detailsPanel.Clear()

	details := fmt.Sprintf(
		"[yellow]%s[white]\n\n"+
			"[yellow]Product Information[white]\n"+
			"Product ID:   %s\n"+
			"Type:         %s\n"+
			"Owner:        %s\n"+
			"Distributor:  %s\n\n"+
			"[yellow]Description[white]\n"+
			"%s\n\n"+
			"[yellow]Support[white]\n"+
			"%s\n\n"+
			"[yellow]Actions[white]\n"+
			"• Press 'p' or Enter to provision this product\n"+
			"• Press 'v' to view provisioned products\n"+
			"• Press 'r' to refresh list\n"+
			"• Press 'b' to go back",
		product.ProductName,
		product.ProductID,
		product.ProductType,
		product.Owner,
		product.Distributor,
		scv.getDisplayValue(product.ShortDescription),
		scv.getDisplayValue(product.SupportDescription),
	)

	scv.detailsPanel.SetText(details)
}

func (scv *ServiceCatalogView) provisionProduct(index int) {
	if index < 0 || index >= len(scv.products) {
		return
	}

	product := scv.products[index]

	// First, get available versions
	scv.showVersionSelector(product)
}

func (scv *ServiceCatalogView) showVersionSelector(product awsService.Product) {
	loadingText := tview.NewTextView().
		SetText(fmt.Sprintf("[yellow]Loading versions for %s...[white]\n\nPlease wait...", product.ProductName)).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	loadingText.SetBorder(true).SetTitle("Loading")

	loadingModal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(loadingText, 7, 1, false).
			AddItem(nil, 0, 1, false), 60, 1, false).
		AddItem(nil, 0, 1, false)

	scv.pages.AddPage("version-loading", loadingModal, true, true)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		artifacts, err := scv.scService.ListProvisioningArtifacts(ctx, product.ProductID)

		scv.app.QueueUpdateDraw(func() {
			scv.pages.RemovePage("version-loading")

			if err != nil {
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to load versions\n\n%v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						scv.pages.RemovePage("version-error")
					})

				scv.pages.AddPage("version-error", errorModal, true, true)
				return
			}

			if len(artifacts) == 0 {
				errorModal := tview.NewModal().
					SetText("No versions available for this product").
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						scv.pages.RemovePage("no-versions")
					})

				scv.pages.AddPage("no-versions", errorModal, true, true)
				return
			}

			scv.displayVersionSelector(product, artifacts)
		})
	}()
}

func (scv *ServiceCatalogView) displayVersionSelector(product awsService.Product, artifacts []awsService.ProvisioningArtifact) {
	list := tview.NewList()

	for i, artifact := range artifacts {
		artifactCopy := artifact
		description := artifact.Description
		if description == "" {
			description = fmt.Sprintf("Created: %s", artifact.CreatedTime.Format("2006-01-02"))
		}

		list.AddItem(
			artifact.Name,
			description,
			rune('1'+i),
			func() {
				scv.pages.RemovePage("version-selector")
				scv.getProvisioningParameters(product, artifactCopy)
			},
		)
	}

	list.AddItem("Cancel", "Go back", 'q', func() {
		scv.pages.RemovePage("version-selector")
	})

	list.SetBorder(true).SetTitle(fmt.Sprintf("Select Version - %s", product.ProductName))

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(list, 0, 3, true).
			AddItem(nil, 0, 1, false), 70, 1, true).
		AddItem(nil, 0, 1, false)

	scv.pages.AddPage("version-selector", flex, true, true)
}

func (scv *ServiceCatalogView) getProvisioningParameters(product awsService.Product, artifact awsService.ProvisioningArtifact) {
	loadingText := tview.NewTextView().
		SetText(fmt.Sprintf("[yellow]Loading parameters...[white]\n\nPlease wait...")).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	loadingText.SetBorder(true).SetTitle("Loading")

	loadingModal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(loadingText, 7, 1, false).
			AddItem(nil, 0, 1, false), 60, 1, false).
		AddItem(nil, 0, 1, false)

	scv.pages.AddPage("params-loading", loadingModal, true, true)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		parameters, err := scv.scService.DescribeProvisioningParameters(ctx, product.ProductID, artifact.ID)

		scv.app.QueueUpdateDraw(func() {
			scv.pages.RemovePage("params-loading")

			if err != nil {
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to load parameters\n\n%v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						scv.pages.RemovePage("params-error")
					})

				scv.pages.AddPage("params-error", errorModal, true, true)
				return
			}

			scv.showProvisioningForm(product, artifact, parameters)
		})
	}()
}

func (scv *ServiceCatalogView) showProvisioningForm(product awsService.Product, artifact awsService.ProvisioningArtifact, parameters []awsService.ProvisioningParameter) {
	form := tview.NewForm()

	// Add product name field
	defaultName := fmt.Sprintf("%s-%d", product.ProductName, time.Now().Unix())
	form.AddInputField("Provisioned Product Name", defaultName, 50, nil, nil)

	// Add parameter fields
	for _, param := range parameters {
		if len(param.AllowedValues) > 0 {
			// Dropdown for allowed values
			form.AddDropDown(param.Key, param.AllowedValues, 0, nil)
		} else {
			// Text input
			defaultVal := param.DefaultValue
			form.AddInputField(param.Key, defaultVal, 50, nil, nil)
		}
	}

	// Add buttons
	form.AddButton("Provision", func() {
		scv.pages.RemovePage("provision-form")
		scv.executeProvisioning(product, artifact, form, parameters)
	})

	form.AddButton("Cancel", func() {
		scv.pages.RemovePage("provision-form")
	})

	form.SetBorder(true).SetTitle(fmt.Sprintf("Provision: %s", product.ProductName))
	form.SetFieldBackgroundColor(tcell.ColorBlack)

	// Center the form
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 0, 3, true).
			AddItem(nil, 0, 1, false), 80, 1, true).
		AddItem(nil, 0, 1, false)

	scv.pages.AddPage("provision-form", flex, true, true)
}

func (scv *ServiceCatalogView) executeProvisioning(product awsService.Product, artifact awsService.ProvisioningArtifact, form *tview.Form, parameters []awsService.ProvisioningParameter) {
	// Get the provisioned product name
	nameField := form.GetFormItem(0).(*tview.InputField)
	provisionedProductName := nameField.GetText()

	// Collect parameter values
	paramValues := make(map[string]string)
	for i, param := range parameters {
		formItem := form.GetFormItem(i + 1)

		var value string
		switch field := formItem.(type) {
		case *tview.InputField:
			value = field.GetText()
		case *tview.DropDown:
			_, value = field.GetCurrentOption()
		}

		paramValues[param.Key] = value
	}

	// Show provisioning progress
	progressText := tview.NewTextView().
		SetText(fmt.Sprintf("[yellow]Provisioning %s...[white]\n\nPlease wait...", product.ProductName)).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	progressText.SetBorder(true).SetTitle("Provisioning")

	progressModal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(progressText, 7, 1, false).
			AddItem(nil, 0, 1, false), 60, 1, false).
		AddItem(nil, 0, 1, false)

	scv.pages.AddPage("provisioning", progressModal, true, true)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		recordID, err := scv.scService.ProvisionProduct(ctx, product.ProductID, artifact.ID, provisionedProductName, paramValues)

		scv.app.QueueUpdateDraw(func() {
			scv.pages.RemovePage("provisioning")

			if err != nil {
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Provisioning Failed\n\n%v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						scv.pages.RemovePage("provision-error")
					})

				scv.pages.AddPage("provision-error", errorModal, true, true)
				return
			}

			successModal := tview.NewModal().
				SetText(fmt.Sprintf(
					"Provisioning Started!\n\n"+
						"Product: %s\n"+
						"Name: %s\n"+
						"Record ID: %s\n\n"+
						"The product is being provisioned.\n"+
						"Check 'View Provisioned Products' for status.",
					product.ProductName,
					provisionedProductName,
					recordID)).
				AddButtons([]string{"OK", "View Provisioned Products"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					scv.pages.RemovePage("provision-success")
					if buttonLabel == "View Provisioned Products" {
						scv.viewProvisionedProducts()
					}
				})

			scv.pages.AddPage("provision-success", successModal, true, true)
		})
	}()
}

func (scv *ServiceCatalogView) viewProvisionedProducts() {
	loadingText := tview.NewTextView().
		SetText("[yellow]Loading provisioned products...[white]\n\nPlease wait...").
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	loadingText.SetBorder(true).SetTitle("Loading")

	loadingModal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(loadingText, 7, 1, false).
			AddItem(nil, 0, 1, false), 60, 1, false).
		AddItem(nil, 0, 1, false)

	scv.pages.AddPage("provisioned-loading", loadingModal, true, true)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		provisionedProducts, err := scv.scService.ListProvisionedProducts(ctx)

		scv.app.QueueUpdateDraw(func() {
			scv.pages.RemovePage("provisioned-loading")

			if err != nil {
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to load provisioned products\n\n%v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						scv.pages.RemovePage("provisioned-error")
					})

				scv.pages.AddPage("provisioned-error", errorModal, true, true)
				return
			}

			scv.displayProvisionedProducts(provisionedProducts)
		})
	}()
}

func (scv *ServiceCatalogView) displayProvisionedProducts(products []awsService.ProvisionedProduct) {
	var content strings.Builder

	content.WriteString(fmt.Sprintf("[yellow]Provisioned Products (%d)[white]\n\n", len(products)))

	if len(products) == 0 {
		content.WriteString("No provisioned products found.\n")
	} else {
		for _, product := range products {
			statusColor := "white"
			switch product.Status {
			case "AVAILABLE":
				statusColor = "green"
			case "ERROR", "TAINTED":
				statusColor = "red"
			case "UNDER_CHANGE":
				statusColor = "yellow"
			}

			content.WriteString(fmt.Sprintf("[yellow]%s[white]\n", product.Name))
			content.WriteString(fmt.Sprintf("  ID: %s\n", product.ID))
			content.WriteString(fmt.Sprintf("  Status: [%s]%s[white]\n", statusColor, product.Status))
			if product.StatusMessage != "" {
				content.WriteString(fmt.Sprintf("  Message: %s\n", product.StatusMessage))
			}
			content.WriteString(fmt.Sprintf("  Created: %s\n", product.CreatedTime.Format("2006-01-02 15:04:05")))
			content.WriteString("\n")
		}
	}

	textView := tview.NewTextView().
		SetText(content.String()).
		SetDynamicColors(true).
		SetScrollable(true)

	textView.SetBorder(true).SetTitle("Provisioned Products")

	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEsc {
			scv.pages.RemovePage("provisioned-products")
			return nil
		}
		return event
	})

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(textView, 0, 8, true).
			AddItem(nil, 0, 1, false), 0, 8, true).
		AddItem(nil, 0, 1, false)

	footer := tview.NewTextView().
		SetText("[yellow]Press 'q' or ESC to close[white]").
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	finalFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(flex, 0, 1, true).
		AddItem(footer, 1, 0, false)

	scv.pages.AddPage("provisioned-products", finalFlex, true, true)
}

func (scv *ServiceCatalogView) refreshProducts() {
	scv.productList.SetTitle("Service Catalog Products (Refreshing...)")
	scv.detailsPanel.Clear()
	scv.detailsPanel.SetText("[yellow]Refreshing products...[white]")
	scv.loadProducts()
}

func (scv *ServiceCatalogView) getDisplayValue(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func (scv *ServiceCatalogView) buildErrorLayout(err error) {
	errorText := tview.NewTextView().
		SetText(fmt.Sprintf("[red]Error initializing Service Catalog:[white]\n\n%v\n\n"+
			"Press 'b' to go back", err)).
		SetDynamicColors(true)

	errorText.SetBorder(true).SetTitle("Error")

	errorText.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'b' {
			scv.goBack()
		}
		return event
	})

	scv.Layout = errorText
}

func (scv *ServiceCatalogView) goBack() {
	scv.pages.SwitchToPage("aws")
}
