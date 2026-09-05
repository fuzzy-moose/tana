package server

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	for _, prefix := range []string{"TANA", "TANA_COLLECTOR"} {
		t.Run(prefix, func(t *testing.T) {
			cfg, err := LoadConfig(func(string) string { return "" }, prefix, "8080")
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Addr != "127.0.0.1:8080" || cfg.GracePeriod != 5*time.Second || cfg.LogLevel != slog.LevelInfo {
				t.Fatalf("unexpected defaults: %+v", cfg)
			}
			env := map[string]string{
				prefix + "_HOST": "::1", prefix + "_PORT": "9000",
				prefix + "_GRACE_PERIOD": "10s", prefix + "_LOG_LEVEL": "DEBUG",
			}
			cfg, err = LoadConfig(func(key string) string { return env[key] }, prefix, "8080")
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Addr != "[::1]:9000" || cfg.GracePeriod != 10*time.Second || cfg.LogLevel != slog.LevelDebug {
				t.Fatalf("unexpected overrides: %+v", cfg)
			}
		})
	}
}

func TestLoadConfigRejectsInvalidSettings(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"PORT", "abc"}, {"PORT", "0"}, {"PORT", "65536"},
		{"GRACE_PERIOD", "never"}, {"GRACE_PERIOD", "0s"}, {"GRACE_PERIOD", "-1s"}, {"GRACE_PERIOD", "61s"},
		{"LOG_LEVEL", "verbose"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			key := "TANA_" + tc.key
			_, err := LoadConfig(func(k string) string {
				if k == key {
					return tc.value
				}
				return ""
			}, "TANA", "8080")
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("expected error identifying %s, got %v", key, err)
			}
		})
	}
}
