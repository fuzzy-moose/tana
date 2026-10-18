// Package server provides the HTTP infrastructure shared by Tana's services.
package server

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr        string
	GracePeriod time.Duration
	LogLevel    slog.Level
}

// LoadConfig reads service-prefixed environment variables, such as TANA_PORT.
func LoadConfig(getenv func(string) string, prefix, defaultPort string) (Config, error) {
	value := func(key, fallback string) string {
		if v := strings.TrimSpace(getenv(prefix + "_" + key)); v != "" {
			return v
		}
		return fallback
	}
	port := value("PORT", defaultPort)
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, fmt.Errorf("%s_PORT must be between 1 and 65535", prefix)
	}
	grace, err := time.ParseDuration(value("GRACE_PERIOD", "5s"))
	if err != nil || grace <= 0 || grace > time.Minute {
		return Config{}, fmt.Errorf("%s_GRACE_PERIOD must be greater than zero and at most 1m", prefix)
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(value("LOG_LEVEL", "INFO"))); err != nil {
		return Config{}, fmt.Errorf("%s_LOG_LEVEL: %w", prefix, err)
	}
	return Config{
		Addr:        net.JoinHostPort(value("HOST", "127.0.0.1"), port),
		GracePeriod: grace,
		LogLevel:    level,
	}, nil
}
