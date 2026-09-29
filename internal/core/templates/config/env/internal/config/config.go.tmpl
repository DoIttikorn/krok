// Package config loads settings into the process environment before the
// rest of the app reads them with os.Getenv.
package config

import (
	"context"
	"errors"
	"io/fs"

	"github.com/joho/godotenv"
)

// Load reads .env into the environment if the file exists. Variables that
// are already set in the real environment are not overwritten.
func Load(ctx context.Context) error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
