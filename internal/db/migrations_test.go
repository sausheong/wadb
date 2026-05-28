package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate_AppliesAllMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(context.Background(), conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	v, err := SchemaVersion(context.Background(), conn)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v < 1 {
		t.Errorf("SchemaVersion = %d, want >= 1", v)
	}
	// Verify expected tables exist.
	for _, table := range []string{
		"contacts", "groups", "group_participants",
		"chats", "messages", "media", "schema_version",
	} {
		var name string
		err := conn.QueryRowContext(context.Background(),
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(context.Background(), conn); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	v1, err := SchemaVersion(context.Background(), conn)
	if err != nil {
		t.Fatalf("first SchemaVersion: %v", err)
	}
	if err := Migrate(context.Background(), conn); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	v2, err := SchemaVersion(context.Background(), conn)
	if err != nil {
		t.Fatalf("second SchemaVersion: %v", err)
	}
	if v1 != v2 {
		t.Errorf("schema version changed on idempotent migrate: %d -> %d", v1, v2)
	}
}

func TestOpen_EnablesWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	var mode string
	if err := conn.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}
