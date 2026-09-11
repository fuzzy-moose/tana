package collectorapi

import "time"

type SitemapStatus struct {
	State              string     `json:"state"`
	Force              bool       `json:"force"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	RetryAt            *time.Time `json:"retry_at,omitempty"`
	ChildrenTotal      int64      `json:"children_total"`
	ChildrenCompleted  int64      `json:"children_completed"`
	ChildrenSkipped    int64      `json:"children_skipped"`
	ChildrenFailed     int64      `json:"children_failed"`
	ReferencesFound    int64      `json:"references_found"`
	ReferencesImported int64      `json:"references_imported"`
	InvalidLocations   int64      `json:"invalid_locations"`
	LastError          string     `json:"last_error,omitempty"`
}
