package common

import (
	"flying_nimbus/internal/app"
)

// NavigateMsg tells the RootModel to push a new screen onto the stack.
type NavigateMsg struct {
	Model NimbusModel
}

// BackMsg tells the RootModel to pop the current screen.
type BackMsg struct{}

type ContentWindowSizeMsg struct {
	Width  int
	Height int
}

type NavItem struct {
	title string
	desc  string
	Model func(app *app.App, currentWindowSize ContentWindowSizeMsg) NimbusModel
}

func NewNavItem(title string, desc string, model func(*app.App, ContentWindowSizeMsg) NimbusModel) NavItem {
	return NavItem{
		title: title,
		desc:  desc,
		Model: model,
	}
}

func (c NavItem) Title() string       { return c.title }
func (c NavItem) Description() string { return c.desc }
func (c NavItem) FilterValue() string { return c.title }
