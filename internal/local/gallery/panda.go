package gallery

import (
	"context"
	"path/filepath"

	"github.com/fuzzy-moose/tana/internal/local/source"
)

type BrowseSort string

const (
	SortTitle         BrowseSort = "title"
	SortFavoritedAsc  BrowseSort = "favorited_asc"
	SortFavoritedDesc BrowseSort = "favorited_desc"
)

func (s BrowseSort) Valid() bool {
	return s == "" || s == SortTitle || s == SortFavoritedAsc || s == SortFavoritedDesc
}

type BrowseOptions struct {
	Categories []string
	Sort       BrowseSort
	PandaFacts []PandaFact
}

func (o BrowseOptions) NeedsPanda() bool {
	return len(o.Categories) > 0 || o.Sort == SortFavoritedAsc || o.Sort == SortFavoritedDesc
}

// PandaFact projects collector data onto an immutable local source identity.
// It applies only while a gallery remains explicitly linked to that source.
type PandaFact struct {
	SourceID    int64  `json:"source_id"`
	Category    string `json:"category"`
	FavoritedAt *int64 `json:"favorited_at"`
}

type PandaCandidate struct {
	SourceID int64
	PandaID  int64
}

func (r *SQLiteRepository) PandaCandidates(ctx context.Context) ([]PandaCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT s.id, s.path, s.kind, l.path
		FROM galleries g JOIN sources s ON s.id = g.source_id
		JOIN libraries l ON l.id = s.library_id ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PandaCandidate{}
	for rows.Next() {
		var sourceID int64
		var name, kind, libraryPath string
		if err := rows.Scan(&sourceID, &name, &kind, &libraryPath); err != nil {
			return nil, err
		}
		if name == "." {
			name = filepath.Base(libraryPath)
		}
		if id := source.PandaCandidateID(name, source.Kind(kind)); id != 0 {
			result = append(result, PandaCandidate{SourceID: sourceID, PandaID: id})
		}
	}
	return result, rows.Err()
}
