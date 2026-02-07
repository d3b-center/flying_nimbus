package common

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type Titled interface {
	Title() string
}

type Commands []key.Binding

type Commanded interface {
	Commands() Commands
}

type NimbusModel interface {
	tea.Model
	Titled
	Commanded
}
