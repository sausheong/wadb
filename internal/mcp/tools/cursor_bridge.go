package tools

// Cursor encode/decode is duplicated here (rather than importing the parent
// internal/mcp package) to avoid an import cycle: internal/mcp imports
// internal/mcp/tools via server.go, so tools cannot import internal/mcp back.
// Format matches internal/mcp.Cursor — base64-url JSON of {"ts": <unix>, "id": "<id>"}.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type cursor struct {
	Ts int64  `json:"ts"`
	ID string `json:"id"`
}

func decodeCursorImpl(s string) (cursor, error) {
	if s == "" {
		return cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	var c cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return cursor{}, fmt.Errorf("unmarshal cursor: %w", err)
	}
	return c, nil
}

func encodeCursorImpl(ts int64, id string) string {
	c := cursor{Ts: ts, ID: id}
	if c == (cursor{}) {
		return ""
	}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
