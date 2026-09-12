package gallery

import (
	"context"
	"database/sql"
	"strings"

	"github.com/fuzzy-moose/tana/internal/gallerysearch"
)

var ErrInvalidQuery = gallerysearch.ErrInvalidQuery
var namespaceShortForms = gallerysearch.NamespaceShortForms

type TagSuggestion = gallerysearch.TagSuggestion
type Completion = gallerysearch.Completion

func searchNamespaces(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	names := make(map[string]bool)
	for _, name := range namespaceShortForms {
		names[name] = true
	}
	rows, err := tx.QueryContext(ctx, "SELECT name FROM namespaces")
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
func searchPredicate(query string, namespaces map[string]bool) (string, []any, error) {
	return gallerysearch.Predicate(query, namespaces, func(term gallerysearch.Term) (string, []any) {
		var fields []string
		var args []any
		if term.Field != "tag" {
			fields = append(fields, "instr(unicode_lower(g.title), ?) > 0")
			args = append(args, term.Value)
		}
		if term.Field != "title" {
			match, values := gallerysearch.TagPredicate(term, "t.value", "n.name")
			fields = append(fields, `EXISTS (SELECT 1 FROM gallery_tags gt
    JOIN tags t ON t.id = gt.tag_id JOIN namespaces n ON n.id = t.namespace_id
    WHERE gt.gallery_id = g.id AND `+match+")")
			args = append(args, values...)
		}
		return strings.Join(fields, " OR "), args
	})
}

func (r *SQLiteRepository) Complete(ctx context.Context, query string, cursor int) (Completion, error) {
	result := Completion{Items: []TagSuggestion{}}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return result, err
	}
	result, term, err := gallerysearch.ActiveTerm(query, cursor, namespaces)
	if err != nil || term == nil {
		return result, err
	}
	predicate, args := gallerysearch.TagPredicate(*term, "t.value", "n.name")
	args = append(args, term.Value)
	rows, err := tx.QueryContext(ctx, `SELECT n.name, t.value FROM tags t
  JOIN namespaces n ON n.id = t.namespace_id WHERE `+predicate+`
  ORDER BY t.value = ? DESC, t.value, n.name LIMIT 10`, args...)
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
	return result, tx.Commit()
}
