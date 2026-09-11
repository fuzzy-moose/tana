package collector

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{
		"TANA_COLLECTOR_DATA_DIR":    dir,
		"TANA_COLLECTOR_API_TOKEN":   "test-token",
		"TANA_COLLECTOR_SITEMAP_URL": "https://panda.test/custom/index.xml?version=2",
		"PANDA_FEED_URL":             "https://panda.test/feed",
		"PANDA_API_URL":              "https://panda.test/api",
		"PANDA_ARCHIVER_URL":         "https://panda.test/account/prepare-archive",
		"PANDA_FAVORITES_URL":        "https://panda.test/favorites",
		"PANDA_FAVORITES_COOKIES":    `{"ipb_member_id":"1"}`,
	}
	getenv := func(key string) string { return env[key] }
	cfg, err := LoadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != "127.0.0.1:8081" || cfg.DataDir != dir || cfg.DownloadDir != filepath.Join(dir, "downloads") ||
		cfg.APIToken != "test-token" || cfg.Feed.URL != env["PANDA_FEED_URL"] || cfg.Panda.APIURL != env["PANDA_API_URL"] ||
		cfg.Sitemap.URL != env["TANA_COLLECTOR_SITEMAP_URL"] ||
		cfg.AuthenticatedPanda.FavoritesURL != env["PANDA_FAVORITES_URL"] || cfg.AuthenticatedPanda.ArchiverURL != env["PANDA_ARCHIVER_URL"] {
		t.Fatal("collector configuration did not retain settings or apply defaults")
	}
	env["TANA_COLLECTOR_DOWNLOAD_DIR"] = filepath.Join(t.TempDir(), "archives")
	env["TANA_COLLECTOR_PORT"] = "9001"
	cfg, err = LoadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DownloadDir != env["TANA_COLLECTOR_DOWNLOAD_DIR"] || cfg.Server.Addr != "127.0.0.1:9001" {
		t.Fatal("collector configuration ignored overrides")
	}
	for _, key := range []string{"TANA_COLLECTOR_API_TOKEN", "PANDA_FEED_URL"} {
		t.Run(key, func(t *testing.T) {
			_, err := LoadConfig(func(k string) string {
				if k == key {
					return ""
				}
				return env[k]
			})
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("expected error identifying %s, got %v", key, err)
			}
		})
	}
}
