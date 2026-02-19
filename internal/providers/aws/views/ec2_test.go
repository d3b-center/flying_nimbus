package views

import (
	"flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestEc2InstancesToItems(t *testing.T) {
	instances := []aws.Ec2Instance{
		{InstanceID: "i-123", Name: "test-1"},
		{InstanceID: "i-456", Name: "test-2"},
	}

	items := ec2InstancesToItems(instances)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].(aws.Ec2Instance).InstanceID != "i-123" {
		t.Errorf("unexpected first item")
	}
}

func TestGenerateEc2InstanceDetail_Valid(t *testing.T) {
	instance := aws.Ec2Instance{
		InstanceID:   "i-12345",
		Name:         "test-instance",
		InstanceType: "t3.medium",
		State:        "running",
	}

	detail := generateEc2InstanceDetail(instance)

	if detail == "No Info" {
		t.Error("expected valid detail, got 'No Info'")
	}
}

func TestGenerateEc2InstanceDetail_Nil(t *testing.T) {
	detail := generateEc2InstanceDetail(nil)

	if detail != "No Info" {
		t.Errorf("expected 'No Info', got %q", detail)
	}
}

func TestEc2UpdateLayout(t *testing.T) {
	model := &Ec2ViewModel{}

	items := []list.Item{}
	model.list = list.New(items, list.NewDefaultDelegate(), 0, 0)

	msg := common.ContentWindowSizeMsg{Height: 10, Width: 10}

	model.updateLayout(msg)

	// Just verify the method doesn't panic and sets some values
	if model.instanceListWidth == 0 {
		t.Error("expected instanceListWidth to be set")
	}

	if model.detailsWidth == 0 {
		t.Error("expected detailsWidth to be set")
	}
}

func TestUpdate_FocusSwitching(t *testing.T) {
	items := []list.Item{}
	model := Ec2ViewModel{
		detailsFocused: false,
		list:           list.New(items, list.NewDefaultDelegate(), 0, 0),
	}

	// Focus right
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	m := updated.(Ec2ViewModel)

	if !m.detailsFocused {
		t.Error("expected detailsFocused to be true")
	}

	// Focus left
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	m = updated.(Ec2ViewModel)

	if m.detailsFocused {
		t.Error("expected detailsFocused to be false")
	}
}
