package domain

import (
	"crypto/sha1"
	"encoding/hex"
)

const grafanaDashboardUIDPrefix = "project-"
const grafanaDashboardUIDHashLen = 32

func ProjectGrafanaDashboardUID(projectID string) string {
	sum := sha1.Sum([]byte(projectID))
	return grafanaDashboardUIDPrefix + hex.EncodeToString(sum[:])[:grafanaDashboardUIDHashLen]
}
