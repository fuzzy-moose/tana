package enrichment

import (
	"fmt"
	"html"
	"strings"

	"github.com/fuzzy-moose/tana/internal/local/metadata"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/tag"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func candidateID(name string, kind source.Kind) int64 {
	return source.PandaCandidateID(name, kind)
}

func metadataValues(value panda.Metadata) (metadata.Values, []error) {
	result := metadata.Values{Title: strings.TrimSpace(html.UnescapeString(value.Title))}
	if result.Title == "" {
		result.Title = strings.TrimSpace(html.UnescapeString(value.TitleJapanese))
	}
	var diagnostics []error
	seen := map[tag.Value]bool{}
	for _, text := range value.Tags {
		namespace, value, namespaced := strings.Cut(text, ":")
		if !namespaced {
			namespace, value = "other", text
		}
		v, err := tag.Normalize(tag.Value{Namespace: namespace, Value: value})
		if err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("tag %q: %w", text, err))
			continue
		}
		if !seen[v] {
			result.Tags = append(result.Tags, v)
			seen[v] = true
		}
	}
	return result, diagnostics
}
