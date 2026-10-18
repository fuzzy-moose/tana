package panda

import (
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// NewRateLimiter creates an application-owned limiter with one token replenished
// per interval. Retain it across page loads; pass the settings from LoadConfig.
func NewRateLimiter(interval time.Duration, burst int) (*rate.Limiter, error) {
	if interval <= 0 || burst <= 0 {
		return nil, fmt.Errorf("panda: rate interval and burst must be greater than zero")
	}
	return rate.NewLimiter(rate.Every(interval), burst), nil
}

// RateLimitedTransport waits for one token before each HTTP request. Pass it
// through the HTTP client supplied to NewClient to opt into rate limiting.
// A nil transport uses http.DefaultTransport.
func RateLimitedTransport(limiter *rate.Limiter, transport http.RoundTripper) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &rateLimitedTransport{limiter: limiter, transport: transport}
}

type rateLimitedTransport struct {
	limiter   *rate.Limiter
	transport http.RoundTripper
}

func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limiter.Wait(req.Context()); err != nil {
		// RoundTripper owns the request body even when no request is sent.
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, err
	}
	return t.transport.RoundTrip(req)
}
