package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/sausheong/wadb/internal/db"
)

func seedDB(t *testing.T) *db.Queries {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := db.NewQueries(conn)
	t.Cleanup(func() { conn.Close() })
	return q
}

func TestListChats_FiltersByKindAndOrders(t *testing.T) {
	q := seedDB(t)
	ctx := context.Background()
	if err := q.UpsertChat(ctx, db.Chat{JID: "g1@g.us", Kind: "group", LastMessageAt: 200}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, db.Chat{JID: "d1@s.whatsapp.net", Kind: "dm", LastMessageAt: 100}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, db.Chat{JID: "d2@s.whatsapp.net", Kind: "dm", LastMessageAt: 300}); err != nil {
		t.Fatal(err)
	}

	h := NewListChatsHandler(q)
	res, err := h(ctx, callReq(map[string]any{"kind": "dm", "limit": float64(10)}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var out struct {
		Chats []struct {
			JID  string `json:"jid"`
			Kind string `json:"kind"`
		} `json:"chats"`
	}
	if err := json.Unmarshal([]byte(firstTextContent(t, res)), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(out.Chats) != 2 {
		t.Fatalf("got %d chats", len(out.Chats))
	}
	if out.Chats[0].JID != "d2@s.whatsapp.net" {
		t.Errorf("first chat = %q (want d2, newest first)", out.Chats[0].JID)
	}
}

func TestListContacts_SubstringSearch(t *testing.T) {
	q := seedDB(t)
	ctx := context.Background()
	if err := q.UpsertContact(ctx, db.Contact{JID: "1@s.whatsapp.net", PushName: "Alice", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertContact(ctx, db.Contact{JID: "2@s.whatsapp.net", PushName: "Bob", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	h := NewListContactsHandler(q)
	res, _ := h(ctx, callReq(map[string]any{"query": "ali", "limit": float64(10)}))
	var out struct {
		Contacts []struct {
			PushName string `json:"push_name"`
		} `json:"contacts"`
	}
	json.Unmarshal([]byte(firstTextContent(t, res)), &out)
	if len(out.Contacts) != 1 || out.Contacts[0].PushName != "Alice" {
		t.Errorf("Contacts = %+v", out.Contacts)
	}
}

func TestGetChat_Group_IncludesParticipants(t *testing.T) {
	q := seedDB(t)
	ctx := context.Background()
	if err := q.UpsertChat(ctx, db.Chat{JID: "g@g.us", Kind: "group"}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertContact(ctx, db.Contact{JID: "a@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertContact(ctx, db.Contact{JID: "b@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertGroup(ctx, db.Group{JID: "g@g.us", Name: "Test Group", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetGroupParticipants(ctx, "g@g.us", []db.GroupParticipant{
		{GroupJID: "g@g.us", ContactJID: "a@s.whatsapp.net", IsAdmin: true},
		{GroupJID: "g@g.us", ContactJID: "b@s.whatsapp.net"},
	}); err != nil {
		t.Fatal(err)
	}
	h := NewGetChatHandler(q)
	res, _ := h(ctx, callReq(map[string]any{"jid": "g@g.us"}))
	var out struct {
		Kind         string `json:"kind"`
		Participants []struct {
			JID     string `json:"jid"`
			IsAdmin bool   `json:"is_admin"`
		} `json:"participants"`
	}
	json.Unmarshal([]byte(firstTextContent(t, res)), &out)
	if out.Kind != "group" {
		t.Errorf("Kind = %q", out.Kind)
	}
	if len(out.Participants) != 2 {
		t.Errorf("Participants = %d", len(out.Participants))
	}
}

func TestGetChat_MissingJIDIsError(t *testing.T) {
	q := seedDB(t)
	h := NewGetChatHandler(q)
	res, _ := h(context.Background(), callReq(map[string]any{}))
	if !res.IsError {
		t.Error("expected error for missing jid")
	}
}

func callReq(args map[string]any) mcpgo.CallToolRequest {
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	return req
}
