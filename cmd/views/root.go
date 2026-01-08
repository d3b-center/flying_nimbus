// Package views is the base view for the app
package views

import (
	"flying_nimbus/cmd/views/aws"

	"github.com/rivo/tview"
)

type RootView struct {
	Layout   tview.Primitive
	pages    *tview.Pages
	app      *tview.Application
	mainMenu *tview.List
}

// NewRootView is constructor for the base app
func NewRootView(pages *tview.Pages, app *tview.Application) *RootView {
	rv := &RootView{
		pages: pages,
		app:   app,
	}

	rv.buildLayout()
	return rv
}

func updateInfoBar(index int, sidebar *tview.TextView) {
	switch index {
	case 0: // AWS
		sidebar.SetText("[yellow]AWS Services[white]\n\n" +
			"Manage AWS resources:\n" +
			"• EC2 Instances\n" +
			"• RDS Instances\n" +
			"• ServiceCatalog")
	case 1: // Azure
		sidebar.SetText("[blue]Azure Services[white]\n\n" +
			"Manage Azure resources:\n" +
			"• Virtual Machines\n" +
			"• SQL Databases\n" +
			"• App Services")
	case 2: // Quit
		sidebar.SetText("[red]Exit Application[white]\n\n" +
			"Press Enter to quit Flying Nimbus")
	}
}

// Build Root View
func (rv *RootView) buildLayout() {
	// Create TextView and set its specific methods first
	sidebar := tview.NewTextView().
		SetText("Welcome! \n \n Select a cloud provider to manage resources").
		SetDynamicColors(true)

	// Then set border and title (these are Box methods)
	sidebar.SetBorder(true).SetTitle("Description")

	rv.mainMenu = tview.NewList().
		AddItem("AWS", "AWS Resources", '1', rv.navToAwsView).
		// AddItem("Azure", "Azure Resources", '2', rv.navToAzureView).
		AddItem("Quit", "Quit", 'q', func() { rv.app.Stop() })

	flex := tview.NewFlex().
		AddItem(rv.mainMenu, 0, 1, true).
		AddItem(sidebar, 0, 1, false)

	rv.mainMenu.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		updateInfoBar(index, sidebar)
	})

	frame := tview.NewFrame(flex).
		SetBorders(2, 2, 2, 2, 6, 2).
		AddText("Flying Nimbus - Multi-Cloud Tool", true, tview.AlignCenter, tview.Styles.PrimaryTextColor).
		AddText("v1.0.0 | Press 'q' to quit", false, tview.AlignCenter, tview.Styles.SecondaryTextColor)

	rv.Layout = frame
}

func (rv *RootView) navToAwsView() {
	awsView := aws.NewAwsView(rv.pages, rv.app)
	rv.pages.AddAndSwitchToPage("aws", awsView.Layout, true)
}

func (rv *RootView) navToAzureView() {
}
