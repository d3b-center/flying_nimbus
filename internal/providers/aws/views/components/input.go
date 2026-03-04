package components

import (
	"log/slog"

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
			Foreground(lipgloss.Color("196"))
)

type InputField struct {
	Label       string
	Placeholder string
	CharLimit   int
	// Value pre-populates the field when non-empty.
	Value string
	// Width sets the input width in characters; 0 uses default (20).
	Width int
}

// Temporary struct that passed into parent's OnSubmit function
type InputFormResult map[string]string

type InputFormCancelMsg struct{}
type InputFormOpenMsg struct{}

type InputFormSubmitMsg struct {
	Values   InputFormResult
	OnSubmit func(InputFormResult) tea.Cmd
}

type InputForm struct {
	title         string
	inputs        []textinput.Model
	labels        []string
	labelWidth    int // min width for label column so labels don't wrap
	cursor        int
	err           string
	onSubmit      func(InputFormResult) tea.Cmd
}

// NewInputForm creates a form with the given fields.
// onSubmit is called with the collected values when the user presses enter on the last field.
func NewInputForm(title string, fields []InputField, onSubmit func(InputFormResult) tea.Cmd) InputForm {
	inputs := make([]textinput.Model, len(fields))
	labels := make([]string, len(fields))
	labelWidth := 14

	for i, f := range fields {
		ti := textinput.New()
		ti.Placeholder = f.Placeholder
		ti.CharLimit = f.CharLimit
		if f.Width > 0 {
			ti.Width = f.Width
		} else {
			ti.Width = 20
		}
		if f.Value != "" {
			ti.SetValue(f.Value)
		}
		if i == 0 {
			ti.Focus()
		}
		inputs[i] = ti
		labels[i] = f.Label
		// ensure label column fits longest label (label + ":")
		if w := len(f.Label) + 1; w > labelWidth {
			labelWidth = w
		}
	}

	return InputForm{
		title:      title,
		inputs:     inputs,
		labels:     labels,
		labelWidth: labelWidth,
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

	default:
		// Pass  to focused input
		var cmd tea.Cmd
		m.inputs[m.cursor], cmd = m.inputs[m.cursor].Update(msg)
		return cmd
	}
}

func (m *InputForm) submit() tea.Cmd {
	slog.Debug("Form submitted!")
	m.err = ""
	values := make(InputFormResult)
	for i, input := range m.inputs {
		values[m.labels[i]] = input.Value()
	}

	return func() tea.Msg {
		return InputFormSubmitMsg{
			Values:   values,
			OnSubmit: m.onSubmit,
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

	labelStyle := inputLabelStyle.Width(m.labelWidth)
	for i, input := range m.inputs {
		label := labelStyle.Render(m.labels[i] + ":")
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center, label, input.View()))
	}

	if m.err != "" {
		rows = append(rows, inputErrorStyle.Render("x "+m.err))
	}

	rows = append(rows, ModalHelpStyle.Render("tab/shft+tab: select field • enter: submit • esc: cancel"))
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	return ModalOverlayStyle.Render(content)
}
