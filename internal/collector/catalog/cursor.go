package catalog

import (
	"encoding/base64"
	"encoding/json"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

type catalogCursor struct {
	Posted    int64 `json:"posted"`
	GalleryID int64 `json:"gallery_id"`
	Before    bool  `json:"before,omitempty"`
	Inclusive bool  `json:"inclusive,omitempty"`
}

func decodeCursor(value string) (catalogCursor, error) {
	var cursor catalogCursor
	if value == "" {
		return cursor, nil
	}
	if len(value) > 256 {
		return cursor, ErrInvalidPagination
	}
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || json.Unmarshal(body, &cursor) != nil || cursor.GalleryID < 1 {
		return catalogCursor{}, ErrInvalidPagination
	}
	return cursor, nil
}

func (c catalogCursor) encode() string {
	body, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(body)
}

func itemCursor(item collectorapi.CatalogItem, before bool) string {
	return (catalogCursor{Posted: item.PostedAt.Unix(), GalleryID: item.GalleryID, Before: before}).encode()
}
