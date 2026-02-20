package views

import (
	"flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

var (
	focusedColor   = lipgloss.Color("62")
	unfocusedColor = lipgloss.Color("240")
	forceRefresh   = key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh RDSs"),
	)
	toggleFocus = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "toggle focus"),
	)
	leftKey key.Binding = key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("left/h", "left"),
	)
	rightKey key.Binding = key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("right/l", "right"),
	)
)

const (
	BorderHeight           = 2 // top + bottom
	BorderWidth            = 4
	instanceListWidthRatio = 0.25
)

// GenerateTagRows takes tags and formats them for rendering
func GenerateTagRows(tags map[string]string) []string {
	var rows []string

	if len(tags) == 0 {
		slog.Debug("No tags found")
		return append(rows, "    None")
	}

	for key, value := range tags {
		rows = append(rows, common.KV("  "+key, value))
	}

	return rows
}

// GenerateEbsVolumeRows takes volumes and formats their data for rendering
func GenerateEbsVolumeRows(volumes []aws.EbsVolume) []string {
	var rows []string

	for _, vol := range volumes {
		rows = append(rows,
			fmt.Sprintf("  • %s", vol.VolumeID),
			fmt.Sprintf("    Size: %d GB | Type: %s ", vol.SizeGb, vol.StorageType),
		)
	}

	return rows
}