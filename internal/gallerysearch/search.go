// Package gallerysearch shares the title/tag query language across gallery catalogs.
package gallerysearch

import (
	"errors"
	"strings"
	"unicode/utf16"
)

var ErrInvalidQuery = errors.New("invalid query")

var NamespaceShortForms = map[string]string{
	"a": "artist", "c": "character", "cos": "cosplayer", "f": "female",
	"g": "group", "l": "language", "loc": "location", "m": "male",
	"x": "mixed", "o": "other", "p": "parody", "r": "reclass",
}

type Token struct {
	Start, End int
	Raw        string
}

type Term struct {
	Prefix    string
	Namespace string
	Field     string
	Value     string
	Exact     bool
}

func querySeparator(c byte) bool {
	return c == ' ' || c == ',' || c == '\t' || c == '\r' || c == '\n'
}

// Token boundaries remain usable while a user is entering an unfinished quote.
func Tokens(query string) []Token {
	var tokens []Token
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
		tokens = append(tokens, Token{Start: start, End: i, Raw: query[start:i]})
	}
	return tokens
}

func ParseTerm(raw string, namespaces map[string]bool, partial bool) (Term, error) {
	term := Term{}
	if strings.HasPrefix(raw, "-") || strings.HasPrefix(raw, "~") {
		term.Prefix, raw = raw[:1], raw[1:]
	}
	if strings.HasPrefix(raw, "-") || strings.HasPrefix(raw, "~") {
		return term, ErrInvalidQuery
	}
	if colon := strings.IndexByte(raw, ':'); colon >= 0 && !strings.ContainsRune(raw[:colon], '"') {
		name := strings.ToLower(raw[:colon])
		raw = raw[colon+1:]
		switch {
		case name == "title" || name == "tag":
			term.Field = name
		case namespaces[name]:
			term.Field, term.Namespace = "tag", name
		case NamespaceShortForms[name] != "":
			term.Field, term.Namespace = "tag", NamespaceShortForms[name]
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
		term.Value = value.String()
	} else {
		if strings.ContainsAny(raw, "\":") {
			return term, ErrInvalidQuery
		}
		term.Value = raw
	}
	if before, ok := strings.CutSuffix(term.Value, "$"); ok {
		term.Exact, term.Value = true, before
		if term.Field == "title" {
			return term, ErrInvalidQuery
		}
		term.Field = "tag"
	}
	if !quoted && strings.ContainsRune(term.Value, '$') {
		return term, ErrInvalidQuery
	}
	term.Value = strings.ToLower(strings.ReplaceAll(term.Value, "_", " "))
	if term.Value == "" && !partial {
		return term, ErrInvalidQuery
	}
	return term, nil
}

// Predicate binds every search value separately; match supplies trusted SQL for one term.
func Predicate(query string, namespaces map[string]bool, match func(Term) (string, []any)) (string, []any, error) {
	var required, alternatives []string
	var requiredArgs, alternativeArgs []any
	for _, token := range Tokens(query) {
		term, err := ParseTerm(token.Raw, namespaces, false)
		if err != nil {
			return "", nil, err
		}
		predicate, args := match(term)
		predicate = "(" + predicate + ")"
		if term.Prefix == "~" {
			alternatives = append(alternatives, predicate)
			alternativeArgs = append(alternativeArgs, args...)
		} else {
			if term.Prefix == "-" {
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

// TagPredicate accepts fixed, caller-owned column expressions only.
func TagPredicate(term Term, valueColumn, namespaceColumn string) (string, []any) {
	var predicate string
	var args []any
	if term.Exact {
		predicate, args = valueColumn+" = ?", []any{term.Value}
	} else {
		predicate = "(instr(" + valueColumn + ", ?) = 1 OR instr(" + valueColumn + ", ' ' || ?) > 0 OR instr(" + valueColumn + ", '-' || ?) > 0 OR instr(" + valueColumn + ", '.' || ?) > 0)"
		args = []any{term.Value, term.Value, term.Value, term.Value}
	}
	if term.Namespace != "" {
		predicate += " AND " + namespaceColumn + " = ?"
		args = append(args, term.Namespace)
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

// ActiveTerm uses browser UTF-16 offsets and replaces the entire active token,
// while matching only its value before the cursor. Incomplete queries are allowed.
func ActiveTerm(query string, cursor int, namespaces map[string]bool) (Completion, *Term, error) {
	result := Completion{Items: []TagSuggestion{}}
	units := utf16.Encode([]rune(query))
	if cursor < 0 || cursor > len(units) {
		return result, nil, ErrInvalidQuery
	}
	position := len(string(utf16.Decode(units[:cursor])))
	for _, token := range Tokens(query) {
		if token.Start >= position || position > token.End {
			continue
		}
		result.Start = len(utf16.Encode([]rune(query[:token.Start])))
		result.End = len(utf16.Encode([]rune(query[:token.End])))
		term, err := ParseTerm(query[token.Start:position], namespaces, true)
		if err != nil || term.Value == "" || term.Field == "title" {
			return result, nil, nil
		}
		term.Exact = false
		return result, &term, nil
	}
	return result, nil, nil
}

func SuggestionTerm(prefix, namespace, rawValue string) string {
	value := rawValue + "$"
	if strings.ContainsAny(rawValue, " \t\r\n,:\"\\$") || strings.HasPrefix(rawValue, "-") || strings.HasPrefix(rawValue, "~") {
		value = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
	}
	return prefix + namespace + ":" + value
}
