package metadataproxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"syscall"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func TestFailureClassifiesWrappedConnectionErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"DNS", &net.DNSError{Err: "private-password", Name: "private-host"}, "proxy_dns_failed"},
		{"refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, "proxy_connection_refused"},
		{"reset", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, "proxy_connection_closed"},
		{"EOF", io.ErrUnexpectedEOF, "proxy_connection_closed"},
		{"timeout", context.DeadlineExceeded, "proxy_timeout"},
		{"certificate", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, "proxy_tls_failed"},
		{"TLS protocol", tls.RecordHeaderError{Msg: "private-password"}, "proxy_protocol_error"},
		{"HTTP on TLS port", http.ErrSchemeMismatch, "proxy_protocol_error"},
		{"SOCKS refusal", &net.OpError{Op: "socks connect", Err: errors.New("unknown error connection refused")}, "proxy_connection_refused"},
		{"SOCKS rejection", &net.OpError{Op: "socks connect", Err: errors.New("unknown error connection not allowed by ruleset")}, "proxy_connection_rejected"},
		{"SOCKS handshake", &net.OpError{Op: "socks connect", Err: errors.New("unexpected protocol version private-password")}, "proxy_socks_failed"},
		{"unknown", errors.New("private-password"), "metadata_proxy_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, auth := failure(&url.Error{Op: "Post", URL: "https://user:private-password@private-host/api", Err: tc.err})
			if code != tc.code || auth {
				t.Fatalf("failure = %q, auth=%v; want %q", code, auth, tc.code)
			}
		})
	}
}

func TestProxyBatchPersistsSpecificFailureWithoutLosingWork(t *testing.T) {
	for _, tc := range []struct {
		name, scheme, body, code string
		status                   int
		storageFailure           bool
	}{
		{name: "CONNECT rejection", scheme: "https", status: 403, body: "private-password", code: "proxy_http_403"},
		{name: "invalid response", body: "<html>private-password</html>", code: "metadata_invalid_response"},
		{name: "API error", body: `{"error":"private-password"}`, code: "metadata_api_error"},
		{name: "missing entries", body: `{"gmetadata":[]}`, code: "metadata_invalid_response"},
		{name: "unrelated entries", body: `{"gmetadata":[{"gid":99,"token":"private-password"}]}`, code: "metadata_invalid_response"},
		{name: "storage error", storageFailure: true, code: "collector_storage_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.storageFailure {
					respond(t, w, r)
					return
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer proxy.Close()
			scheme := tc.scheme
			if scheme == "" {
				scheme = "http"
			}
			s := testService(t, t.TempDir(), scheme+"://panda.invalid/api")
			defer s.Close()
			seed(t, s.db, 1)
			if tc.storageFailure {
				if _, err := s.db.Exec(`CREATE TRIGGER reject_metadata BEFORE INSERT ON gallery_metadata BEGIN SELECT RAISE(ABORT, 'private-password'); END`); err != nil {
					t.Fatal(err)
				}
			}
			addChannel(t, s, "A", proxy.URL)
			if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
				t.Fatal(err)
			}
			result := awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool { return result.Channels[0].LastError != "" })
			ch := result.Channels[0]
			if ch.LastError != tc.code || ch.State != "waiting_retry" || ch.RetryAt == nil {
				t.Fatalf("failure status: %+v; want %s", ch, tc.code)
			}
			var pending int
			if err := s.db.QueryRow(`SELECT count(*) FROM gallery_refs WHERE metadata_attempted_at IS NULL`).Scan(&pending); err != nil || pending != 1 {
				t.Fatalf("pending work = %d, %v", pending, err)
			}
		})
	}
}
