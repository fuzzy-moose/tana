package gallery

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/pgdb"
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

func (r *Store) pandaCandidateID(ctx context.Context, galleryID int64) (int64, error) {
	var name, kind, libraryPath string
	err := r.db.QueryRow(ctx, `SELECT s.path, s.kind, l.path
		FROM galleries g JOIN sources s ON s.id = g.source_id
		JOIN libraries l ON l.id = s.library_id WHERE g.id = $1`, galleryID).Scan(&name, &kind, &libraryPath)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return sourcePandaCandidateID(name, kind, libraryPath), nil
}

// sourcePandaCandidateID infers the Panda gallery candidate of a source; a library root source takes the root's name.
func sourcePandaCandidateID(name, kind, libraryPath string) int64 {
	if name == "." {
		name = filepath.Base(libraryPath)
	}
	return source.PandaCandidateID(name, source.Kind(kind))
}

// PandaCandidateIDsByGallery maps each of the given source-linked galleries to its Panda gallery candidate ID.
func (r *Store) PandaCandidateIDsByGallery(ctx context.Context, galleryIDs []int64) (map[int64]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT g.id, s.path, s.kind, l.path
		FROM galleries g JOIN sources s ON s.id = g.source_id
		JOIN libraries l ON l.id = s.library_id WHERE g.id = ANY($1::bigint[])`, galleryIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]int64{}
	for rows.Next() {
		var galleryID int64
		var name, kind, libraryPath string
		if err := rows.Scan(&galleryID, &name, &kind, &libraryPath); err != nil {
			return nil, err
		}
		if id := sourcePandaCandidateID(name, kind, libraryPath); id != 0 {
			result[galleryID] = id
		}
	}
	return result, rows.Err()
}

func (r *Store) PandaCandidates(ctx context.Context, search string) ([]PandaCandidate, error) {
	tx, err := r.db.BeginTx(ctx, pgdb.ReadOnly)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return nil, err
	}
	var args pgdb.Args
	predicate, err := searchPredicate(search, namespaces, &args)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT s.id, s.path, s.kind, l.path
		FROM galleries g JOIN sources s ON s.id = g.source_id
		JOIN libraries l ON l.id = s.library_id WHERE `+predicate+` ORDER BY s.id`, args...)
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
		if id := sourcePandaCandidateID(name, kind, libraryPath); id != 0 {
			result = append(result, PandaCandidate{SourceID: sourceID, PandaID: id})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, tx.Commit(ctx)
}
