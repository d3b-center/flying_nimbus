package common

import (
	"flying_nimbus/internal/app"

	tea "github.com/charmbracelet/bubbletea"
)

type NavItem struct {
	title string
	desc  string
	Model func(appService *app.App) tea.Model
}

func NewNavItem(title string, desc string, model func(appService *app.App) tea.Model) NavItem {
	return NavItem{
		title: title,
		desc:  desc,
		Model: model,
	}
}

func (c NavItem) Title() string       { return c.title }
func (c NavItem) Description() string { return c.desc }
func (c NavItem) FilterValue() string { return c.title }
