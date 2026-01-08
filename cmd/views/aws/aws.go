// Package view.aws responsible for aws actionts
package aws

import (
	awsview "flying_nimbus/cmd/views/aws/services"

	"github.com/rivo/tview"
)

type ListView struct {
	Layout  tview.Primitive
	pages   *tview.Pages
	app     *tview.Application
	awsMenu *tview.List
}

func NewAwsView(pages *tview.Pages, app *tview.Application) *ListView {
	awsView := &ListView{
		pages: pages,
		app:   app,
	}

	awsView.buildLayout()
	return awsView
}

func (awsView *ListView) buildLayout() {
	sidebar := tview.NewTextView().
		SetText("Select a AWS service to manage").
		SetDynamicColors(true)

	// Then set border and title (these are Box methods)
	sidebar.SetBorder(true).SetTitle("Description")

	awsView.awsMenu = tview.NewList().
		AddItem("EC2", "EC2 Resources", '1', awsView.navToAwsEC2).
		AddItem("RDS", "RDS Resources", '2', awsView.navToAwsRDS).
		AddItem("Service Catalog", "Service Catalog", '3', awsView.navToAwsSC).
		AddItem("Back", "Quit", 'q', func() { awsView.goBack() })

	flex := tview.NewFlex().
		AddItem(awsView.awsMenu, 0, 1, true).
		AddItem(sidebar, 0, 1, false)

	frame := tview.NewFrame(flex).
		SetBorders(2, 2, 2, 2, 6, 2).
		AddText("Flying Nimbus - Multi-Cloud Tool", true, tview.AlignCenter, tview.Styles.PrimaryTextColor).
		AddText("Press 'q' to quit", false, tview.AlignCenter, tview.Styles.SecondaryTextColor)

	awsView.Layout = frame
}

func (awsView *ListView) navToAwsEC2() {
	ec2View := awsview.NewEC2View(awsView.pages, awsView.app)
	awsView.pages.AddAndSwitchToPage("ec2", ec2View.Layout, true)
}

func (awsView *ListView) navToAwsRDS() {
	awsView.pages.SwitchToPage("root")
}

func (awsView *ListView) navToAwsSC() {
	scView := awsview.NewServiceCatalogView(awsView.pages, awsView.app)
	awsView.pages.AddAndSwitchToPage("servicecatalog", scView.Layout, true)
}

func (awsView *ListView) goBack() {
	awsView.pages.SwitchToPage("root")
}
