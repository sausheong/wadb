package tools

import "github.com/sausheong/wadb/internal/mcp/cursor"

// Thin wrappers so handler files can use cursors with a stable internal name.
func decodeCursorImpl(s string) (cursor.Cursor, error) { return cursor.Decode(s) }
func encodeCursorImpl(ts int64, id string) string      { return cursor.Cursor{Ts: ts, ID: id}.Encode() }
