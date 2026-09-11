// Package appdata resolves executable-owned data directories.
package appdata

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Dir uses an explicit override or the platform's data directory for name.
func Dir(getenv func(string) string, override, name string) (string, error) {
	return dir(getenv, os.UserHomeDir, runtime.GOOS, override, name)
}

func dir(getenv func(string) string, homeDir func() (string, error), goos, override, name string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	var base string
	switch goos {
	case "windows":
		base = getenv("LOCALAPPDATA")
	case "darwin":
		// macOS uses ~/Library/Application Support.
	default:
		base = getenv("XDG_DATA_HOME")
		if base != "" && !filepath.IsAbs(base) {
			base = "" // The XDG specification ignores relative paths.
		}
	}
	if base == "" {
		home, err := homeDir()
		if err != nil {
			return "", fmt.Errorf("resolve application data directory: %w", err)
		}
		switch goos {
		case "windows":
			base = filepath.Join(home, "AppData", "Local")
		case "darwin":
			base = filepath.Join(home, "Library", "Application Support")
		default:
			base = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Abs(filepath.Join(base, name))
}
