package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testFields() []InputField {
	return []InputField{
		{Label: "Local Port", Placeholder: "8080", CharLimit: 5},
		{Label: "Remote Port", Placeholder: "443", CharLimit: 5},
	}
}

func noopSubmit(values InputFormResult) tea.Cmd {
	return nil
}

func TestNewInputForm(t *testing.T) {
	form := NewInputForm("Test Form", testFields(), noopSubmit)

	if form.title != "Test Form" {
		t.Errorf("expected title 'Test Form', got %q", form.title)
	}

	if len(form.inputs) != 2 {
		t.Fatalf("expected 2 inputs, got %d", len(form.inputs))
	}

	if len(form.labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(form.labels))
	}

	if form.labels[0] != "Local Port" {
		t.Errorf("expected first label 'Local Port', got %q", form.labels[0])
	}

	if form.labels[1] != "Remote Port" {
		t.Errorf("expected second label 'Remote Port', got %q", form.labels[1])
	}

	if form.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", form.cursor)
	}
}

func TestMoveCursor_Next(t *testing.T) {
	form := NewInputForm("Test", testFields(), noopSubmit)

	form.moveCursor(tea.KeyMsg{Type: tea.KeyTab})

	if form.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", form.cursor)
	}

	if !form.inputs[1].Focused() {
		t.Error("expected second input to be focused")
	}

	if form.inputs[0].Focused() {
		t.Error("expected first input to be blurred")
	}
}

func TestMoveCursor_NextWraps(t *testing.T) {
	form := NewInputForm("Test", testFields(), noopSubmit)
	form.cursor = 1

	form.moveCursor(tea.KeyMsg{Type: tea.KeyTab})

	if form.cursor != 0 {
		t.Errorf("expected cursor to wrap to 0, got %d", form.cursor)
	}
}

func TestMoveCursor_Prev(t *testing.T) {
	form := NewInputForm("Test", testFields(), noopSubmit)
	form.cursor = 1

	form.moveCursor(tea.KeyMsg{Type: tea.KeyShiftTab})

	if form.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", form.cursor)
	}
}

func TestMoveCursor_PrevWraps(t *testing.T) {
	form := NewInputForm("Test", testFields(), noopSubmit)

	form.moveCursor(tea.KeyMsg{Type: tea.KeyShiftTab})

	if form.cursor != 1 {
		t.Errorf("expected cursor to wrap to 1, got %d", form.cursor)
	}
}

func TestSubmit_ReturnsValues(t *testing.T) {
	var captured InputFormResult

	onSubmit := func(values InputFormResult) tea.Cmd {
		captured = values
		return nil
	}

	form := NewInputForm("Test", testFields(), onSubmit)
	form.inputs[0].SetValue("9090")
	form.inputs[1].SetValue("3306")

	cmd := form.submit()

	if cmd == nil {
		t.Fatal("expected cmd, got nil")
	}

	msg := cmd()
	submitMsg, ok := msg.(InputFormSubmitMsg)
	if !ok {
		t.Fatalf("expected InputFormSubmitMsg, got %T", msg)
	}

	if submitMsg.Values["Local Port"] != "9090" {
		t.Errorf("expected Local Port '9090', got %q", submitMsg.Values["Local Port"])
	}

	if submitMsg.Values["Remote Port"] != "3306" {
		t.Errorf("expected Remote Port '3306', got %q", submitMsg.Values["Remote Port"])
	}

	// Verify onSubmit receives the values
	submitMsg.OnSubmit(submitMsg.Values)
	if captured["Local Port"] != "9090" {
		t.Errorf("onSubmit received wrong Local Port: %q", captured["Local Port"])
	}
}
