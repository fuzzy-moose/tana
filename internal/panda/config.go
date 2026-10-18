package panda

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	APIURL       string
	RateInterval time.Duration
	RateBurst    int
}

// LoadConfig reads Panda settings from the application's environment getter.
// APIURL has no default; configuration does not enable rate limiting by itself.
func LoadConfig(getenv func(string) string) (Config, error) {
	value := func(key, fallback string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return fallback
	}
	apiURL := value("PANDA_API_URL", "")
	if err := validateAPIURL(apiURL); err != nil {
		return Config{}, fmt.Errorf("PANDA_API_URL: %w", err)
	}
	interval, err := time.ParseDuration(value("PANDA_RATE_INTERVAL", "2500ms"))
	if err != nil || interval <= 0 {
		return Config{}, fmt.Errorf("PANDA_RATE_INTERVAL must be a duration greater than zero")
	}
	burst, err := strconv.Atoi(value("PANDA_RATE_BURST", "4"))
	if err != nil || burst <= 0 {
		return Config{}, fmt.Errorf("PANDA_RATE_BURST must be an integer greater than zero")
	}
	return Config{APIURL: apiURL, RateInterval: interval, RateBurst: burst}, nil
}

func validateAPIURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Fragment != "" {
		return fmt.Errorf("must be an absolute HTTP(S) URL without a fragment")
	}
	return nil
}
