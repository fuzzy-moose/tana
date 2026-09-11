package downloads

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

var errExpiredURL = errors.New("archive URL expired")
var errInvalidZIP = errors.New("invalid ZIP archive")

type Transfer interface {
	Copy(context.Context, string, io.Writer) (int64, error)
}

type HTTPTransfer struct{ client *http.Client }

// NewHTTPTransfer never shares Panda cookies with archive hosts. Its transport
// may still use the collector's shared ban state.
func NewHTTPTransfer(client *http.Client) *HTTPTransfer {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Minute}
	}
	c := *client
	c.Jar = nil
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !panda.ValidArchiveURL(req.URL) {
			return panda.ErrArchivePage
		}
		return nil
	}
	return &HTTPTransfer{client: &c}
}

func (t *HTTPTransfer) Copy(ctx context.Context, address string, dest io.Writer) (int64, error) {
	u, err := url.Parse(address)
	if err != nil || !panda.ValidArchiveURL(u) {
		return 0, panda.ErrArchivePage
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Tana")
	resp, err := t.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusNotFound, http.StatusGone:
		return 0, errExpiredURL
	}
	if resp.StatusCode != http.StatusOK {
		return 0, &panda.HTTPError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	return io.Copy(dest, resp.Body)
}

// Read every entry to verify CRCs without extracting or trusting ZIP paths.
func validateZIP(ctx context.Context, path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return errInvalidZIP
	}
	defer r.Close()
	files := 0
	for _, file := range r.File {
		if file.FileInfo().IsDir() {
			continue
		}
		files++
		entry, err := file.Open()
		if err != nil {
			return errInvalidZIP
		}
		_, err = io.Copy(io.Discard, contextReader{ctx: ctx, reader: entry})
		closeErr := entry.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || closeErr != nil {
			return errInvalidZIP
		}
	}
	if files == 0 {
		return errInvalidZIP
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
