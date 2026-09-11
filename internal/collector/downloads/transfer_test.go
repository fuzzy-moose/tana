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
		_, err = client.Copy(t.Context(), u.String(), &dest)
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
	if _, err := client.Copy(t.Context(), "https://node.hath.network/archive", io.Discard); err == nil || calls != 1 {
		t.Fatalf("redirect followed: %d, %v", calls, err)
	}
}
