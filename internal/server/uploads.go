package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var ErrUploadInterrupted = errors.New("upload interrupted")

// PrepareUpload bounds the body while allowing file transfers to outlive the
// ordinary API read and write deadlines. Call before reading any upload bytes.
func PrepareUpload(w http.ResponseWriter, r *http.Request, limit int64) error {
	if r.ContentLength > limit {
		return &http.MaxBytesError{Limit: limit}
	}
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if err := controller.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	r.Body = uploadBody{http.MaxBytesReader(w, r.Body, limit)}
	return nil
}

type uploadBody struct{ io.ReadCloser }

func (b uploadBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && err != io.EOF {
		err = fmt.Errorf("%w: %w", ErrUploadInterrupted, err)
	}
	return n, err
}
