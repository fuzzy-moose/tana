package server

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/joho/godotenv"
)

// LoadEnvironment loads an optional .env only in development mode. Loading at
// startup makes the values available to existing os.Getenv consumers as well;
// environment variables already set by the process take precedence.
func LoadEnvironment(development bool) error {
	if !development {
		return nil
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	return nil
}
