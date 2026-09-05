package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/gallery"
)

func TestGalleryDetailTags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata string
		want     []gallery.DetailTag
	}{
		{name: "no tags", want: []gallery.DetailTag{}},
		{
			name:     "grouped alphabetical tags",
			metadata: "Tags: language:japanese, artist:zeta, all ages, artist:alpha, custom:example\nAttribution\n",
			want: []gallery.DetailTag{
				{Namespace: "artist", Value: "alpha"},
				{Namespace: "artist", Value: "zeta"},
				{Namespace: "custom", Value: "example"},
				{Namespace: "language", Value: "japanese"},
				{Namespace: "other", Value: "all ages"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler(t)
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "1.png"), []byte("image"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.metadata != "" {
				if err := os.WriteFile(filepath.Join(root, "galleryinfo.txt"), []byte(tc.metadata), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			request(t, h, "POST", "/api/libraries", registrationJSON(t, "Library", root), 201)
			request(t, h, "POST", "/api/scans", `{}`, 202)
			awaitScan(t, h)
			var listing gallery.Listing
			if err := json.Unmarshal(request(t, h, "GET", "/api/galleries", "", 200).Body.Bytes(), &listing); err != nil {
				t.Fatal(err)
			}
			if len(listing.Items) != 1 {
				t.Fatalf("expected one gallery, got %+v", listing)
			}
			var detail gallery.Detail
			if err := json.Unmarshal(request(t, h, "GET", "/api/galleries/"+strconv.FormatInt(listing.Items[0].ID, 10), "", 200).Body.Bytes(), &detail); err != nil {
				t.Fatal(err)
			}
			if detail.Summary != listing.Items[0] || !reflect.DeepEqual(detail.Tags, tc.want) {
				t.Fatalf("unexpected gallery detail: %+v; want tags %+v", detail, tc.want)
			}
			if len(tc.want) > 0 {
				var filtered gallery.Listing
				if err := json.Unmarshal(request(t, h, "GET", "/api/galleries?q="+url.QueryEscape("a:alpha$ custom:example$"), "", 200).Body.Bytes(), &filtered); err != nil {
					t.Fatal(err)
				}
				if filtered.Total != 1 || filtered.Items[0].ID != detail.ID {
					t.Fatalf("tag query did not match imported gallery: %+v", filtered)
				}
				var completion gallery.Completion
				if err := json.Unmarshal(request(t, h, "GET", "/api/gallery-search/completions?q=a%3Aal&cursor=4", "", 200).Body.Bytes(), &completion); err != nil {
					t.Fatal(err)
				}
				if len(completion.Items) != 1 || completion.Items[0].Term != "artist:alpha$" {
					t.Fatalf("tag completion: %+v", completion)
				}
			}
		})
	}
}

func TestGallerySearchReturnsGenericErrors(t *testing.T) {
	h := testHandler(t)
	for _, path := range []string{
		"/api/galleries?q=" + url.QueryEscape(`title:blue$`),
		"/api/galleries?q=" + url.QueryEscape(`"unclosed`),
		"/api/galleries?q=unknown%3Ablue",
		"/api/gallery-search/completions?q=blue&cursor=99",
		"/api/gallery-search/completions?q=blue&cursor=bad",
	} {
		var body map[string]string
		if err := json.Unmarshal(request(t, h, "GET", path, "", 400).Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(body, map[string]string{"error": "invalid_query"}) {
			t.Fatalf("expected generic error, got %v", body)
		}
	}
	request(t, h, "GET", "/api/gallery-search/completions?q=a%3A%22art&cursor=6", "", 200)
	request(t, h, "POST", "/api/gallery-search/completions", "", 405)
}

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
		request(t, h, "POST", "/api/scans", `{"library_id":`+strconv.FormatInt(l.ID, 10)+`}`, 202)
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
		if g.ID == 0 || g.PageCount != 1 {
			t.Fatalf("missing imported gallery: %+v", listing)
		}
		path := "/api/galleries/" + strconv.FormatInt(g.ID, 10)
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
	request(t, h, "GET", "/api/galleries/"+strconv.FormatInt(listing.Items[0].ID, 10)+"/pages/1/image", "", 422)
}
