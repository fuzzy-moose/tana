package delivery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

type streamingCollector struct {
	*fakeCollector
	stream func(context.Context) io.Reader
}

func (c streamingCollector) OpenDownload(ctx context.Context, id int64, method string, headers http.Header) (*http.Response, error) {
	if id != 7 {
		return c.fakeCollector.OpenDownload(ctx, id, method, headers)
	}
	c.f.event("transfer:7")
	return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: io.NopCloser(c.stream(ctx))}, nil
}

type archiveReaderFunc func([]byte) (int, error)

func (f archiveReaderFunc) Read(p []byte) (int, error) { return f(p) }

type stalledOpeningCollector struct {
	*fakeCollector
	opening chan struct{}
}

func (c stalledOpeningCollector) OpenDownload(ctx context.Context, id int64, method string, headers http.Header) (*http.Response, error) {
	if id != 7 {
		return c.fakeCollector.OpenDownload(ctx, id, method, headers)
	}
	close(c.opening)
	<-ctx.Done()
	// Error-body decoding can block until cancellation, then return an HTTP error.
	return nil, &collectorapi.HTTPError{StatusCode: http.StatusServiceUnavailable}
}

func TestStalledArchiveTransferReleasesWorker(t *testing.T) {
	for _, phase := range []string{"open", "body"} {
		for _, stop := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stop=%v", phase, stop), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					f := newFixture(t)
					reading := make(chan struct{})
					f.service.collector = streamingCollector{fakeCollector: f.collector, stream: func(ctx context.Context) io.Reader {
						first := true
						return archiveReaderFunc(func(p []byte) (int, error) {
							if first {
								first = false
								return copy(p, "partial archive"), nil
							}
							close(reading)
							<-ctx.Done()
							return 0, ctx.Err()
						})
					}}
					if phase == "open" {
						f.service.collector = stalledOpeningCollector{fakeCollector: f.collector, opening: reading}
					}
					b := f.start(7, 8)
					done := make(chan error, 1)
					go func() {
						_, err := f.service.step()
						done <- err
					}()
					<-reading
					if stop {
						if _, err := f.service.Stop(t.Context(), b.ID); err != nil {
							t.Fatal(err)
						}
					}
					time.Sleep(2 * time.Minute)
					synctest.Wait()
					select {
					case err := <-done:
						if err != nil {
							t.Fatal(err)
						}
					default:
						f.service.cancel()
						<-done
						t.Fatal("stalled archive transfer kept the worker blocked")
					}
					b = f.drain(b.ID)
					if b.Items[0].State != "failed" || !strings.Contains(b.Items[0].Error, "inactive") {
						t.Fatalf("stalled archive: %+v", b.Items[0])
					}
					if stop {
						if b.State != "stopped" || b.Items[1].State != "queued" {
							t.Fatalf("stop did not settle: %+v", b)
						}
					} else if b.State != "completed_with_errors" || b.Items[1].State != "completed" {
						t.Fatalf("next archive did not complete: %+v", b)
					}
					for _, path := range []string{stagingPath(b, b.Items[0]), filepath.Join(f.root, archiveName(7))} {
						if _, err := os.Lstat(path); !os.IsNotExist(err) {
							t.Fatalf("stalled transfer left %s: %v", path, err)
						}
					}
				})
			})
		}
	}
}

func TestArchiveTransferDeadlineTracksInactivity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.service.collector = streamingCollector{fakeCollector: f.collector, stream: func(ctx context.Context) io.Reader {
			time.Sleep(20 * time.Second)
			chunks := 4
			return archiveReaderFunc(func(p []byte) (int, error) {
				if chunks == 0 {
					return 0, io.EOF
				}
				select {
				case <-time.After(20 * time.Second):
					chunks--
					return copy(p, "archive chunk"), nil
				case <-ctx.Done():
					return 0, ctx.Err()
				}
			})
		}}
		b := f.start(7)
		f.drain(b.ID)
		f.assertRemoved(b.ID)
	})
}
