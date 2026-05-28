// Package macimport copies history from the macOS WhatsApp Desktop app's
// local SQLite (ChatStorage.sqlite) into wadb.db.
package macimport

import (
	"database/sql"
)

// coreDataEpoch is the offset between Core Data's epoch (2001-01-01 UTC)
// and the Unix epoch (1970-01-01 UTC), in seconds.
const coreDataEpoch = int64(978307200)

// coreDataToUnix converts a Core Data REAL timestamp (seconds since
// 2001-01-01 UTC, possibly fractional) to a Unix timestamp in seconds.
// Returns 0 for zero or negative inputs — Core Data uses 0 for "unset".
func coreDataToUnix(cd float64) int64 {
	if cd <= 0 {
		return 0
	}
	return int64(cd) + coreDataEpoch
}

// nullStringValue unwraps sql.NullString, returning "" when invalid.
func nullStringValue(s sql.NullString) string {
	if !s.Valid {
		return ""
	}
	return s.String
}

// nullInt64Value unwraps sql.NullInt64, returning 0 when invalid.
func nullInt64Value(n sql.NullInt64) int64 {
	if !n.Valid {
		return 0
	}
	return n.Int64
}

// nullFloat64Value unwraps sql.NullFloat64, returning 0 when invalid.
func nullFloat64Value(f sql.NullFloat64) float64 {
	if !f.Valid {
		return 0
	}
	return f.Float64
}
