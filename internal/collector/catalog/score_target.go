package catalog

import (
	"errors"
	"strings"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/gallerysearch"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var ErrTagNotFound = errors.New("tag not found")

// scoreTarget is the tag, uploader or category that a pin names. For a tag,
// namespace and value are the lowercase keys of catalog_tags (namespace,
// value_lower).
type scoreTarget struct {
	kind, namespace, value string
}

// parseScoreTarget applies the value rules of pins and of the tag weight
// lookup, before a tag's namespace short form is expanded: the value is
// trimmed, a tag is namespace:value folded like replaceTags, a category is a
// Panda gallery category, and an uploader is kept as given. ok is false for an
// invalid kind or value.
func parseScoreTarget(kind, value string) (target scoreTarget, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return scoreTarget{}, false
	}
	switch kind {
	case collectorapi.ScoreKindTag:
		namespace, tag, found := strings.Cut(value, ":")
		namespace, tag = strings.TrimSpace(namespace), strings.TrimSpace(tag)
		if !found || namespace == "" || tag == "" {
			return scoreTarget{}, false
		}
		return scoreTarget{kind, strings.ToLower(namespace), strings.ToLower(strings.ReplaceAll(tag, "_", " "))}, true
	case collectorapi.ScoreKindCategory:
		categories, err := panda.NormalizeCategories([]string{value})
		if err != nil {
			return scoreTarget{}, false
		}
		return scoreTarget{kind: kind, value: categories[0]}, true
	case collectorapi.ScoreKindUploader:
		return scoreTarget{kind: kind, value: value}, true
	}
	return scoreTarget{}, false
}

// parseSavedScoreTarget is parseScoreTarget for saving a pin:
// a tag's namespace that is a short form of the search language becomes its
// full namespace. Panda has no namespace named like a short form, so the
// expansion is unconditional.
func parseSavedScoreTarget(kind, value string) (scoreTarget, bool) {
	target, ok := parseScoreTarget(kind, value)
	if full := gallerysearch.NamespaceShortForms[target.namespace]; ok && target.kind == collectorapi.ScoreKindTag && full != "" {
		target.namespace = full
	}
	return target, ok
}
