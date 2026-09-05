package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestLoadEnvironment(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, key := range []string{"TANA_PORT", "TANA_DATA_DIR", "PANDA_API_URL", "PANDA_RATE_INTERVAL", "PANDA_RATE_BURST"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TANA_PORT", "9000")
	if err := os.WriteFile(".env", []byte("TANA_PORT=8088\nTANA_DATA_DIR=./data\nPANDA_API_URL=https://panda.example.test/api.php\nPANDA_RATE_INTERVAL=5s\nPANDA_RATE_BURST=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnvironment(false); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("PANDA_API_URL") != "" {
		t.Fatal("loaded .env outside development mode")
	}
	if err := LoadEnvironment(true); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("TANA_PORT") != "9000" || os.Getenv("TANA_DATA_DIR") != "./data" {
		t.Fatal("expected process environment precedence and file values available through os.Getenv")
	}
	cfg, err := panda.LoadConfig(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "https://panda.example.test/api.php" || cfg.RateInterval != 5*time.Second || cfg.RateBurst != 2 {
		t.Fatalf("unexpected Panda configuration from .env: %+v", cfg)
	}
}

func TestLoadEnvironmentMissingAndMalformedFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := LoadEnvironment(true); err != nil {
		t.Fatalf("missing .env should be optional: %v", err)
	}
	if err := os.WriteFile(".env", []byte("BROKEN=\"unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnvironment(false); err != nil {
		t.Fatalf("should ignore .env outside development: %v", err)
	}
	if err := LoadEnvironment(true); err == nil || !strings.Contains(err.Error(), ".env") {
		t.Fatalf("expected error identifying malformed .env, got %v", err)
	}
}
