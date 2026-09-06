// Package collectorapi defines the collector's metadata lookup protocol.
package collectorapi

import (
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

const MaxLookupSize = 100

type CollectedMetadata struct {
	Metadata    panda.Metadata `json:"metadata"`
	RefreshedAt time.Time      `json:"refreshed_at"`
}

// LookupResult partitions requested IDs into four disjoint outcomes. Retained
// metadata wins over any subsequent collection failure or pending refresh.
type LookupResult struct {
	Galleries  []CollectedMetadata `json:"galleries"`
	PendingIDs []int64             `json:"pending_ids"`
	FailedIDs  []int64             `json:"failed_ids"`
	UnknownIDs []int64             `json:"unknown_ids"`
}
