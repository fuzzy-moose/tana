package local

import (
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TANA_DATA_DIR": dir}
	getenv := func(key string) string { return env[key] }
	cfg, err := LoadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != "127.0.0.1:8080" || cfg.DataDir != dir || cfg.WebDir != "" || cfg.CollectorURL != "" {
		t.Fatal("Tana configuration did not apply defaults")
	}
	env["TANA_PORT"] = "9000"
	env["TANA_WEB_DIR"] = " ./web "
	env["TANA_COLLECTOR_URL"] = " https://collector.test "
	env["TANA_COLLECTOR_API_TOKEN"] = " test-token "
	cfg, err = LoadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != "127.0.0.1:9000" || cfg.WebDir != "./web" || cfg.CollectorURL != "https://collector.test" || cfg.CollectorAPIToken != "test-token" {
		t.Fatal("Tana configuration ignored overrides")
	}
}
