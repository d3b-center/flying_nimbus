package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestModal(numActions int) ActionMenu {
	actions := make([]ActionItem, numActions)
	for i := range actions {
		actions[i] = ActionItem{Label: "action", Action: func() tea.Cmd { return nil }}
	}
	return NewActionModal("test", actions)
}

func rightKeyMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRight}
}

func leftKeyMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyLeft}
}

func TestMoveCursor_Right(t *testing.T) {
	m := newTestModal(3)

	m.moveCursor(rightKeyMsg())
	if m.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.cursor)
	}

	m.moveCursor(rightKeyMsg())
	if m.cursor != 2 {
		t.Errorf("expected cursor 2, got %d", m.cursor)
	}

	// Wrap around
	m.moveCursor(rightKeyMsg())
	if m.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", m.cursor)
	}
}

func TestMoveCursor_Left(t *testing.T) {
	m := newTestModal(3)

	m.moveCursor(leftKeyMsg())
	if m.cursor != 2 {
		t.Errorf("expected cursor 2, got %d", m.cursor)
	}

	m.moveCursor(leftKeyMsg())
	if m.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.cursor)
	}

	// Wrap around
	m.moveCursor(leftKeyMsg())
	if m.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", m.cursor)
	}
}
