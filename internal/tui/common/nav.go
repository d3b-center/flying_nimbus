package common

import (
	"flying_nimbus/internal/app"

	tea "github.com/charmbracelet/bubbletea"
)

// NavigateMsg tells the RootModel to push a new screen onto the stack.
type NavigateMsg struct {
	Model tea.Model
}

// BackMsg tells the RootModel to pop the current screen.
type BackMsg struct{}

type NavItem struct {
	title string
	desc  string
	Model func(app *app.App) tea.Model
}

func NewNavItem(title string, desc string, model func(*app.App) tea.Model) NavItem {
	return NavItem{
		title: title,
		desc:  desc,
		Model: model,
	}
}

func (c NavItem) Title() string       { return c.title }
func (c NavItem) Description() string { return c.desc }
func (c NavItem) FilterValue() string { return c.title }
