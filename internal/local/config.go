package local

import (
	"fmt"
	"strings"

	"github.com/fuzzy-moose/tana/internal/appdata"
	"github.com/fuzzy-moose/tana/internal/server"
)

type Config struct {
	Server            server.Config
	DataDir           string
	WebDir            string
	CollectorURL      string
	CollectorAPIToken string
}

func LoadConfig(getenv func(string) string) (Config, error) {
	var cfg Config
	var err error
	cfg.Server, err = server.LoadConfig(getenv, "TANA", "8080")
	if err != nil {
		return Config{}, err
	}
	cfg.DataDir, err = appdata.Dir(getenv, getenv("TANA_DATA_DIR"), "tana")
	if err != nil {
		return Config{}, fmt.Errorf("resolve local storage: %w", err)
	}
	cfg.WebDir = strings.TrimSpace(getenv("TANA_WEB_DIR"))
	cfg.CollectorURL = strings.TrimSpace(getenv("TANA_COLLECTOR_URL"))
	cfg.CollectorAPIToken = strings.TrimSpace(getenv("TANA_COLLECTOR_API_TOKEN"))
	return cfg, nil
}
