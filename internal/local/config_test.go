package local

import (
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TANA_DATA_DIR": dir, "TANA_DATABASE_URL": " postgres://tana@db.test/tana "}
	getenv := func(key string) string { return env[key] }
	cfg, err := LoadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != "127.0.0.1:8080" || cfg.DataDir != dir || cfg.WebDir != "" || cfg.CollectorURL != "" ||
		cfg.DatabaseURL != "postgres://tana@db.test/tana" || cfg.DatabasePoolSize != 4 {
		t.Fatal("Tana configuration did not apply defaults")
	}
	env["TANA_PORT"] = "9000"
	env["TANA_WEB_DIR"] = " ./web "
	env["TANA_COLLECTOR_URL"] = " https://collector.test "
	env["TANA_COLLECTOR_API_TOKEN"] = " test-token "
	env["TANA_DATABASE_POOL_SIZE"] = "2"
	cfg, err = LoadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != "127.0.0.1:9000" || cfg.WebDir != "./web" || cfg.CollectorURL != "https://collector.test" || cfg.CollectorAPIToken != "test-token" ||
		cfg.DatabasePoolSize != 2 {
		t.Fatal("Tana configuration ignored overrides")
	}
	for key, value := range map[string]string{"TANA_DATABASE_URL": " ", "TANA_DATABASE_POOL_SIZE": "0"} {
		previous := env[key]
		env[key] = value
		if _, err := LoadConfig(getenv); err == nil {
			t.Errorf("%s=%q accepted", key, value)
		}
		env[key] = previous
	}
}
