// Package catalogfilter stores the shared background filter for Panda browsing.
package catalogfilter

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/fuzzy-moose/tana/internal/panda"
)

type Filter struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories"`
}

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func Normalize(filter Filter) (Filter, error) {
	categories, err := panda.NormalizeCategories(filter.Categories)
	if categories == nil {
		categories = []string{}
	}
	return Filter{Query: strings.TrimSpace(filter.Query), Categories: categories}, err
}

func (s *Store) Load(ctx context.Context) (Filter, error) {
	var result Filter
	var categories string
	if err := s.db.QueryRowContext(ctx, "SELECT query, categories FROM panda_catalog_default_filter WHERE id = 1").Scan(&result.Query, &categories); err != nil {
		return result, err
	}
	err := json.Unmarshal([]byte(categories), &result.Categories)
	return result, err
}

func (s *Store) Save(ctx context.Context, filter Filter) error {
	categories, err := json.Marshal(filter.Categories)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "UPDATE panda_catalog_default_filter SET query = ?, categories = ? WHERE id = 1", filter.Query, string(categories))
	return err
}
