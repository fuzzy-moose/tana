package httpapi

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/scan"
)

func scanStatus(t *testing.T, h http.Handler) scan.Status {
	t.Helper()
	var status scan.Status
	if err := json.Unmarshal(request(t, h, "GET", "/api/scans/status", "", 200).Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func awaitScan(t *testing.T, h http.Handler) scan.Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status := scanStatus(t, h)
		if status.FinishedAt != nil {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan did not finish: %+v", status)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScanAPISingleAndAllLibraries(t *testing.T) {
	h := testHandler(t)
	if status := scanStatus(t, h); status.Phase != "idle" || status.StartedAt != nil || status.FinishedAt != nil {
		t.Fatalf("initial status: %+v", status)
	}
	var ids []int64
	for _, name := range []string{"A", "B"} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "1.jpg"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		l := decodeLibrary(t, request(t, h, "POST", "/api/libraries", registrationJSON(t, name, root), 201))
		ids = append(ids, l.ID)
	}
	w := request(t, h, "POST", "/api/scans", `{"library_id":`+strconv.FormatInt(ids[0], 10)+`}`, 202)
	if w.Header().Get("Location") != "/api/scans/status" || strings.TrimSpace(w.Body.String()) != `{"status":"accepted"}` {
		t.Fatalf("acceptance: %v %s", w.Header(), w.Body)
	}
	status := awaitScan(t, h)
	if status.Phase != "completed" || status.LibraryID != ids[0] || status.LibrariesTotal != 1 || status.Imported != 1 || status.GalleriesCreated != 1 {
		t.Fatalf("single-library scan: %+v", status)
	}
	request(t, h, "POST", "/api/scans", `{}`, 202)
	status = awaitScan(t, h)
	if status.Phase != "completed" || status.LibraryID != 0 || status.LibrariesTotal != 2 || status.Discovered != 1 || status.Imported != 1 {
		t.Fatalf("all-library scan: %+v", status)
	}
	request(t, h, "POST", "/api/scans", `{}`, 202)
	status = awaitScan(t, h)
	if status.Discovered != 0 || status.Imported != 0 || status.GalleriesCreated != 0 {
		t.Fatalf("repeat scan duplicated imports: %+v", status)
	}
}

func TestScanAPIValidation(t *testing.T) {
	h := testHandler(t)
	for _, tc := range []struct {
		method, path, body, code string
		status                   int
	}{
		{"POST", "/api/scans", "{", "invalid_json", 400},
		{"POST", "/api/scans", `{"library_id":"12"}`, "invalid_json", 400},
		{"POST", "/api/scans", `{"extra":1}`, "invalid_json", 400},
		{"POST", "/api/scans", `{"library_id":999}`, "not_found", 404},
		{"GET", "/api/scans", "", "method_not_allowed", 405},
		{"POST", "/api/scans/status", `{}`, "method_not_allowed", 405},
	} {
		w := request(t, h, tc.method, tc.path, tc.body, tc.status)
		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result["error"] != tc.code {
			t.Fatalf("unexpected error: %s, %v", w.Body, err)
		}
		if tc.status == 405 && w.Header().Get("Allow") == "" {
			t.Fatal("missing Allow header")
		}
	}
	if status := scanStatus(t, h); status.Phase != "idle" {
		t.Fatalf("invalid request started scan: %+v", status)
	}
	request(t, h, "POST", "/api/scans", `{}`, 202)
	if status := awaitScan(t, h); status.Phase != "completed" || status.LibrariesTotal != 0 {
		t.Fatalf("empty scan: %+v", status)
	}
}

type scanGateFS struct {
	fs.FS
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (f *scanGateFS) ReadDir(name string) ([]fs.DirEntry, error) {
	f.once.Do(func() { close(f.entered); <-f.release })
	return fs.ReadDir(f.FS, name)
}

func TestScanAPIRejectsOverlappingRequests(t *testing.T) {
	root := t.TempDir()
	gate := &scanGateFS{FS: os.DirFS(root), entered: make(chan struct{}), release: make(chan struct{})}
	h := testHandlerWithScanFS(t, func(string) fs.FS { return gate })
	defer close(gate.release)
	l := decodeLibrary(t, request(t, h, "POST", "/api/libraries", registrationJSON(t, "Comics", root), 201))
	request(t, h, "POST", "/api/scans", `{}`, 202)
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not start")
	}
	if status := scanStatus(t, h); status.Phase != "discovering" || status.FinishedAt != nil {
		t.Fatalf("active status: %+v", status)
	}
	for _, body := range []string{`{}`, `{"library_id":` + strconv.FormatInt(l.ID, 10) + `}`} {
		w := request(t, h, "POST", "/api/scans", body, 409)
		if strings.TrimSpace(w.Body.String()) != `{"error":"scan_active"}` {
			t.Fatalf("conflict response: %s", w.Body)
		}
	}
}
