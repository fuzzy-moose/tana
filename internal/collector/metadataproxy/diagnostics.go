package metadataproxy

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"

	"github.com/fuzzy-moose/tana/internal/panda"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var errProxyAuth = errors.New("proxy_authentication_required")

type proxyHTTPError struct{ status int }

func (e *proxyHTTPError) Error() string { return fmt.Sprintf("proxy CONNECT: HTTP %d", e.status) }

// Only stable error codes leave the service. Transport errors may contain a
// proxy URL or credentials, so their raw text is never stored or logged.
func failure(err error) (string, bool) {
	if errors.Is(err, errProxyAuth) {
		return errProxyAuth.Error(), true
	}
	if errors.Is(err, errProxyIPLeak) {
		return errProxyIPLeak.Error(), false
	}
	if errors.Is(err, errProxyIPCheck) {
		return errProxyIPCheck.Error(), false
	}
	op, isOp := errors.AsType[*net.OpError](err)
	socks := isOp && strings.Contains(op.Op, "socks")
	if socks {
		// net/http's SOCKS transport exposes these failures as untyped errors.
		message := op.Err.Error()
		if strings.Contains(message, "username/password authentication failed") ||
			message == "invalid username/password" || strings.Contains(message, "no acceptable authentication methods") || strings.Contains(message, "unsupported authentication method") {
			return errProxyAuth.Error(), true
		}
	}
	if _, ok := errors.AsType[*panda.BanError](err); ok {
		return "panda_banned", false
	}
	if proxy, ok := errors.AsType[*proxyHTTPError](err); ok {
		return fmt.Sprintf("proxy_http_%d", proxy.status), false
	}
	if upstream, ok := errors.AsType[*panda.HTTPError](err); ok {
		return fmt.Sprintf("upstream_http_%d", upstream.StatusCode), false
	}
	if db, ok := errors.AsType[*sqlite.Error](err); ok {
		switch db.Code() & 0xff {
		case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED:
			return "collector_storage_busy", false
		case sqlite3.SQLITE_FULL:
			return "collector_storage_full", false
		case sqlite3.SQLITE_READONLY:
			return "collector_storage_readonly", false
		}
		return "collector_storage_failed", false
	}
	if errors.Is(err, panda.ErrMetadataResponse) {
		return "metadata_invalid_response", false
	}
	if errors.Is(err, panda.ErrMetadataAPI) {
		return "metadata_api_error", false
	}
	if timeout, ok := errors.AsType[net.Error](err); ok && timeout.Timeout() {
		return "proxy_timeout", false
	}
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return "proxy_dns_failed", false
	}
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return "proxy_tls_failed", false
	}
	if _, ok := errors.AsType[tls.RecordHeaderError](err); ok {
		return "proxy_protocol_error", false
	}
	if errors.Is(err, http.ErrSchemeMismatch) {
		return "proxy_protocol_error", false
	}
	if _, ok := errors.AsType[tls.AlertError](err); ok {
		return "proxy_tls_failed", false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "proxy_connection_refused", false
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		return "proxy_unreachable", false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return "proxy_connection_closed", false
	}
	if socks {
		switch op.Err.Error() {
		case "unknown error connection refused":
			return "proxy_connection_refused", false
		case "unknown error connection not allowed by ruleset":
			return "proxy_connection_rejected", false
		case "unknown error network unreachable", "unknown error host unreachable":
			return "proxy_unreachable", false
		}
		return "proxy_socks_failed", false
	}
	if isOp {
		return "proxy_connection_failed", false
	}
	return "metadata_proxy_failed", false
}
