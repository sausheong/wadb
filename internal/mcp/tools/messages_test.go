package tools

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/sausheong/wadb/internal/db"
)

func TestGetMessages_NewestFirstPaginated(t *testing.T) {
	q := seedDB(t)
	ctx := context.Background()
	if err := q.UpsertContact(ctx, db.Contact{JID: "s@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, db.Chat{JID: "s@s.whatsapp.net", Kind: "dm"}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		txt := "m" + strconv.Itoa(i)
		if err := q.InsertMessage(ctx, db.Message{
			ID: strconv.Itoa(i), ChatJID: "s@s.whatsapp.net", SenderJID: "s@s.whatsapp.net",
			Timestamp: int64(1000 + i), Kind: "text", Text: &txt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	h := NewGetMessagesHandler(q)
	res, err := h(ctx, callReq(map[string]any{"chat_jid": "s@s.whatsapp.net", "limit": float64(2)}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		NextCursor string `json:"next_cursor"`
	}
	json.Unmarshal([]byte(firstTextContent(t, res)), &out)
	if len(out.Messages) != 2 || out.Messages[0].ID != "4" {
		t.Errorf("messages = %+v", out.Messages)
	}
	if out.NextCursor == "" {
		t.Error("expected next_cursor when page is full")
	}
}

func TestSearchMessages_HitsFTS(t *testing.T) {
	q := seedDB(t)
	ctx := context.Background()
	if err := q.UpsertContact(ctx, db.Contact{JID: "s@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, db.Chat{JID: "s@s.whatsapp.net", Kind: "dm"}); err != nil {
		t.Fatal(err)
	}
	tx := "the rain in spain"
	if err := q.InsertMessage(ctx, db.Message{
		ID: "M1", ChatJID: "s@s.whatsapp.net", SenderJID: "s@s.whatsapp.net",
		Timestamp: 1, Kind: "text", Text: &tx,
	}); err != nil {
		t.Fatal(err)
	}
	h := NewSearchMessagesHandler(q)
	res, _ := h(ctx, callReq(map[string]any{"query": "spain", "limit": float64(10)}))
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	json.Unmarshal([]byte(firstTextContent(t, res)), &out)
	if len(out.Messages) != 1 || out.Messages[0].ID != "M1" {
		t.Errorf("hits = %+v", out.Messages)
	}
}

func TestGetMessage_Single(t *testing.T) {
	q := seedDB(t)
	ctx := context.Background()
	if err := q.UpsertContact(ctx, db.Contact{JID: "s@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, db.Chat{JID: "s@s.whatsapp.net", Kind: "dm"}); err != nil {
		t.Fatal(err)
	}
	tx := "hello"
	if err := q.InsertMessage(ctx, db.Message{
		ID: "M1", ChatJID: "s@s.whatsapp.net", SenderJID: "s@s.whatsapp.net",
		Timestamp: 1, Kind: "text", Text: &tx,
	}); err != nil {
		t.Fatal(err)
	}
	h := NewGetMessageHandler(q)
	res, _ := h(ctx, callReq(map[string]any{"chat_jid": "s@s.whatsapp.net", "id": "M1"}))
	var out struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	json.Unmarshal([]byte(firstTextContent(t, res)), &out)
	if out.ID != "M1" || out.Text != "hello" {
		t.Errorf("got %+v", out)
	}
}
