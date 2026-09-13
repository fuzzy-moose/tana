package source

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

var pandaCandidateSuffix = regexp.MustCompile(`\[([0-9]+)]$`)

// PandaCandidateID infers an upstream ID from a source name, without verifying
// its identity. Callers decide whether this naming convention is sufficient.
func PandaCandidateID(name string, kind Kind) int64 {
	name = path.Base(name)
	if kind == Archive {
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	match := pandaCandidateSuffix.FindStringSubmatch(name)
	if match == nil {
		return 0
	}
	id, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}
