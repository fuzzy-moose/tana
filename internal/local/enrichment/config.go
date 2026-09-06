package enrichment

import (
	"fmt"
	"strings"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// ConfiguredClient returns nil when Panda enrichment is disabled.
func ConfiguredClient(getenv func(string) string) (*collectorapi.Client, error) {
	baseURL := strings.TrimSpace(getenv("TANA_COLLECTOR_URL"))
	if baseURL == "" {
		return nil, nil
	}
	client, err := collectorapi.NewClient(baseURL, strings.TrimSpace(getenv("TANA_COLLECTOR_API_TOKEN")))
	if err != nil {
		return nil, fmt.Errorf("configure Panda enrichment: %w", err)
	}
	return client, nil
}
