package components

import (
	"fmt"
	"log/slog"
	"encoding/json"
	"flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
)

func GenerateTagRows(tags map[string]string) []string {
	var rows []string
	debug, _ := json.Marshal(tags)
	slog.Debug(string(debug))

	if len(tags) == 0 {
		slog.Debug("No tags found")
		return append(rows, "    None")
	}

	for key, value := range tags {
		slog.Debug("Appending tag", "key", key, "value", value)
		rows = append(rows, common.KV("  "+key, value))
	}

	return rows
}

func GenerateEbsVolumeRows(volumes []aws.EbsVolume) []string {
	var rows []string

	for _, vol := range volumes {
		rows = append(rows, 
			fmt.Sprintf("  • %s", vol.VolumeID),
			fmt.Sprintf("    Size: %s GB | Type: %s ", vol.Size, vol.StorageType),
		)
	}
	
	return rows
}