package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/gallery"
)

func TestGalleryAPIReadsDirectoryAndArchiveImages(t *testing.T) {
	h := testHandler(t)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	for _, archive := range []bool{false, true} {
		root := t.TempDir()
		name := filepath.Join(root, "1.png")
		data := encoded.Bytes()
		if archive {
			name = filepath.Join(root, "Story.cbz")
			var buffer bytes.Buffer
			w := zip.NewWriter(&buffer)
			entry, err := w.Create("chapter/1.png")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write(data); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			data = buffer.Bytes()
		}
		if err := os.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
		l := decodeLibrary(t, request(t, h, "POST", "/api/libraries", registrationJSON(t, "Library", root), 201))
		request(t, h, "POST", "/api/scans", `{"library_id":"`+l.ID+`"}`, 202)
		awaitScan(t, h)
		var listing gallery.Listing
		if err := json.Unmarshal(request(t, h, "GET", "/api/galleries", "", 200).Body.Bytes(), &listing); err != nil {
			t.Fatal(err)
		}
		var g gallery.Summary
		for _, item := range listing.Items {
			if (archive && item.Title == "Story") || (!archive && item.Title == filepath.Base(root)) {
				g = item
			}
		}
		if g.ID == "" || g.PageCount != 1 {
			t.Fatalf("missing imported gallery: %+v", listing)
		}
		path := "/api/galleries/" + g.ID
		request(t, h, "GET", path, "", 200)
		imagePath := path + "/pages/1/image"
		w := request(t, h, "GET", imagePath, "", 200)
		if w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), encoded.Bytes()) {
			t.Fatalf("incorrect image response: %v", w.Header())
		}
		if head := request(t, h, "HEAD", imagePath, "", 200); head.Body.Len() != 0 {
			t.Fatal("HEAD returned image bytes")
		}
		request(t, h, "GET", path+"/pages/2/image", "", 404)
		request(t, h, "GET", path+"/pages/0/image", "", 404)
		request(t, h, "POST", imagePath, "", 405)
		if err := os.Remove(name); err != nil {
			t.Fatal(err)
		}
		request(t, h, "GET", imagePath, "", 422)
		// Missing files do not remove galleries or their cataloged pages.
		request(t, h, "GET", path, "", 200)
		if err := os.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
		request(t, h, "GET", imagePath, "", 200)
		if archive {
			if err := os.WriteFile(name, []byte("broken archive"), 0o600); err != nil {
				t.Fatal(err)
			}
			request(t, h, "GET", imagePath, "", 422)
		}
	}
	request(t, h, "GET", "/api/galleries/missing", "", 404)
	for _, query := range []string{"page=0", "page=-1", "page_size=101", "page_size=invalid"} {
		request(t, h, "GET", "/api/galleries?"+query, "", 400)
	}
}

func TestGalleryImageRejectsSymlinkReplacementOutsideLibrary(t *testing.T) {
	h := testHandler(t)
	root := t.TempDir()
	name := filepath.Join(root, "1.png")
	if err := os.WriteFile(name, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	decodeLibrary(t, request(t, h, "POST", "/api/libraries", registrationJSON(t, "Library", root), 201))
	request(t, h, "POST", "/api/scans", `{}`, 202)
	awaitScan(t, h)
	var listing gallery.Listing
	if err := json.Unmarshal(request(t, h, "GET", "/api/galleries", "", 200).Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("outside library"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, name); err != nil {
		t.Fatal(err)
	}
	request(t, h, "GET", "/api/galleries/"+listing.Items[0].ID+"/pages/1/image", "", 422)
}
