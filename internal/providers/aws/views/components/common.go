package components

import (
	"fmt"
	"flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
)

func GenerateTagRows(tags map[string]string) []string {
	var rows []string

	if len(tags) == 0 {
		return append(rows, "  None")
	}

	for key, value := range tags {
		if key != "Name" { // Skip Name tag since it's already shown
			rows = append(rows, common.KV("  "+key, value))
		}
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