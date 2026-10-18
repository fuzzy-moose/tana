package feed

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	URL        string
	Interval   time.Duration
	RetryDelay time.Duration
}

func LoadConfig(getenv func(string) string) (Config, error) {
	value := func(key, fallback string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return fallback
	}
	feedURL := value("PANDA_FEED_URL", "")
	u, err := url.Parse(feedURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Fragment != "" {
		return Config{}, fmt.Errorf("PANDA_FEED_URL must be an absolute HTTP(S) URL without a fragment")
	}
	interval, err := time.ParseDuration(value("PANDA_FEED_INTERVAL", "30m"))
	if err != nil || interval <= 0 {
		return Config{}, fmt.Errorf("PANDA_FEED_INTERVAL must be a duration greater than zero")
	}
	retry, err := time.ParseDuration(value("PANDA_FEED_RETRY_DELAY", "1m"))
	if err != nil || retry <= 0 {
		return Config{}, fmt.Errorf("PANDA_FEED_RETRY_DELAY must be a duration greater than zero")
	}
	return Config{URL: feedURL, Interval: interval, RetryDelay: retry}, nil
}
