package common

import tea "github.com/charmbracelet/bubbletea"

type Titled interface {
	Title() string
}

type Commands map[string]string

type Commanded interface {
	Commands() Commands
}

type NimbusModel interface {
	tea.Model
	Titled
	Commanded
}
