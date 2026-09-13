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

type MetadataFetchJob struct {
	ID          string               `json:"id"`
	Status      string               `json:"status"`
	CreatedAt   time.Time            `json:"created_at"`
	CompletedAt *time.Time           `json:"completed_at,omitempty"`
	Entries     []MetadataFetchEntry `json:"entries"`
}

type MetadataFetchEntry struct {
	GalleryID   int64      `json:"gid"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	RefreshedAt *time.Time `json:"refreshed_at,omitempty"`
}

// LookupResult partitions requested IDs into four disjoint outcomes. Retained
// metadata wins over any subsequent collection failure or pending refresh.
type LookupResult struct {
	Galleries  []CollectedMetadata `json:"galleries"`
	PendingIDs []int64             `json:"pending_ids"`
	FailedIDs  []int64             `json:"failed_ids"`
	UnknownIDs []int64             `json:"unknown_ids"`
}
