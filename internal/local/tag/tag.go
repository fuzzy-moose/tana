// Package tag owns the shared namespace and tag vocabulary for galleries.
package tag

import (
	"errors"
	"strings"
)

var ErrInvalid = errors.New("namespace must contain only ASCII letters; tag must contain only ASCII letters, digits, spaces, hyphens, and dots")

type Namespace struct {
	ID   string
	Name string
}

type Tag struct {
	ID        string
	Namespace Namespace
	Value     string
}

// Value identifies a tag before resolving its shared catalog identity.
type Value struct {
	Namespace string
	Value     string
}

// Normalize is shared by providers and all writes to the tag catalog.
func Normalize(v Value) (Value, error) {
	namespace, err := normalize(v.Namespace, "")
	if err != nil {
		return Value{}, err
	}
	value, err := normalize(v.Value, "0123456789 .-")
	if err != nil {
		return Value{}, err
	}
	return Value{Namespace: namespace, Value: value}, nil
}

func normalize(value, extraCharacters string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalid
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || strings.ContainsRune(extraCharacters, c)) {
			return "", ErrInvalid
		}
	}
	return strings.ToLower(value), nil
}
