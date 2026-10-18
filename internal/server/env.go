package server

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/joho/godotenv"
)

// LoadEnvironment loads an optional .env before executable configuration in
// development mode. Existing process environment variables take precedence.
func LoadEnvironment(development bool) error {
	if !development {
		return nil
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	return nil
}
