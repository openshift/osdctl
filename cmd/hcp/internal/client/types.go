package client

import (
	"sort"
	"strings"
)

// VeleroBackupRequest identifies the schedule and metadata for a Velero backup.
type VeleroBackupRequest struct {
	Namespace   string
	Schedule    string
	Container   string
	LabelKey    string
	LabelValue  string
	Labels      map[string]string
	Annotations map[string]string
}

func joinSortedMap(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+values[key])
	}
	return strings.Join(pairs, ",")
}
