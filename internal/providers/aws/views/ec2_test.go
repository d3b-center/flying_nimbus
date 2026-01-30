package views

import (
	"flying_nimbus/internal/providers/aws/backend"
	"testing"

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

func TestResizeViewport(t *testing.T) {
	model := &Ec2ViewModel{
		ready: false,
	}

	model.resizeViewport(800, 600)

	if !model.ready {
		t.Error("expected ready to be true after resize")
	}

	if model.detailViewport.Width != 800 {
		t.Errorf("expected width 800, got %d", model.detailViewport.Width)
	}

	if model.detailViewport.Height != 600 {
		t.Errorf("expected height 600, got %d", model.detailViewport.Height)
	}
}

func TestSetWindowSizes(t *testing.T) {
	model := &Ec2ViewModel{}

	model.setWindowSizes(10, 10)

	// Just verify the method doesn't panic and sets some values
	if model.instanceListWidth == 0 {
		t.Error("expected instanceListWidth to be set")
	}

	if model.detailsWidth == 0 {
		t.Error("expected detailsWidth to be set")
	}
}

func TestUpdate_FocusSwitching(t *testing.T) {
	model := Ec2ViewModel{
		detailsFocused: false,
	}

	// Focus right
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	m := updated.(Ec2ViewModel)

	if !m.detailsFocused {
		t.Error("expected detailsFocused to be true")
	}

	// Focus left
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(Ec2ViewModel)

	if m.detailsFocused {
		t.Error("expected detailsFocused to be false")
	}
}
