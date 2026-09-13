package metadataproxy

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func ipReport(ip, headers string) string {
	return `<div id="ip">` + html.EscapeString(ip) + `</div><pre id="http-request">` +
		html.EscapeString("GET / HTTP/1.1\r\nHost: www.ip.wtf\r\n"+headers+"\r\n") + `</pre>`
}

func checkResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestIPVerificationChecksExitAndExposedHeaders(t *testing.T) {
	for _, tc := range []struct {
		name, ip, headers, body string
		want                    error
	}{
		{name: "different exit", ip: "203.0.113.2"},
		{name: "same exit", ip: "198.51.100.1", want: errProxyIPLeak},
		{name: "forwarded original", ip: "203.0.113.2", headers: "X-Forwarded-For: 198.51.100.1, 203.0.113.2", want: errProxyIPLeak},
		{name: "IP with port", ip: "203.0.113.2", headers: "X-Real-IP: 198.51.100.1:4321", want: errProxyIPLeak},
		{name: "IPv6 header on IPv4 exit", ip: "203.0.113.2", headers: `Forwarded: for="[2001:0db8:0:0:0:0:0:1]:1234"`, want: errProxyIPLeak},
		{name: "IPv6 exit", ip: "2001:db8::2"},
		{name: "same IPv6 exit", ip: "2001:0db8:0:0:0:0:0:1", want: errProxyIPLeak},
		{name: "mapped IPv4", ip: "::ffff:198.51.100.1", want: errProxyIPLeak},
		{name: "missing IP", body: `<pre id="http-request">GET / HTTP/1.1</pre>`, want: errProxyIPCheck},
		{name: "missing request", body: `<div id="ip">203.0.113.2</div>`, want: errProxyIPCheck},
		{name: "invalid IP", ip: "not an IP", want: errProxyIPCheck},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directCalls := make(map[string]int)
			v := newIPVerifier()
			if v.direct.(*http.Transport).Proxy != nil {
				t.Fatal("direct IP lookup uses an environment proxy")
			}
			v.direct = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				directCalls[r.URL.Host]++
				if r.URL.Scheme != "https" || r.Header.Get("Accept") != "text/plain" {
					t.Errorf("unexpected direct request: %s, %v", r.URL, r.Header)
				}
				switch r.URL.Host {
				case "v4.ip.wtf":
					return checkResponse("198.51.100.1\n"), nil
				case "v6.ip.wtf":
					return checkResponse("2001:db8::1\n"), nil
				default:
					t.Fatalf("unexpected direct host: %s", r.URL.Host)
					return nil, nil
				}
			})
			proxy := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://www.ip.wtf/" || r.Header.Get("Accept") != "text/html" || r.Method != http.MethodGet {
					t.Errorf("unexpected proxy check: %s, %v", r.URL, r.Header)
				}
				body := tc.body
				if body == "" {
					body = ipReport(tc.ip, tc.headers)
				}
				return checkResponse(body), nil
			})
			for range 2 {
				if err := v.verify(t.Context(), proxy); !errors.Is(err, tc.want) {
					t.Fatalf("verification = %v; want %v", err, tc.want)
				}
			}
			for host, calls := range directCalls {
				if calls != 1 {
					t.Errorf("%s called %d times; direct lookups must be shared and cached", host, calls)
				}
			}
		})
	}
}

func TestIPVerificationRejectsFailedChecks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		direct bool
		status int
		body   string
		err    error
	}{
		{name: "proxy timeout", err: context.DeadlineExceeded},
		{name: "proxy auth", err: errProxyAuth},
		{name: "redirect", status: http.StatusFound},
		{name: "server error", status: http.StatusServiceUnavailable},
		{name: "oversize", body: strings.Repeat("x", (256<<10)+1)},
		{name: "direct failure", direct: true, err: context.DeadlineExceeded},
		{name: "invalid baseline", direct: true, body: "invalid"},
		{name: "wrong address family", direct: true, body: "2001:db8::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failed := roundTripFunc(func(*http.Request) (*http.Response, error) {
				if tc.err != nil {
					return nil, tc.err
				}
				response := checkResponse(tc.body)
				if tc.status != 0 {
					response.StatusCode = tc.status
				}
				return response, nil
			})
			v := newIPVerifier()
			proxy := failed
			v.direct = roundTripFunc(func(*http.Request) (*http.Response, error) { return checkResponse("198.51.100.1"), nil })
			if tc.direct {
				v.direct = failed
				proxy = roundTripFunc(func(*http.Request) (*http.Response, error) { return checkResponse(ipReport("203.0.113.2", "")), nil })
			}
			if err := v.verify(t.Context(), proxy); !errors.Is(err, errProxyIPCheck) {
				t.Fatalf("failed check accepted: %v", err)
			}
		})
	}
}

func TestVerifiedTransportChecksBeforeSendingAndRechecksAfterExpiry(t *testing.T) {
	var checks, requests int
	var checkError error
	transport := &verifiedTransport{
		base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests++
			return checkResponse("metadata"), nil
		}),
		verify: func(context.Context, http.RoundTripper) error {
			checks++
			return checkError
		},
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://panda.invalid/api", nil)
	for range 2 {
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	if checks != 1 || requests != 2 {
		t.Fatalf("checks=%d metadata requests=%d", checks, requests)
	}
	transport.until = time.Now().Add(-time.Second)
	checkError = errProxyIPLeak
	if _, err := transport.RoundTrip(req); !errors.Is(err, errProxyIPLeak) || requests != 2 || checks != 2 {
		t.Fatalf("expired verification bypassed: checks=%d requests=%d err=%v", checks, requests, err)
	}
}

func TestFailedVerificationRemovesProxyWithoutRequestingMetadata(t *testing.T) {
	for _, checkError := range []error{errProxyIPLeak, errProxyIPCheck, errors.Join(errProxyIPCheck, errProxyAuth)} {
		t.Run(checkError.Error(), func(t *testing.T) {
			var requests atomic.Int32
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				respond(t, w, r)
			}))
			defer proxy.Close()
			s := testServiceWithIPCheck(t, t.TempDir(), "http://panda.invalid/api", func(context.Context, http.RoundTripper) error { return checkError })
			defer s.Close()
			seed(t, s.db, 1)
			addChannel(t, s, "A", proxy.URL)
			if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
				t.Fatal(err)
			}
			awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool { return len(result.Channels) == 0 })
			if requests.Load() != 0 {
				t.Fatal("metadata requested before verification")
			}
			batch, err := s.metadata.ClaimBackground(t.Context())
			if err != nil || batch == nil || batch.Size() != 1 {
				t.Fatalf("verification failure lost work: %+v, %v", batch, err)
			}
			batch.Close()
		})
	}
}

func TestIPVerificationRepeatsForEditedChannel(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var checks atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { respond(t, w, r) }))
	defer proxy.Close()
	s := testServiceWithIPCheck(t, t.TempDir(), "http://panda.invalid/api", func(ctx context.Context, _ http.RoundTripper) error {
		if checks.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return errProxyIPLeak
		}
		return nil
	})
	defer s.Close()
	seed(t, s.db, 1)
	id := addChannel(t, s, "A", proxy.URL)
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	receive(t, started)
	if err := s.Save(t.Context(), id, collectorapi.MetadataProxyInput{Name: "Edited", ProxyURL: proxy.URL, Username: "updated", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	close(release)
	result := awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool {
		return len(result.Channels) == 1 && result.Channels[0].LastSuccessAt != nil
	})
	if checks.Load() != 2 || result.Channels[0].Name != "Edited" {
		t.Fatalf("edited channel was not reverified: checks=%d status=%+v", checks.Load(), result)
	}
}
