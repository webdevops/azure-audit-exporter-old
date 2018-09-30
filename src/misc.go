package main

import (
	"regexp"
)

var (
	resourceGroupFromResourceIdRegExp = regexp.MustCompile("/resourceGroups/([^/]*)")
)

func extractResourceGroupFromAzureId (azureId string) (resourceGroup string) {
	rgSubMatch := resourceGroupFromResourceIdRegExp.FindStringSubmatch(azureId)

	if len(rgSubMatch) >= 1 {
		resourceGroup = rgSubMatch[1]
	}

	return
}
