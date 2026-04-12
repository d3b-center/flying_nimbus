package components

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flying_nimbus/internal/tui/constants"
)

var (
	inputLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true)

	inputErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			Width(14)

	errorPopupStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

type InputField struct {
	Label       string
	Placeholder string
	Value       string
	CharLimit   int
	Validator   func(string) error
	Options     []string
	Required    bool
}

// Temporary struct that passed into parent's OnSubmit function
type InputFormResult map[string]string

type (
	InputFormCancelMsg struct{}
	InputFormOpenMsg   struct{}
)

type InputFormSubmitMsg struct {
	Values   InputFormResult
	OnSubmit func(InputFormResult) tea.Cmd
}

type InputForm struct {
	title      string
	inputs     []textinput.Model
	labels     []string
	isDropdown []bool
	options    [][]string
	cursor     int
	onSubmit   func(InputFormResult) tea.Cmd
}

// NewInputForm creates a form with the given fields.
// onSubmit is called with the collected values when the user presses enter on the last field.
// If a field has Options, it will be rendered as a dropdown selector.
func NewInputForm(title string, fields []InputField, onSubmit func(InputFormResult) tea.Cmd) InputForm {
	inputs := make([]textinput.Model, len(fields))
	labels := make([]string, len(fields))
	isDropdown := make([]bool, len(fields))
	options := make([][]string, len(fields))

	for i, f := range fields {
		ti := textinput.New()
		ti.Placeholder = f.Placeholder
		ti.CharLimit = f.CharLimit
		ti.Width = 20
		ti.Validate = f.Validator
		if i == 0 {
			ti.Focus()
		}

		if len(f.Options) > 0 {
			isDropdown[i] = true
			options[i] = f.Options
			ti.SetValue(f.Options[0])
			ti.Placeholder = strings.Join(f.Options, ", ")
		} else if f.Value != "" {
			ti.SetValue(f.Value)
		}

		if f.Required && ti.Value() == "" {
			ti.Err = fmt.Errorf("required")
		}

		inputs[i] = ti
		labels[i] = f.Label
	}

	return InputForm{
		title:      title,
		inputs:     inputs,
		labels:     labels,
		isDropdown: isDropdown,
		options:    options,
		cursor:     0,
		onSubmit:   onSubmit,
	}
}

func (m InputForm) Init() tea.Cmd {
	return nil
}

func (m InputForm) Update(msg tea.Msg) (InputForm, tea.Cmd) {
	var cmd tea.Cmd

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		cmd = m.handleKeypress(keyMsg)
	}

	return m, cmd
}

func (m *InputForm) handleKeypress(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, constants.Keymap.Back):
		return func() tea.Msg { return InputFormCancelMsg{} }

	case key.Matches(msg, constants.Keymap.Enter):
		return m.submit()

	case key.Matches(msg, NextField), key.Matches(msg, PrevField):
		m.moveCursor(msg)
		return textinput.Blink

	case key.Matches(msg, constants.Keymap.CursorUp), key.Matches(msg, constants.Keymap.CursorDown):
		if m.isDropdown[m.cursor] && len(m.options[m.cursor]) > 0 {
			return m.cycleDropdown(msg)
		}
		var cmd tea.Cmd
		m.inputs[m.cursor], cmd = m.inputs[m.cursor].Update(msg)
		return cmd

	default:
		// Pass  to focused input
		var cmd tea.Cmd
		m.inputs[m.cursor], cmd = m.inputs[m.cursor].Update(msg)
		m.validateRequired()
		return cmd
	}
}

func (m *InputForm) cycleDropdown(msg tea.KeyMsg) tea.Cmd {
	currentVal := m.inputs[m.cursor].Value()
	currentIdx := 0
	for i, opt := range m.options[m.cursor] {
		if opt == currentVal {
			currentIdx = i
			break
		}
	}

	var nextIdx int
	if key.Matches(msg, constants.Keymap.CursorDown) {
		nextIdx = (currentIdx + 1) % len(m.options[m.cursor])
	} else {
		nextIdx = (currentIdx - 1 + len(m.options[m.cursor])) % len(m.options[m.cursor])
	}

	m.inputs[m.cursor].SetValue(m.options[m.cursor][nextIdx])
	m.inputs[m.cursor].Err = nil
	return nil
}

func (m *InputForm) submit() tea.Cmd {
	m.validateRequired()

	var hasErrors bool

	values := make(InputFormResult)
	for i, input := range m.inputs {
		if input.Err != nil {
			hasErrors = true
			slog.Error(input.Err.Error())
		}
		values[m.labels[i]] = input.Value()
	}

	if hasErrors {
		return nil
	}

	slog.Debug("Form submitted!")
	return func() tea.Msg {
		return InputFormSubmitMsg{
			Values:   values,
			OnSubmit: m.onSubmit,
		}
	}
}

func (m *InputForm) validateRequired() {
	for i, input := range m.inputs {
		if input.Value() == "" && m.inputs[i].Err == nil {
			input.Err = fmt.Errorf("required field")
			m.inputs[i] = input
		} else if input.Value() != "" && m.inputs[i].Err != nil {
			input.Err = nil
			m.inputs[i] = input
		}
	}
}

func (m *InputForm) moveCursor(msg tea.KeyMsg) {
	if key.Matches(msg, NextField) {
		m.cursor = (m.cursor + 1) % len(m.inputs)
	} else {
		m.cursor = (m.cursor - 1 + len(m.inputs)) % len(m.inputs)
	}

	m.focusCurrent()
}

func (m *InputForm) focusCurrent() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	m.inputs[m.cursor].Focus()
}

func (m InputForm) View() string {
	var rows []string

	rows = append(rows, ModalTitleStyle.Render(m.title))

	var hasErrors bool

	for i, input := range m.inputs {
		style := inputLabelStyle
		if input.Err != nil {
			hasErrors = true
			style = inputErrorStyle
		}
		label := style.Render(m.labels[i] + ":")

		var inputView string
		if m.isDropdown[i] {
			val := input.Value()
			if val == "" {
				val = m.options[i][0]
			}
			inputView = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).
				Padding(0, 1).
				Render(val + " ▾")
		} else {
			inputView = input.View()
		}

		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center, label, inputView))
	}

	if hasErrors {
		rows = append(rows, errorPopupStyle.Render("\nInput is invalid!\n"))
	}

	help := "tab/shft+tab: select field • enter: submit • esc: cancel"
	if m.isDropdown[m.cursor] {
		help = "↑/↓: change option • " + help
	}
	rows = append(rows, ModalHelpStyle.Render(help))
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	return ModalOverlayStyle.Render(content)
}
