package tui

import tea "github.com/charmbracelet/bubbletea"

type CapabilityItem struct {
	id    string
	title string
	desc  string
	build func() tea.Model
}

func (c CapabilityItem) Title() string       { return c.title }
func (c CapabilityItem) Description() string { return c.desc }
func (c CapabilityItem) FilterValue() string { return c.title }
