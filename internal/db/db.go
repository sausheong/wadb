// Package db owns the application SQLite database (wadb.db).
package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) the SQLite database at path with WAL + sensible defaults.
func Open(path string) (*sql.DB, error) {
	// _journal_mode and _foreign_keys are honored by modernc.org/sqlite.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := conn.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	// The ingester is the sole writer; readers can be many. A small pool is fine.
	conn.SetMaxOpenConns(8)
	return conn, nil
}
