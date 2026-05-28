package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_DefaultsToHomeWadb(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("WADB_HOME", "")
	t.Setenv("HOME", fakeHome)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(fakeHome, ".wadb")
	if c.Home != want {
		t.Errorf("Home = %q, want %q", c.Home, want)
	}
	if c.SessionDB != filepath.Join(want, "session.db") {
		t.Errorf("SessionDB = %q", c.SessionDB)
	}
	if c.AppDB != filepath.Join(want, "wadb.db") {
		t.Errorf("AppDB = %q", c.AppDB)
	}
	if c.MediaDir != filepath.Join(want, "media") {
		t.Errorf("MediaDir = %q", c.MediaDir)
	}
}

func TestLoad_RespectsWADB_HOME(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WADB_HOME", dir)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Home != dir {
		t.Errorf("Home = %q, want %q", c.Home, dir)
	}
}

func TestLoad_CreatesHomeDirectory(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "nested", "wadb")
	t.Setenv("WADB_HOME", target)
	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("Home not created: %v", err)
	}
}

func TestLoad_LogLevelDefaultInfo(t *testing.T) {
	t.Setenv("WADB_LOG_LEVEL", "")
	t.Setenv("WADB_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", c.LogLevel)
	}
}

func TestLoad_LogLevelRespected(t *testing.T) {
	t.Setenv("WADB_LOG_LEVEL", "debug")
	t.Setenv("WADB_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", c.LogLevel)
	}
}

func TestLoad_HomeAndMediaDirsAre0700(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WADB_HOME", dir)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, p := range []string{c.Home, c.MediaDir} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if mode := info.Mode().Perm(); mode != 0o700 {
			t.Errorf("%s perm = %o, want 0700", p, mode)
		}
	}
}
