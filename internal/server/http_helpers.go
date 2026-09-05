package server

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
)

// DecodeJSON requires application/json and a single, bounded JSON value with known fields.
func DecodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	contentTypes := r.Header.Values("Content-Type")
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if len(contentTypes) != 1 || err != nil || mediaType != "application/json" {
		WriteJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "unsupported_media_type"})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err = d.Decode(target)
	if err == nil {
		var extra any
		if d.Decode(&extra) == io.EOF {
			return true
		}
	}
	WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
	return false
}

// WriteJSON writes an uncached JSON response.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
