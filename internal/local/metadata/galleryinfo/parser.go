// Package galleryinfo parses the local galleryinfo.txt metadata format.
package galleryinfo

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fuzzy-moose/tana/internal/local/tag"
)

type Document struct {
	Title            string
	UploadTime       time.Time
	UploadedBy       string
	Downloaded       time.Time
	Tags             []tag.Value
	UploaderComments string
	Attribution      string
}

// Parse retains valid fields when other fields fail. The last nonblank line is
// always attribution, regardless of its text; see docs/galleryinfo-format.md.
func Parse(r io.Reader) (Document, []error) {
	data, err := io.ReadAll(r)
	var diagnostics []error
	if err != nil {
		diagnostics = append(diagnostics, fmt.Errorf("read galleryinfo: %w", err))
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	var doc Document
	if len(lines) == 0 {
		return doc, append(diagnostics, fmt.Errorf("empty galleryinfo: missing attribution footer"))
	}
	doc.Attribution = lines[len(lines)-1]
	lines = lines[:len(lines)-1]
	seen := map[string]bool{}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		label, value, ok := strings.Cut(line, ":")
		if !ok {
			diagnostics = append(diagnostics, fmt.Errorf("line %d: expected field header", i+1))
			continue
		}
		value = strings.TrimSpace(value)
		if label == "Uploader's Comments" {
			comments := lines[i+1:]
			for len(comments) > 0 && strings.TrimSpace(comments[0]) == "" {
				comments = comments[1:]
			}
			for len(comments) > 0 && strings.TrimSpace(comments[len(comments)-1]) == "" {
				comments = comments[:len(comments)-1]
			}
			doc.UploaderComments = strings.Join(comments, "\n")
			break
		}
		switch label {
		case "Title", "Upload Time", "Uploaded By", "Downloaded", "Tags":
		default:
			continue
		}
		if seen[label] {
			continue
		}
		if value == "" || !utf8.ValidString(value) {
			diagnostics = append(diagnostics, fmt.Errorf("line %d: invalid %s", i+1, label))
			continue
		}
		valid := true
		switch label {
		case "Title":
			doc.Title = value
		case "Uploaded By":
			doc.UploadedBy = value
		case "Upload Time", "Downloaded":
			timestamp, err := time.Parse("2006-01-02 15:04", value)
			if err != nil {
				diagnostics = append(diagnostics, fmt.Errorf("line %d: %s: %w", i+1, label, err))
				valid = false
			} else if label == "Upload Time" {
				doc.UploadTime = timestamp
			} else {
				doc.Downloaded = timestamp
			}
		case "Tags":
			var problems []error
			doc.Tags, problems = parseTags(value)
			for _, problem := range problems {
				diagnostics = append(diagnostics, fmt.Errorf("line %d: Tags: %w", i+1, problem))
			}
			valid = len(doc.Tags) > 0
		}
		seen[label] = valid
	}
	if len(seen) == 0 {
		diagnostics = append(diagnostics, fmt.Errorf("galleryinfo contains no recognized fields"))
	}
	return doc, diagnostics
}

func parseTags(value string) ([]tag.Value, []error) {
	var tags []tag.Value
	var diagnostics []error
	seen := map[tag.Value]bool{}
	for text := range strings.SplitSeq(value, ",") {
		namespace, value, namespaced := strings.Cut(text, ":")
		if !namespaced {
			namespace, value = "other", text
		}
		v, err := tag.Normalize(tag.Value{Namespace: namespace, Value: value})
		if err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("tag %q: %w", strings.TrimSpace(text), err))
			continue
		}
		if !seen[v] {
			tags = append(tags, v)
			seen[v] = true
		}
	}
	return tags, diagnostics
}
