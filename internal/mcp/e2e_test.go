package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/ingest"
	"github.com/sausheong/wadb/internal/mcp"
	"github.com/sausheong/wadb/internal/waclient"
)

// TestServer_ListChats_E2E drives the MCP server end-to-end through mcp-go's
// in-process transport: a real client speaks JSON-RPC to a real MCPServer
// across the transport.Interface boundary (no direct handler invocation).
// This exercises tool registration, request routing, and result framing.
func TestServer_ListChats_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmp := t.TempDir()
	conn, err := db.Open(filepath.Join(tmp, "e.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	q := db.NewQueries(conn)
	if err := q.UpsertChat(ctx, db.Chat{
		JID:           "z@s.whatsapp.net",
		Kind:          "dm",
		LastMessageAt: 1716800000,
	}); err != nil {
		t.Fatalf("seed chat: %v", err)
	}

	fake := waclient.NewFake()
	ing := ingest.New(q, fake)
	srv := mcp.New(q, fake, ing, filepath.Join(tmp, "media"), nil)

	// In-process transport: client and server share memory but communicate
	// through the same JSON-RPC framing used over stdio/HTTP.
	c, err := mcpclient.NewInProcessClient(srv.Underlying())
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	defer c.Close()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("client start: %v", err)
	}

	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpgo.Implementation{Name: "wadb-e2e", Version: "0.0.1"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	callReq := mcpgo.CallToolRequest{}
	callReq.Params.Name = "list_chats"
	callReq.Params.Arguments = map[string]any{"limit": float64(10)}

	res, err := c.CallTool(ctx, callReq)
	if err != nil {
		t.Fatalf("call list_chats: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_chats returned IsError=true: %+v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatalf("list_chats returned no content")
	}
	text, ok := res.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("first content not TextContent: %T", res.Content[0])
	}

	var payload struct {
		Chats []struct {
			JID  string `json:"jid"`
			Kind string `json:"kind"`
		} `json:"chats"`
	}
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v (raw: %q)", err, text.Text)
	}
	if len(payload.Chats) != 1 {
		t.Fatalf("got %d chats, want 1: %+v", len(payload.Chats), payload.Chats)
	}
	if payload.Chats[0].JID != "z@s.whatsapp.net" {
		t.Errorf("chat JID = %q, want z@s.whatsapp.net", payload.Chats[0].JID)
	}
	if payload.Chats[0].Kind != "dm" {
		t.Errorf("chat Kind = %q, want dm", payload.Chats[0].Kind)
	}
}
