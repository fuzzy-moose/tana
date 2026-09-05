package panda_test

import (
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestLoadConfig(t *testing.T) {
	for _, tc := range []struct {
		name     string
		env      map[string]string
		interval time.Duration
		burst    int
	}{
		{"rate defaults", map[string]string{"PANDA_API_URL": testAPIURL}, 2500 * time.Millisecond, 4},
		{"overrides", map[string]string{
			"PANDA_API_URL": " " + testAPIURL + " ", "PANDA_RATE_INTERVAL": " 5s ", "PANDA_RATE_BURST": " 2 ",
		}, 5 * time.Second, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := panda.LoadConfig(func(key string) string { return tc.env[key] })
			if err != nil {
				t.Fatal(err)
			}
			if cfg.APIURL != testAPIURL || cfg.RateInterval != tc.interval || cfg.RateBurst != tc.burst {
				t.Fatalf("unexpected configuration: %+v", cfg)
			}
			limiter, err := panda.NewRateLimiter(cfg.RateInterval, cfg.RateBurst)
			if err != nil {
				t.Fatal(err)
			}
			if float64(limiter.Limit()) != 1/tc.interval.Seconds() || limiter.Burst() != tc.burst {
				t.Fatalf("limiter does not reflect configuration: rate %v, burst %d", limiter.Limit(), limiter.Burst())
			}
		})
	}
}

func TestLoadConfigRejectsInvalidSettings(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"PANDA_API_URL", ""},
		{"PANDA_API_URL", "   "},
		{"PANDA_API_URL", "/api.php"},
		{"PANDA_RATE_INTERVAL", "0s"},
		{"PANDA_RATE_INTERVAL", "-1s"},
		{"PANDA_RATE_INTERVAL", "invalid"},
		{"PANDA_RATE_BURST", "0"},
		{"PANDA_RATE_BURST", "-1"},
		{"PANDA_RATE_BURST", "1.5"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			env := map[string]string{"PANDA_API_URL": testAPIURL, tc.key: tc.value}
			_, err := panda.LoadConfig(func(key string) string { return env[key] })
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("expected error identifying %s, got %v", tc.key, err)
			}
		})
	}
}

func TestNewClientRequiresAPIURL(t *testing.T) {
	for _, rawURL := range []string{"", "/api.php", "https://", "ftp://example.com/api", "https://example.com/%zz", "https://example.com/#api"} {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := panda.NewClient(rawURL, nil); err == nil {
				t.Fatalf("accepted invalid API URL %q", rawURL)
			}
		})
	}
	for _, rawURL := range []string{testAPIURL, "http://127.0.0.1:8089/api.php"} {
		if _, err := panda.NewClient(rawURL, nil); err != nil {
			t.Fatalf("rejected API URL %q: %v", rawURL, err)
		}
	}
}

func TestNewRateLimiterRejectsInvalidSettings(t *testing.T) {
	for _, tc := range []struct {
		interval time.Duration
		burst    int
	}{{0, 4}, {-time.Second, 4}, {time.Second, 0}, {time.Second, -1}} {
		if _, err := panda.NewRateLimiter(tc.interval, tc.burst); err == nil {
			t.Fatalf("accepted invalid rate settings: %+v", tc)
		}
	}
}
