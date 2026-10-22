package tui

import (
	"testing"

	"flying_nimbus/internal/tui/common"

	tea "github.com/charmbracelet/bubbletea"
)

// Stand-in screen used only in tests. It records which keys it
// receives, so a test can tell whether RootModel passed a key down to the
// screen or handled it itself.
type fakeModel struct {
	strategy common.InputRoutingStrategy
	gotKeys  *[]string
}

func (f fakeModel) Init() tea.Cmd { return nil }

func (f fakeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		*f.gotKeys = append(*f.gotKeys, k.String())
	}
	return f, nil
}

func (f fakeModel) View() string                                      { return "" }
func (f fakeModel) Title() string                                     { return "fake" }
func (f fakeModel) Commands() common.Commands                         { return nil }
func (f fakeModel) InputRoutingStrategy() common.InputRoutingStrategy { return f.strategy }

// Reports whether running cmd would quit the app.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func newRootWith(strategy common.InputRoutingStrategy) (RootModel, *[]string) {
	keys := &[]string{}
	model := fakeModel{strategy: strategy, gotKeys: keys}
	m := RootModel{stack: []tea.Model{model}}
	return m, keys
}

func keyRunes(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// When the user is typing in the filter box, keys like 'q' should be
// typed as text and reach the screen and NOT quit the app.
func TestHandleKeyMsg_FocusedFirstDelegatesHotkeys(t *testing.T) {
	for _, r := range []rune{'q', 'd', '?'} {
		m, keys := newRootWith(common.RouteFocusedFirst)
		_, cmd := m.handleKeyMsg(keyRunes(r), m.stack[0])

		if isQuit(cmd) {
			t.Fatalf("key %q under RouteFocusedFirst quit the app; expected delegation", r)
		}
		if len(*keys) != 1 || (*keys)[0] != string(r) {
			t.Fatalf("key %q under RouteFocusedFirst not delegated to model; got %v", r, *keys)
		}
	}
}

func TestHandleKeyMsg_GlobalFirstQuitsOnQ(t *testing.T) {
	m, keys := newRootWith(common.RouteGlobalFirst)
	_, cmd := m.handleKeyMsg(keyRunes('q'), m.stack[0])

	if !isQuit(cmd) {
		t.Fatal("'q' under RouteGlobalFirst should quit the app")
	}
	if len(*keys) != 0 {
		t.Fatalf("'q' under RouteGlobalFirst should not reach the model; got %v", *keys)
	}
}

// Ctrl+C must always quit
func TestHandleKeyMsg_ForceQuitAlwaysQuits(t *testing.T) {
	m, _ := newRootWith(common.RouteFocusedFirst)
	_, cmd := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlC}, m.stack[0])

	if !isQuit(cmd) {
		t.Fatal("ctrl+c should force quit even under RouteFocusedFirst")
	}
}

func TestHandleKeyMsg_GlobalFirstTogglesHelp(t *testing.T) {
	m, _ := newRootWith(common.RouteGlobalFirst)
	next, _ := m.handleKeyMsg(keyRunes('?'), m.stack[0])

	if !next.(RootModel).help.ShowAll {
		t.Fatal("'?' under RouteGlobalFirst should toggle full help on")
	}
}
