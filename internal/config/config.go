// Package config resolves paths and env vars for wadb.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Home      string
	SessionDB string
	AppDB     string
	MediaDir  string
	LogLevel  string
}

func Load() (*Config, error) {
	home := os.Getenv("WADB_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		home = filepath.Join(userHome, ".wadb")
	}
	if err := ensureDir0700(home); err != nil {
		return nil, err
	}
	mediaDir := filepath.Join(home, "media")
	if err := ensureDir0700(mediaDir); err != nil {
		return nil, err
	}
	level := os.Getenv("WADB_LOG_LEVEL")
	if level == "" {
		level = "info"
	}
	return &Config{
		Home:      home,
		SessionDB: filepath.Join(home, "session.db"),
		AppDB:     filepath.Join(home, "wadb.db"),
		MediaDir:  mediaDir,
		LogLevel:  level,
	}, nil
}

// ensureDir0700 creates dir if it doesn't exist and enforces 0700 permissions.
// MkdirAll alone is insufficient: it's a no-op on pre-existing directories,
// and umask can strip mode bits even on fresh creation.
func ensureDir0700(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("chmod %s: %w", dir, err)
	}
	return nil
}
