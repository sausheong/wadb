package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.up.sql
var migrationFS embed.FS

// Migrate applies any pending migrations in order. Idempotent.
func Migrate(ctx context.Context, conn *sql.DB) error {
	if _, err := conn.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}
	current, err := SchemaVersion(ctx, conn)
	if err != nil {
		return err
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	type m struct {
		version int
		name    string
	}
	var ms []m
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		// NNN_description.up.sql → NNN
		prefix := strings.SplitN(name, "_", 2)[0]
		v, err := strconv.Atoi(prefix)
		if err != nil {
			return fmt.Errorf("bad migration name %q: %w", name, err)
		}
		ms = append(ms, m{v, name})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for _, mig := range ms {
		if mig.version <= current {
			continue
		}
		sqlBytes, err := migrationFS.ReadFile("migrations/" + mig.name)
		if err != nil {
			return fmt.Errorf("read %s: %w", mig.name, err)
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", mig.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_version(version) VALUES (?)`, mig.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record %s: %w", mig.name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// SchemaVersion returns the highest applied migration version, or 0 if none.
func SchemaVersion(ctx context.Context, conn *sql.DB) (int, error) {
	var name string
	err := conn.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'`).Scan(&name)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("check schema_version table: %w", err)
	}
	var v sql.NullInt64
	if err := conn.QueryRowContext(ctx,
		`SELECT MAX(version) FROM schema_version`).Scan(&v); err != nil {
		return 0, fmt.Errorf("read schema_version: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}
