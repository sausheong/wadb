package db

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
)

func newTestDB(t *testing.T) *Queries {
	t.Helper()
	conn, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(context.Background(), conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return NewQueries(conn)
}

func TestUpsertContact_RoundTrip(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	c := Contact{JID: "111@s.whatsapp.net", PushName: "Alice", Phone: "+1...", UpdatedAt: 1000}
	if err := q.UpsertContact(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := q.GetContact(ctx, c.JID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PushName != "Alice" {
		t.Errorf("PushName = %q", got.PushName)
	}
	// Second upsert with older updated_at should be ignored.
	c2 := c
	c2.PushName = "Bob"
	c2.UpdatedAt = 999 // older
	if err := q.UpsertContact(ctx, c2); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	got, _ = q.GetContact(ctx, c.JID)
	if got.PushName != "Alice" {
		t.Errorf("older upsert overwrote newer: %q", got.PushName)
	}
}

func TestInsertMessage_PopulatesFTS(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.UpsertContact(ctx, Contact{JID: "111@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, Chat{JID: "111@s.whatsapp.net", Kind: "dm"}); err != nil {
		t.Fatal(err)
	}
	txt := "hello world"
	m := Message{
		ID: "M1", ChatJID: "111@s.whatsapp.net", SenderJID: "111@s.whatsapp.net",
		FromMe: false, Timestamp: 1716800000, Kind: "text", Text: &txt,
	}
	if err := q.InsertMessage(ctx, m); err != nil {
		t.Fatalf("insert: %v", err)
	}
	hits, err := q.SearchMessages(ctx, "hello", "", "", 0, 0, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "M1" {
		t.Errorf("search hits = %+v", hits)
	}
}

func TestInsertMessage_Idempotent(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.UpsertContact(ctx, Contact{JID: "1@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, Chat{JID: "1@s.whatsapp.net", Kind: "dm"}); err != nil {
		t.Fatal(err)
	}
	txt := "hi"
	m := Message{ID: "M1", ChatJID: "1@s.whatsapp.net", SenderJID: "1@s.whatsapp.net",
		Timestamp: 100, Kind: "text", Text: &txt}
	for i := 0; i < 3; i++ {
		if err := q.InsertMessage(ctx, m); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	hits, _ := q.SearchMessages(ctx, "hi", "", "", 0, 0, 10)
	if len(hits) != 1 {
		t.Errorf("duplicate inserts produced %d rows", len(hits))
	}
}

func TestGetMessages_PaginatesByCursor(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.UpsertContact(ctx, Contact{JID: "1@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, Chat{JID: "1@s.whatsapp.net", Kind: "dm"}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		txt := "m" + strconv.Itoa(i)
		if err := q.InsertMessage(ctx, Message{
			ID: strconv.Itoa(i), ChatJID: "1@s.whatsapp.net", SenderJID: "1@s.whatsapp.net",
			Timestamp: int64(1000 + i), Kind: "text", Text: &txt,
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	page1, err := q.GetMessages(ctx, "1@s.whatsapp.net", 0, "", 2)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 || page1[0].ID != "5" {
		t.Errorf("page1 = %+v", page1)
	}
	page2, _ := q.GetMessages(ctx, "1@s.whatsapp.net", page1[1].Timestamp, page1[1].ID, 2)
	if len(page2) != 2 || page2[0].ID != "3" {
		t.Errorf("page2 = %+v", page2)
	}
}

func TestUpsertChat_DoesNotClobberFlags(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.UpsertChat(ctx, Chat{JID: "c@s.whatsapp.net", Kind: "dm", LastMessageAt: 100}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := q.UpdateChatFlags(ctx, "c@s.whatsapp.net", true, true, 9999); err != nil {
		t.Fatalf("update flags: %v", err)
	}
	// Ingester-style upsert with a newer message — must NOT reset flags.
	if err := q.UpsertChat(ctx, Chat{JID: "c@s.whatsapp.net", Kind: "dm", LastMessageAt: 200}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	got, err := q.GetChat(ctx, "c@s.whatsapp.net")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Archived || !got.Pinned {
		t.Errorf("flags clobbered: archived=%v pinned=%v", got.Archived, got.Pinned)
	}
	if got.MutedUntil != 9999 {
		t.Errorf("muted_until clobbered: %d", got.MutedUntil)
	}
	if got.LastMessageAt != 200 {
		t.Errorf("last_message_at not bumped: %d", got.LastMessageAt)
	}
}

func TestUpsertChat_BumpsLastMessageAtTakingMax(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.UpsertChat(ctx, Chat{JID: "c@s.whatsapp.net", Kind: "dm", LastMessageAt: 200}); err != nil {
		t.Fatal(err)
	}
	// An out-of-order event arrives with an OLDER timestamp.
	if err := q.UpsertChat(ctx, Chat{JID: "c@s.whatsapp.net", Kind: "dm", LastMessageAt: 100}); err != nil {
		t.Fatal(err)
	}
	got, _ := q.GetChat(ctx, "c@s.whatsapp.net")
	if got.LastMessageAt != 200 {
		t.Errorf("LastMessageAt = %d, want 200 (MAX of 200, 100)", got.LastMessageAt)
	}
}

func TestSearchMessages_StableOrderOnTiedTimestamp(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.UpsertContact(ctx, Contact{JID: "s@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, Chat{JID: "s@s.whatsapp.net", Kind: "dm"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"A", "B", "C"} {
		txt := "shared word " + id
		if err := q.InsertMessage(ctx, Message{
			ID: id, ChatJID: "s@s.whatsapp.net", SenderJID: "s@s.whatsapp.net",
			Timestamp: 5000, Kind: "text", Text: &txt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := q.SearchMessages(ctx, "shared", "", "", 0, 0, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("got %d hits", len(hits))
	}
	// With ties on timestamp, secondary sort by id DESC → C, B, A.
	want := []string{"C", "B", "A"}
	for i, h := range hits {
		if h.ID != want[i] {
			t.Errorf("hits[%d] = %q, want %q", i, h.ID, want[i])
		}
	}
}
