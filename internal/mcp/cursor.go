// Package mcp wires the Model Context Protocol server for wadb.
//
// This file contains the opaque pagination cursor used by read tools.
package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Cursor is an opaque pagination cursor. Encoded form is base64-url JSON
// of {"ts": <unix>, "id": "<message_id>"}. Callers compare (Ts, ID)
// lexicographically when stepping through results.
type Cursor struct {
	Ts int64  `json:"ts"`
	ID string `json:"id"`
}

func (c Cursor) Encode() string {
	if c == (Cursor{}) {
		return ""
	}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, fmt.Errorf("unmarshal cursor: %w", err)
	}
	return c, nil
}
