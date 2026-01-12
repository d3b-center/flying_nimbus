package aws

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type AwsProviderModel struct {
	list list.Model
}

func (m AwsProviderModel) Init() tea.Cmd {
	return nil
}

func (m AwsProviderModel) View() string {
	return m.list.View()
}
