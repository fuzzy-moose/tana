package gallery

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf16"
)

var ErrInvalidQuery = errors.New("invalid query")

var namespaceShortForms = map[string]string{
	"a": "artist", "c": "character", "cos": "cosplayer", "f": "female",
	"g": "group", "l": "language", "loc": "location", "m": "male",
	"x": "mixed", "o": "other", "p": "parody", "r": "reclass",
}

type searchToken struct {
	start, end int
	raw        string
}

type searchTerm struct {
	prefix    string
	namespace string
	field     string
	value     string
	exact     bool
}

func querySeparator(c byte) bool {
	return c == ' ' || c == ',' || c == '\t' || c == '\r' || c == '\n'
}

// Token boundaries remain usable while a user is entering an unfinished quote.
func searchTokens(query string) []searchToken {
	var tokens []searchToken
	for i := 0; i < len(query); {
		if querySeparator(query[i]) {
			i++
			continue
		}
		start, quoted := i, false
		for i < len(query) {
			if quoted && query[i] == '\\' && i+1 < len(query) {
				i += 2
				continue
			}
			if query[i] == '"' {
				quoted = !quoted
			} else if !quoted && querySeparator(query[i]) {
				break
			}
			i++
		}
		tokens = append(tokens, searchToken{start: start, end: i, raw: query[start:i]})
	}
	return tokens
}

func parseSearchTerm(raw string, namespaces map[string]bool, partial bool) (searchTerm, error) {
	term := searchTerm{}
	if strings.HasPrefix(raw, "-") || strings.HasPrefix(raw, "~") {
		term.prefix, raw = raw[:1], raw[1:]
	}
	if strings.HasPrefix(raw, "-") || strings.HasPrefix(raw, "~") {
		return term, ErrInvalidQuery
	}
	if colon := strings.IndexByte(raw, ':'); colon >= 0 && !strings.ContainsRune(raw[:colon], '"') {
		name := strings.ToLower(raw[:colon])
		raw = raw[colon+1:]
		switch {
		case name == "title" || name == "tag":
			term.field = name
		case namespaces[name]:
			term.field, term.namespace = "tag", name
		case namespaceShortForms[name] != "":
			term.field, term.namespace = "tag", namespaceShortForms[name]
		default:
			return term, ErrInvalidQuery
		}
	}
	quoted := strings.HasPrefix(raw, "\"")
	if quoted {
		var value strings.Builder
		closed := false
		for i := 1; i < len(raw); i++ {
			switch raw[i] {
			case '\\':
				i++
				if i == len(raw) && partial {
					break
				}
				if i == len(raw) || raw[i] != '\\' && raw[i] != '"' {
					return term, ErrInvalidQuery
				}
				value.WriteByte(raw[i])
			case '"':
				if i != len(raw)-1 {
					return term, ErrInvalidQuery
				}
				closed = true
			default:
				value.WriteByte(raw[i])
			}
		}
		if !closed && !partial {
			return term, ErrInvalidQuery
		}
		term.value = value.String()
	} else {
		if strings.ContainsAny(raw, "\":") {
			return term, ErrInvalidQuery
		}
		term.value = raw
	}
	if before, ok := strings.CutSuffix(term.value, "$"); ok {
		term.exact, term.value = true, before
		if term.field == "title" {
			return term, ErrInvalidQuery
		}
		term.field = "tag"
	}
	if (!quoted || term.exact) && strings.ContainsRune(term.value, '$') {
		return term, ErrInvalidQuery
	}
	term.value = strings.ToLower(strings.ReplaceAll(term.value, "_", " "))
	if term.value == "" && !partial {
		return term, ErrInvalidQuery
	}
	return term, nil
}

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

// Only fixed SQL fragments are composed; every search value is bound separately.
// Sharing the predicate keeps pagination counts and results identical.
func searchPredicate(query string, namespaces map[string]bool) (string, []any, error) {
	var required, alternatives []string
	var requiredArgs, alternativeArgs []any
	for _, token := range searchTokens(query) {
		term, err := parseSearchTerm(token.raw, namespaces, false)
		if err != nil {
			return "", nil, err
		}
		var fields []string
		var args []any
		if term.field != "tag" {
			fields = append(fields, "instr(unicode_lower(g.title), ?) > 0")
			args = append(args, term.value)
		}
		if term.field != "title" {
			match, values := tagPredicate(term)
			fields = append(fields, `EXISTS (SELECT 1 FROM gallery_tags gt
				JOIN tags t ON t.id = gt.tag_id JOIN namespaces n ON n.id = t.namespace_id
				WHERE gt.gallery_id = g.id AND `+match+")")
			args = append(args, values...)
		}
		predicate := "(" + strings.Join(fields, " OR ") + ")"
		if term.prefix == "~" {
			alternatives = append(alternatives, predicate)
			alternativeArgs = append(alternativeArgs, args...)
		} else {
			if term.prefix == "-" {
				predicate = "NOT " + predicate
			}
			required = append(required, predicate)
			requiredArgs = append(requiredArgs, args...)
		}
	}
	if len(alternatives) > 0 {
		required = append(required, "("+strings.Join(alternatives, " OR ")+")")
	}
	if len(required) == 0 {
		return "1", nil, nil
	}
	return strings.Join(required, " AND "), append(requiredArgs, alternativeArgs...), nil
}

func tagPredicate(term searchTerm) (string, []any) {
	var predicate string
	var args []any
	if term.exact {
		predicate, args = "t.value = ?", []any{term.value}
	} else {
		predicate = `(instr(t.value, ?) = 1 OR instr(t.value, ' ' || ?) > 0
			OR instr(t.value, '-' || ?) > 0 OR instr(t.value, '.' || ?) > 0)`
		args = []any{term.value, term.value, term.value, term.value}
	}
	if term.namespace != "" {
		predicate += " AND n.name = ?"
		args = append(args, term.namespace)
	}
	return predicate, args
}

type TagSuggestion struct {
	Namespace string `json:"namespace"`
	Value     string `json:"value"`
	Term      string `json:"term"`
}

type Completion struct {
	Start int             `json:"start"`
	End   int             `json:"end"`
	Items []TagSuggestion `json:"items"`
}

// Complete uses browser UTF-16 cursor offsets and replaces the entire active
// token, while matching only its value before the cursor.
func (r *SQLiteRepository) Complete(ctx context.Context, query string, cursor int) (Completion, error) {
	result := Completion{Items: []TagSuggestion{}}
	units := utf16.Encode([]rune(query))
	if cursor < 0 || cursor > len(units) {
		return result, ErrInvalidQuery
	}
	position := len(string(utf16.Decode(units[:cursor])))
	var active *searchToken
	for _, token := range searchTokens(query) {
		if token.start < position && position <= token.end {
			active = &token
			break
		}
	}
	if active == nil {
		return result, nil
	}
	result.Start = len(utf16.Encode([]rune(query[:active.start])))
	result.End = len(utf16.Encode([]rune(query[:active.end])))
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return result, err
	}
	term, err := parseSearchTerm(query[active.start:position], namespaces, true)
	if err != nil || term.value == "" || term.field == "title" {
		return result, nil
	}
	term.exact = false
	predicate, args := tagPredicate(term)
	args = append(args, term.value)
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
		value := suggestion.Value + "$"
		if strings.ContainsAny(value, " \"\\") || strings.HasPrefix(value, "-") {
			value = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
		}
		suggestion.Term = term.prefix + suggestion.Namespace + ":" + value
		result.Items = append(result.Items, suggestion)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, tx.Commit()
}
