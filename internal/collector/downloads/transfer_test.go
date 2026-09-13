package downloads

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestArchiveTransferOmitsCookiesAndChecksResponse(t *testing.T) {
	for _, status := range []int{200, 403, 404, 410, 500} {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse("https://node.hath.network/archive?start=1")
		jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "secret"}})
		client := NewHTTPTransfer(&http.Client{Jar: jar, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
				t.Error("credentials reached archive host")
			}
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("archive"))}, nil
		})})
		var dest bytes.Buffer
		_, err = client.Copy(t.Context(), u.String(), &dest, func(int64) error { return nil })
		if status == 200 {
			if err != nil || dest.String() != "archive" {
				t.Fatalf("transfer: %s, %v", &dest, err)
			}
		} else if err == nil || dest.Len() != 0 {
			t.Fatalf("HTTP %d body saved: %s, %v", status, &dest, err)
		}
		if (status == 403 || status == 404 || status == 410) && !errors.Is(err, errExpiredURL) {
			t.Fatalf("URL not invalidated: %v", err)
		}
	}
}

func TestArchiveTransferRejectsExternalRedirect(t *testing.T) {
	calls := 0
	client := NewHTTPTransfer(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://other.test/archive"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if _, err := client.Copy(t.Context(), "https://node.hath.network/archive", io.Discard, func(int64) error { return nil }); err == nil || calls != 1 {
		t.Fatalf("redirect followed: %d, %v", calls, err)
	}
}

func TestArchiveTransferChecksContentLengthBeforeCopying(t *testing.T) {
	client := NewHTTPTransfer(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Error("content length might describe compressed data")
		}
		return &http.Response{StatusCode: 200, ContentLength: 7, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader("archive"))}, nil
	})})
	var dest bytes.Buffer
	_, err := client.Copy(t.Context(), "https://node.hath.network/archive", &dest, func(size int64) error {
		if size != 7 || dest.Len() != 0 {
			t.Fatalf("admission received size %d after writing %d bytes", size, dest.Len())
		}
		return errStoragePaused
	})
	if !errors.Is(err, errStoragePaused) || dest.Len() != 0 {
		t.Fatalf("rejected archive body was copied: %d bytes, %v", dest.Len(), err)
	}
}
