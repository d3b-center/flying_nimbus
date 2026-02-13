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

type InputRoutingStrategy int

const (
	// The App/Root handles keys before passing leftovers down
	RouteGlobalFirst InputRoutingStrategy = iota
	// The active component handles keys before passing leftovers up
	RouteFocusedFirst
)

type NimbusModel interface {
	tea.Model
	Titled
	Commanded
	InputRoutingStrategy() InputRoutingStrategy
}
