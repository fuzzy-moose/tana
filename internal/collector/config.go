package collector

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/fuzzy-moose/tana/internal/appdata"
	"github.com/fuzzy-moose/tana/internal/collector/feed"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

type Config struct {
	Server             server.Config
	DataDir            string
	DownloadDir        string
	APIToken           string
	Feed               feed.Config
	Panda              panda.Config
	AuthenticatedPanda panda.AuthenticatedConfig
}

func LoadConfig(getenv func(string) string) (Config, error) {
	var cfg Config
	var err error
	cfg.Server, err = server.LoadConfig(getenv, "TANA_COLLECTOR", "8081")
	if err != nil {
		return Config{}, err
	}
	cfg.APIToken = strings.TrimSpace(getenv("TANA_COLLECTOR_API_TOKEN"))
	if cfg.APIToken == "" || strings.IndexFunc(cfg.APIToken, unicode.IsSpace) >= 0 {
		return Config{}, fmt.Errorf("TANA_COLLECTOR_API_TOKEN must be nonempty and contain no whitespace")
	}
	cfg.Feed, err = feed.LoadConfig(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.Panda, err = panda.LoadConfig(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.AuthenticatedPanda, err = panda.LoadAuthenticatedConfig(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.DataDir, err = appdata.Dir(getenv, getenv("TANA_COLLECTOR_DATA_DIR"), "tana-collector")
	if err != nil {
		return Config{}, fmt.Errorf("resolve collector storage: %w", err)
	}
	cfg.DownloadDir = getenv("TANA_COLLECTOR_DOWNLOAD_DIR")
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = filepath.Join(cfg.DataDir, "downloads")
	}
	cfg.DownloadDir, err = filepath.Abs(cfg.DownloadDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve collector downloads: %w", err)
	}
	return cfg, nil
}
