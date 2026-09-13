package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var ErrInvalidGalleryReference = errors.New("invalid gallery reference")

func (s *Service) GalleryURL(ref panda.GalleryRef) string {
	return s.galleryOrigin + "/g/" + strconv.FormatInt(ref.ID, 10) + "/" + url.PathEscape(ref.Token) + "/"
}

// Lookup reads the inventory and retained metadata independently of the catalog
// projection, including galleries whose upstream availability has changed.
func (s *Service) Lookup(ctx context.Context, id int64) (collectorapi.GalleryLookupResult, error) {
	if id <= 0 {
		return collectorapi.GalleryLookupResult{}, ErrInvalidGalleryReference
	}
	result := collectorapi.GalleryLookupResult{GalleryID: id}
	var body []byte
	var refreshedAt sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT r.token, m.body, m.refreshed_at
		FROM gallery_refs r LEFT JOIN gallery_metadata m ON m.gallery_id = r.gallery_id
		WHERE r.gallery_id = ?`, id).Scan(&result.Token, &body, &refreshedAt)
	if err != nil {
		return collectorapi.GalleryLookupResult{}, err
	}
	result.URL = s.GalleryURL(panda.GalleryRef{ID: id, Token: result.Token})
	if body != nil {
		var value panda.Metadata
		if err := json.Unmarshal(body, &value); err != nil {
			return collectorapi.GalleryLookupResult{}, fmt.Errorf("decode collected metadata: %w", err)
		}
		at := time.UnixMilli(refreshedAt.Int64).UTC()
		result.Metadata, result.RefreshedAt = &value, &at
	}
	return result, nil
}
