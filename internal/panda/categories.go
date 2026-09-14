package panda

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidCategory = errors.New("invalid Panda gallery category")

// NormalizeCategories validates gallery types and removes duplicate selections.
func NormalizeCategories(categories []string) ([]string, error) {
	result := make([]string, 0, len(categories))
	seen := make(map[string]bool, len(categories))
	for _, category := range categories {
		category = strings.ToLower(strings.TrimSpace(category))
		switch category {
		case "doujinshi", "manga", "artist cg", "game cg", "western", "non-h", "misc", "cosplay", "asian porn", "image set":
		default:
			return nil, fmt.Errorf("%w: %q", ErrInvalidCategory, category)
		}
		if !seen[category] {
			result = append(result, category)
			seen[category] = true
		}
	}
	return result, nil
}
