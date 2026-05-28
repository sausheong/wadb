package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/ingest"
	"github.com/sausheong/wadb/internal/waclient"
)

func TestStatus_ReturnsConnectionAndDBStats(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := db.NewQueries(conn)
	fake := waclient.NewFake()
	fake.JID = "111:1@s.whatsapp.net"
	ing := ingest.New(q, fake)
	h := NewStatusHandler(q, fake, ing)

	req := mcpgo.CallToolRequest{}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("isError: %+v", res)
	}
	text := firstTextContent(t, res)
	var got struct {
		Connected    bool   `json:"connected"`
		LinkedDevice string `json:"linked_device"`
		LastEventAt  int64  `json:"last_event_at"`
		DB           struct {
			Messages        int   `json:"messages"`
			Contacts        int   `json:"contacts"`
			Groups          int   `json:"groups"`
			OldestMessageAt int64 `json:"oldest_message_at"`
			NewestMessageAt int64 `json:"newest_message_at"`
		} `json:"db"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !got.Connected {
		t.Errorf("Connected = false")
	}
	if got.LinkedDevice != "111:1@s.whatsapp.net" {
		t.Errorf("LinkedDevice = %q", got.LinkedDevice)
	}
	if got.LastEventAt != 0 {
		t.Errorf("LastEventAt = %d, want 0 (no events yet)", got.LastEventAt)
	}
	if got.DB.Messages != 0 {
		t.Errorf("DB.Messages = %d, want 0", got.DB.Messages)
	}
}

func firstTextContent(t *testing.T, res *mcpgo.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("no content")
	}
	tc, ok := res.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want TextContent", res.Content[0])
	}
	return tc.Text
}
