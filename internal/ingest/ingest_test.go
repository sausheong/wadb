package ingest

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/waclient"
)

func setup(t *testing.T) (*db.Queries, *waclient.Fake, *Ingester) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := db.NewQueries(conn)
	fake := waclient.NewFake()
	ing := New(q, fake)
	t.Cleanup(func() { conn.Close() })
	return q, fake, ing
}

func TestIngest_TextMessage(t *testing.T) {
	q, fake, ing := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); ing.Run(ctx) }()

	fake.EventsCh <- TestMessage{
		ChatJID: "1@s.whatsapp.net", SenderJID: "1@s.whatsapp.net",
		ID: "M1", Text: "hello", Timestamp: time.Unix(1716800000, 0),
	}
	close(fake.EventsCh)
	wg.Wait()

	m, err := q.GetMessage(ctx, "1@s.whatsapp.net", "M1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if m.Text == nil || *m.Text != "hello" {
		t.Errorf("Text = %v", m.Text)
	}
}

func TestIngest_Edit(t *testing.T) {
	q, fake, ing := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); ing.Run(ctx) }()

	fake.EventsCh <- TestMessage{ChatJID: "c@s.whatsapp.net", SenderJID: "c@s.whatsapp.net",
		ID: "M1", Text: "v1", Timestamp: time.Unix(100, 0)}
	fake.EventsCh <- TestEdit{ChatJID: "c@s.whatsapp.net", ID: "M1", NewText: "v2", EditedAt: time.Unix(200, 0)}
	close(fake.EventsCh)
	wg.Wait()

	m, _ := q.GetMessage(ctx, "c@s.whatsapp.net", "M1")
	if m.Text == nil || *m.Text != "v2" {
		t.Errorf("Text = %v", m.Text)
	}
	if m.EditedAt != 200 {
		t.Errorf("EditedAt = %d", m.EditedAt)
	}
}

func TestIngest_Delete(t *testing.T) {
	q, fake, ing := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); ing.Run(ctx) }()

	fake.EventsCh <- TestMessage{ChatJID: "c@s.whatsapp.net", SenderJID: "c@s.whatsapp.net",
		ID: "M1", Text: "hi", Timestamp: time.Unix(100, 0)}
	fake.EventsCh <- TestDelete{ChatJID: "c@s.whatsapp.net", ID: "M1", DeletedAt: time.Unix(300, 0)}
	close(fake.EventsCh)
	wg.Wait()

	m, _ := q.GetMessage(ctx, "c@s.whatsapp.net", "M1")
	if m.Text != nil {
		t.Errorf("text not cleared: %v", *m.Text)
	}
	if m.DeletedAt != 300 {
		t.Errorf("DeletedAt = %d", m.DeletedAt)
	}
}

func TestIngest_Reaction(t *testing.T) {
	q, fake, ing := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); ing.Run(ctx) }()

	fake.EventsCh <- TestMessage{ChatJID: "c@s.whatsapp.net", SenderJID: "c@s.whatsapp.net",
		ID: "M1", Text: "hi", Timestamp: time.Unix(100, 0)}
	fake.EventsCh <- TestReaction{ChatJID: "c@s.whatsapp.net", TargetID: "M1",
		FromJID: "f@s.whatsapp.net", Emoji: "👍", Timestamp: time.Unix(150, 0)}
	close(fake.EventsCh)
	wg.Wait()

	m, _ := q.GetMessage(ctx, "c@s.whatsapp.net", "M1")
	if m.Reactions == "" {
		t.Error("Reactions not set")
	}
}

func TestIngest_IgnoresUnknownEvent(t *testing.T) {
	_, fake, ing := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); ing.Run(ctx) }()

	fake.EventsCh <- struct{ Foo string }{Foo: "bar"}
	close(fake.EventsCh)
	wg.Wait()
	// success = no panic
}
