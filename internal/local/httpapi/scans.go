package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/local/scan"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleRequestScan(scans *scan.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			LibraryID int64 `json:"library_id"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := scans.Request(r.Context(), input.LibraryID); err != nil {
			if errors.Is(err, scan.ErrActive) {
				server.WriteJSON(w, http.StatusConflict, map[string]string{"error": "scan_active"})
			} else {
				writeLibraryError(w, r, err)
			}
			return
		}
		w.Header().Set("Location", "/api/scans/status")
		server.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	})
}

func HandleScanStatus(scans *scan.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, http.StatusOK, scans.Status())
	})
}
