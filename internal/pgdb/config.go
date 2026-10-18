package pgdb

import (
	"fmt"
	"strconv"
	"strings"
)

// ConfigFromEnv reads a required database URL from urlKey and a positive pool
// size from sizeKey, which defaults to fallback.
func ConfigFromEnv(getenv func(string) string, urlKey, sizeKey string, fallback int32) (string, int32, error) {
	url := strings.TrimSpace(getenv(urlKey))
	if url == "" {
		return "", 0, fmt.Errorf("%s must be nonempty", urlKey)
	}
	size := fallback
	if raw := getenv(sizeKey); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed < 1 {
			return "", 0, fmt.Errorf("%s must be a positive integer", sizeKey)
		}
		size = int32(parsed)
	}
	return url, size, nil
}
