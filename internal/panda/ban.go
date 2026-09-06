package panda

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type BanError struct{ Until time.Time }

func (e *BanError) Error() string {
	return "panda: temporarily banned until " + e.Until.UTC().Format(time.RFC3339)
}

var htmlTag = regexp.MustCompile(`(?i)<\s*(?:[a-z][a-z0-9]*\b|!doctype\b|!--|/)`)
var banDuration = regexp.MustCompile(`(?i)\b(\d+)\s*(days?|hours?|minutes?|seconds?)\b`)

// DetectBan examines text, not Content-Type: Panda may serve a text ban as HTML
// or with a successful HTTP status. Never interpret gallery HTML as ban text.
func DetectBan(body []byte, at time.Time) *BanError {
	text := strings.ToLower(string(body))
	if htmlTag.Match(body) || !strings.Contains(text, "ban") ||
		!(strings.Contains(text, "ip") || strings.Contains(text, "temporar")) ||
		!(strings.Contains(text, "excessive") || strings.Contains(text, "pageload") || strings.Contains(text, "expires")) {
		return nil
	}
	delay := time.Duration(0)
	for _, match := range banDuration.FindAllStringSubmatch(text, -1) {
		n, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || n > 365*24*60*60 {
			delay = 0
			break
		}
		unit := time.Second
		switch match[2][0] {
		case 'd':
			unit = 24 * time.Hour
		case 'h':
			unit = time.Hour
		case 'm':
			unit = time.Minute
		}
		if n > int64((365*24*time.Hour)/unit) {
			delay = 0
			break
		}
		delay += time.Duration(n) * unit
		if delay > 365*24*time.Hour {
			delay = 0
			break
		}
	}
	if delay <= 0 {
		delay = 24 * time.Hour
	} else {
		delay += 5 * time.Second
	}
	return &BanError{Until: at.Add(delay)}
}

type BanState interface {
	Until(context.Context) (time.Time, error)
	Extend(context.Context, time.Time) error
}

// BanTransport belongs inside the rate limiter so a ban discovered while
// waiting for a token is checked immediately before sending the request.
func BanTransport(state BanState, transport http.RoundTripper) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &banTransport{state: state, transport: transport}
}

type banTransport struct {
	state     BanState
	transport http.RoundTripper
}

func (t *banTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	until, err := t.state.Until(req.Context())
	if err == nil && until.After(time.Now()) {
		err = &BanError{Until: until}
	}
	if err != nil {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, err
	}
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	// Ban messages are short. Preserve this prefix for the normal response parser.
	prefix, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	if ban := DetectBan(prefix, time.Now()); ban != nil {
		resp.Body.Close()
		// A canceled request must still record an observed ban before shutdown.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), 5*time.Second)
		defer cancel()
		if err := t.state.Extend(ctx, ban.Until); err != nil {
			return nil, errors.Join(ban, fmt.Errorf("persist Panda ban: %w", err))
		}
		return nil, ban
	}
	resp.Body = &prefixedBody{Reader: io.MultiReader(bytes.NewReader(prefix), resp.Body), Closer: resp.Body}
	return resp, nil
}

type prefixedBody struct {
	io.Reader
	io.Closer
}

// RetryDelay is shared by collector workers, not their independently paced requests.
func RetryDelay(failures int64, cause error, at time.Time) time.Duration {
	if ban, ok := errors.AsType[*BanError](cause); ok {
		return max(0, ban.Until.Sub(at))
	}
	delay := min(time.Minute<<min(max(failures, 0), 6), time.Hour)
	if httpErr, ok := errors.AsType[*HTTPError](cause); ok {
		value := strings.TrimSpace(httpErr.RetryAfter)
		if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
			delay = max(delay, time.Duration(seconds)*time.Second)
		} else if deadline, err := http.ParseTime(value); err == nil {
			delay = max(delay, deadline.Sub(at))
		}
	}
	return delay
}
