package metadataproxy

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
	"golang.org/x/time/rate"
)

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
		if response.StatusCode != http.StatusOK {
			return &proxyHTTPError{status: response.StatusCode}
		}
		return nil
	}
	base := &proxyTransport{base: transport, userAgent: ch.UserAgent}
	verified := &verifiedTransport{base: base, verify: s.verifyIP}
	ban := &banState{service: s, id: ch.ID, endpoint: ch.Endpoint}
	client, err := panda.NewClient(s.config.APIURL, &http.Client{
		Timeout:   time.Minute,
		Transport: panda.RateLimitedTransport(limiter, panda.BanTransport(ban, verified)),
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
