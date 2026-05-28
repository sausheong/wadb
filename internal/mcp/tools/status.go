// Package tools holds MCP tool handlers. Each file groups related tools.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/ingest"
	"github.com/sausheong/wadb/internal/waclient"
)

// ToolHandler matches mcp-go's server.ToolHandlerFunc but lives here so tool
// files don't need to import the server package.
type ToolHandler func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)

// NewStatusHandler returns the handler for the `status` tool.
func NewStatusHandler(q *db.Queries, c waclient.Client, ing *ingest.Ingester) ToolHandler {
	return func(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		stats, err := q.Stats(ctx)
		if err != nil {
			return errResult("stats: " + err.Error()), nil
		}
		var lastEvent int64
		if t := ing.LastEventAt(); !t.IsZero() {
			lastEvent = t.Unix()
		}
		payload := map[string]any{
			"connected":     c.Connected(),
			"linked_device": c.DeviceJID(),
			"last_event_at": lastEvent,
			"db": map[string]any{
				"messages":          stats.Messages,
				"contacts":          stats.Contacts,
				"groups":            stats.Groups,
				"oldest_message_at": stats.OldestMessageAt,
				"newest_message_at": stats.NewestMessageAt,
			},
		}
		return jsonResult(payload), nil
	}
}

// StatusTool is the MCP tool spec for status.
func StatusTool() mcpgo.Tool {
	return mcpgo.NewTool("status",
		mcpgo.WithDescription("Health check: connection state, linked device JID, ingestion freshness, and DB row counts."),
	)
}

// --- shared helpers used by every tool ---

// jsonResult marshals v as JSON and returns it as a successful text content result.
func jsonResult(v any) *mcpgo.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return errResult("encode result: " + err.Error())
	}
	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: string(b)}},
	}
}

// errResult returns a CallToolResult flagged as an error with msg as the body.
func errResult(msg string) *mcpgo.CallToolResult {
	return &mcpgo.CallToolResult{
		IsError: true,
		Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: msg}},
	}
}

// validateRequired returns an error if any of the named arguments are missing
// or are empty strings.
func validateRequired(args map[string]any, names ...string) error {
	for _, n := range names {
		v, ok := args[n]
		if !ok {
			return fmt.Errorf("missing required argument: %s", n)
		}
		if s, isStr := v.(string); isStr && s == "" {
			return fmt.Errorf("empty required argument: %s", n)
		}
	}
	return nil
}

// argStr extracts a string argument, returning "" if missing or not a string.
func argStr(args map[string]any, name string) string {
	if v, ok := args[name].(string); ok {
		return v
	}
	return ""
}

// argInt extracts an int argument, returning def if missing or not numeric.
// JSON unmarshal produces float64 for numeric values, hence the type switch.
func argInt(args map[string]any, name string, def int) int {
	switch v := args[name].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

// argInt64 extracts an int64 argument, returning 0 if missing or not numeric.
func argInt64(args map[string]any, name string) int64 {
	switch v := args[name].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	}
	return 0
}
