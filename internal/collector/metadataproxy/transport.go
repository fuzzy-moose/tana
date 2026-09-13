package metadataproxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
	"golang.org/x/time/rate"
)

var errProxyAuth = errors.New("proxy_authentication_required")

func (s *Service) client(ch channel, limiter *rate.Limiter) (*panda.Client, *http.Transport, error) {
	proxy, err := url.Parse(ch.Endpoint)
	if err != nil {
		return nil, nil, err
	}
	if ch.Username != "" || ch.Password != "" {
		proxy.User = url.UserPassword(ch.Username, ch.Password)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxy)
	transport.ProxyConnectHeader = http.Header{"User-Agent": {ch.UserAgent}}
	transport.OnProxyConnectResponse = func(_ context.Context, _ *url.URL, _ *http.Request, response *http.Response) error {
		if response.StatusCode == http.StatusProxyAuthRequired {
			return errProxyAuth
		}
		return nil
	}
	base := &proxyTransport{base: transport, userAgent: ch.UserAgent}
	ban := &banState{service: s, id: ch.ID, endpoint: ch.Endpoint}
	client, err := panda.NewClient(s.config.APIURL, &http.Client{
		Timeout:   time.Minute,
		Transport: panda.RateLimitedTransport(limiter, panda.BanTransport(ban, base)),
	})
	return client, transport, err
}

type proxyTransport struct {
	base      http.RoundTripper
	userAgent string
}

func (t *proxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	request := req.Clone(req.Context())
	request.Header.Set("User-Agent", t.userAgent)
	response, err := t.base.RoundTrip(request)
	if err == nil && response.StatusCode == http.StatusProxyAuthRequired {
		response.Body.Close()
		return nil, errProxyAuth
	}
	return response, err
}

// Only stable error codes leave the service. Transport errors may contain a
// proxy URL or credentials, so their raw text is never stored or logged.
func failure(err error) (string, bool) {
	if errors.Is(err, errProxyAuth) {
		return errProxyAuth.Error(), true
	}
	if op, ok := errors.AsType[*net.OpError](err); ok && strings.Contains(op.Op, "socks") {
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
	if upstream, ok := errors.AsType[*panda.HTTPError](err); ok {
		return fmt.Sprintf("upstream_http_%d", upstream.StatusCode), false
	}
	if timeout, ok := errors.AsType[net.Error](err); ok && timeout.Timeout() {
		return "proxy_timeout", false
	}
	return "metadata_proxy_failed", false
}
