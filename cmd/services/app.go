// Package models has logic for funtionality
package services

import (
	"flying_nimbus/cmd/views"

	"github.com/rivo/tview"
)

type App struct {
	TviewApp *tview.Application
	Pages    *tview.Pages
	Root     *views.RootView
}

func NewApp() *App {
	app := &App{
		TviewApp: tview.NewApplication(),
		Pages:    tview.NewPages(),
	}

	// Init root views
	app.Root = views.NewRootView(app.Pages, app.TviewApp)

	app.Pages.AddPage("root", app.Root.Layout, true, true)

	return app
}

func (app *App) Run() error {
	return app.TviewApp.SetRoot(app.Pages, true).Run()
}
