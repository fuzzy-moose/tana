package cleanup

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func pandaID(name, kind string) int64 {
	return source.PandaCandidateID(name, source.Kind(kind))
}

func title(value panda.Metadata) string {
	text := strings.TrimSpace(html.UnescapeString(value.Title))
	if text == "" {
		text = strings.TrimSpace(html.UnescapeString(value.TitleJapanese))
	}
	return text
}

// Lookup follows only already collected parent references. Missing metadata
// ends that branch; it never schedules collection or guesses intermediate IDs.
func (s *Service) metadata(ctx context.Context, ids []int64) (map[int64]panda.Metadata, error) {
	values := map[int64]panda.Metadata{}
	queued := map[int64]bool{}
	queue := make([]int64, 0, len(ids))
	add := func(id int64) {
		if id > 0 && !queued[id] {
			queued[id] = true
			queue = append(queue, id)
		}
	}
	for _, id := range ids {
		add(id)
	}
	for len(queue) != 0 {
		n := min(len(queue), collectorapi.MaxLookupSize)
		batch := append([]int64(nil), queue[:n]...)
		queue = queue[n:]
		result, err := s.client.Lookup(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		for _, item := range result.Galleries {
			if item.Metadata.Error == "" {
				values[item.Metadata.ID] = item.Metadata
				add(item.Metadata.ParentID)
			}
		}
	}
	return values, nil
}

// A known parent link proves its endpoint even if metadata beyond that
// endpoint is missing. Cycles make the whole traversed chain untrustworthy.
func ancestry(id int64, values map[int64]panda.Metadata) ([]int64, string) {
	seen := map[int64]bool{id: true}
	var ancestors []int64
	for {
		value, ok := values[id]
		if !ok {
			return ancestors, "Parent-chain metadata is incomplete"
		}
		if value.ParentID == 0 {
			return ancestors, ""
		}
		if value.ParentID < 0 || value.ParentToken == "" {
			return ancestors, "Parent-chain metadata is incomplete"
		}
		if parent, ok := values[value.ParentID]; ok && parent.Token != "" && parent.Token != value.ParentToken {
			return nil, "Parent reference conflicts with collected metadata"
		}
		if seen[value.ParentID] {
			return nil, "Parent chain contains a cycle"
		}
		seen[value.ParentID] = true
		ancestors = append(ancestors, value.ParentID)
		id = value.ParentID
	}
}
