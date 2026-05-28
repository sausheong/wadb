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
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", home, err)
	}
	mediaDir := filepath.Join(home, "media")
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", mediaDir, err)
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
