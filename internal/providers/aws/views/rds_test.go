package views

import (
	"flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
	"testing"

	"github.com/charmbracelet/bubbles/list"
)

func TestDbInstancesToItems(t *testing.T) {
	dbs := []aws.RDSInstance{
		{Id: "db-1"},
		{Id: "db-2"},
	}

	items := dbInstancesToItems(dbs)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].(aws.RDSInstance).Id != "db-1" {
		t.Errorf("unexpected first item")
	}
}

func TestUpdateLayout(t *testing.T) {
	model := RdsViewModel{}

	items := []list.Item{}
	model.list = list.New(items, list.NewDefaultDelegate(), 0, 0)

	msg := common.ContentWindowSizeMsg{
		Width:  100,
		Height: 40,
	}

	model.updateLayout(msg)

	expectedListWidth := int(float64(100) * instanceListWidthRatio)

	if model.instanceListWidth != expectedListWidth && model.list.Width() != expectedListWidth {
		t.Errorf("expected %d, got %d", expectedListWidth, model.instanceListWidth)
	}

	if model.detailsWidth != 100-expectedListWidth {
		t.Errorf("details width mismatch")
	}
}

func TestGatherSecurityGroupIds_Deduplicates(t *testing.T) {
	items := []list.Item{
		aws.RDSInstance{SecurityGroupIds: []string{"sg-1", "sg-2"}},
		aws.RDSInstance{SecurityGroupIds: []string{"sg-2", "sg-3"}},
	}

	ids := gatherSecurityGroupIds(items)

	expected := map[string]bool{
		"sg-1": true,
		"sg-2": true,
		"sg-3": true,
	}

	if len(ids) != len(expected) {
		t.Fatalf("expected %d ids, got %d", len(expected), len(ids))
	}

	for _, id := range ids {
		if !expected[id] {
			t.Errorf("unexpected sg id: %s", id)
		}
	}
}
