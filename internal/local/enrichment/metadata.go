package enrichment

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/fuzzy-moose/tana/internal/local/metadata"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/tag"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var candidateSuffix = regexp.MustCompile(`\[([0-9]+)]$`)

func candidateID(name string, kind source.Kind) int64 {
	name = path.Base(name)
	if kind == source.Archive {
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	match := candidateSuffix.FindStringSubmatch(name)
	if match == nil {
		return 0
	}
	id, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func metadataValues(value panda.Metadata) (metadata.Values, []error) {
	result := metadata.Values{Title: strings.TrimSpace(value.Title)}
	if result.Title == "" {
		result.Title = strings.TrimSpace(value.TitleJapanese)
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
