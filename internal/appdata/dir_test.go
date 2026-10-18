package appdata

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDataDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	for _, tc := range []struct {
		name, goos string
		env        map[string]string
		want       string
	}{
		{"linux", "linux", nil, filepath.Join(home, ".local", "share", "tana")},
		{"xdg", "linux", map[string]string{"XDG_DATA_HOME": home}, filepath.Join(home, "tana")},
		{"relative xdg ignored", "linux", map[string]string{"XDG_DATA_HOME": "relative"}, filepath.Join(home, ".local", "share", "tana")},
		{"mac", "darwin", nil, filepath.Join(home, "Library", "Application Support", "tana")},
		{"windows", "windows", map[string]string{"LOCALAPPDATA": home}, filepath.Join(home, "tana")},
		{"windows fallback", "windows", nil, filepath.Join(home, "AppData", "Local", "tana")},
		{"override", "linux", map[string]string{"TANA_DATA_DIR": home}, home},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dir(func(k string) string { return tc.env[k] }, func() (string, error) { return home, nil }, tc.goos, tc.env["TANA_DATA_DIR"], "tana")
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	_, err := dir(func(string) string { return "" }, func() (string, error) { return "", errors.New("no home") }, "linux", "", "tana")
	if err == nil {
		t.Fatal("missing home accepted")
	}
}

func TestExecutablesUseSeparateDataDirectories(t *testing.T) {
	base := t.TempDir()
	getenv := func(key string) string {
		if key == "XDG_DATA_HOME" {
			return base
		}
		return ""
	}
	for _, name := range []string{"tana", "tana-collector"} {
		got, err := dir(getenv, nil, "linux", "", name)
		if err != nil || got != filepath.Join(base, name) {
			t.Fatalf("%s directory: %q, %v", name, got, err)
		}
	}
}
