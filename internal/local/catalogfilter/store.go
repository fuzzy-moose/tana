// Package catalogfilter stores the shared background filter for Panda browsing.
package catalogfilter

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/panda"
)

type Filter struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories"`
}

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }

func Normalize(filter Filter) (Filter, error) {
	categories, err := panda.NormalizeCategories(filter.Categories)
	if categories == nil {
		categories = []string{}
	}
	return Filter{Query: strings.TrimSpace(filter.Query), Categories: categories}, err
}

func (s *Store) Load(ctx context.Context) (Filter, error) {
	var result Filter
	err := s.db.QueryRow(ctx, "SELECT query, categories FROM panda_catalog_default_filter WHERE id = 1").Scan(&result.Query, &result.Categories)
	return result, err
}

func (s *Store) Save(ctx context.Context, filter Filter) error {
	categories := filter.Categories
	if categories == nil {
		categories = []string{}
	}
	_, err := s.db.Exec(ctx, "UPDATE panda_catalog_default_filter SET query = $1, categories = $2 WHERE id = 1", filter.Query, categories)
	return err
}
