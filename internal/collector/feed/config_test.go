package feed

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	env := map[string]string{"PANDA_FEED_URL": testFeedURL}
	getenv := func(key string) string { return env[key] }
	cfg, err := LoadConfig(getenv)
	if err != nil || cfg != (Config{URL: testFeedURL, Interval: 30 * time.Minute, RetryDelay: time.Minute}) {
		t.Fatalf("defaults: %+v, %v", cfg, err)
	}
	env["PANDA_FEED_INTERVAL"] = " 15m "
	env["PANDA_FEED_RETRY_DELAY"] = "10s"
	cfg, err = LoadConfig(getenv)
	if err != nil || cfg.Interval != 15*time.Minute || cfg.RetryDelay != 10*time.Second {
		t.Fatalf("overrides: %+v, %v", cfg, err)
	}
	for _, tc := range []struct{ key, value string }{
		{"PANDA_FEED_URL", ""}, {"PANDA_FEED_URL", "/relative"},
		{"PANDA_FEED_URL", "file:///tmp/feed"}, {"PANDA_FEED_URL", "https://example.test/#fragment"},
		{"PANDA_FEED_INTERVAL", "0"}, {"PANDA_FEED_INTERVAL", "-1m"}, {"PANDA_FEED_INTERVAL", "soon"},
		{"PANDA_FEED_RETRY_DELAY", "0s"}, {"PANDA_FEED_RETRY_DELAY", "-1s"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			_, err := LoadConfig(func(key string) string {
				if key == tc.key {
					return tc.value
				}
				if key == "PANDA_FEED_URL" {
					return testFeedURL
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("expected %s validation error, got %v", tc.key, err)
			}
		})
	}
}
