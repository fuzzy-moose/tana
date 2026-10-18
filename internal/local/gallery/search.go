package gallery

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/gallerysearch"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

var ErrInvalidQuery = gallerysearch.ErrInvalidQuery
var namespaceShortForms = gallerysearch.NamespaceShortForms

type TagSuggestion = gallerysearch.TagSuggestion
type Completion = gallerysearch.Completion

func searchNamespaces(ctx context.Context, tx pgx.Tx) (map[string]bool, error) {
	names := make(map[string]bool)
	for _, name := range namespaceShortForms {
		names[name] = true
	}
	rows, err := tx.Query(ctx, "SELECT name FROM namespaces")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names[name] = true
	}
	return names, rows.Err()
}

// Sharing the predicate keeps pagination counts and results identical.
func searchPredicate(query string, namespaces map[string]bool, args *pgdb.Args) (string, error) {
	return gallerysearch.Predicate(query, namespaces, args, func(term gallerysearch.Term, args *pgdb.Args) string {
		var fields []string
		if term.Field != "tag" {
			fields = append(fields, "strpos(lower(g.title), "+args.Add(term.Value)+") > 0")
		}
		if term.Field != "title" {
			match := gallerysearch.TagPredicate(term, "t.value", "n.name", args)
			if term.Prefix == "-" {
				// Resolve matching tags once, then probe each candidate's assignments.
				fields = append(fields, `EXISTS (SELECT 1 FROM gallery_tags gt
					WHERE gt.gallery_id = g.id AND gt.tag_id IN
					(SELECT t.id FROM tags t JOIN namespaces n ON n.id = t.namespace_id WHERE `+match+"))")
			} else {
				fields = append(fields, `g.id IN (SELECT gt.gallery_id FROM gallery_tags gt
					WHERE gt.tag_id IN (SELECT t.id FROM tags t
					JOIN namespaces n ON n.id = t.namespace_id WHERE `+match+"))")
			}
		}
		return strings.Join(fields, " OR ")
	})
}

func (r *Store) Complete(ctx context.Context, query string, cursor int) (Completion, error) {
	result := Completion{Items: []TagSuggestion{}}
	tx, err := r.db.BeginTx(ctx, pgdb.ReadOnly)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return result, err
	}
	result, term, err := gallerysearch.ActiveTerm(query, cursor, namespaces)
	if err != nil || term == nil {
		return result, err
	}
	var args pgdb.Args
	predicate := gallerysearch.TagPredicate(*term, "t.value", "n.name", &args)
	exact := args.Add(term.Value)
	rows, err := tx.Query(ctx, `SELECT n.name, t.value FROM tags t
  JOIN namespaces n ON n.id = t.namespace_id WHERE `+predicate+`
  ORDER BY t.value = `+exact+` DESC, t.value, n.name LIMIT 10`, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var suggestion TagSuggestion
		if err := rows.Scan(&suggestion.Namespace, &suggestion.Value); err != nil {
			return result, err
		}
		suggestion.Term = gallerysearch.SuggestionTerm(term.Prefix, suggestion.Namespace, suggestion.Value)
		result.Items = append(result.Items, suggestion)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
