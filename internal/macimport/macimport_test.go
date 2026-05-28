package macimport

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sausheong/wadb/internal/db"
)

// setupFixture builds a fresh ChatStorage.sqlite at <tempdir>/src.sqlite
// from the .sql files in testdata/, and a fresh wadb.db at <tempdir>/dst.sqlite
// with all migrations applied. Returns (srcPath, dstQueries).
func setupFixture(t *testing.T) (string, *db.Queries) {
	t.Helper()
	tmp := t.TempDir()

	srcPath := filepath.Join(tmp, "src.sqlite")
	src, err := sql.Open("sqlite", "file:"+srcPath)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}

	for _, file := range []string{"testdata/fixture_schema.sql", "testdata/fixture.sql"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := src.ExecContext(context.Background(), string(raw)); err != nil {
			t.Fatalf("exec %s: %v", file, err)
		}
	}
	if err := src.Close(); err != nil {
		t.Fatalf("close src: %v", err)
	}

	dstConn, err := db.Open(filepath.Join(tmp, "dst.sqlite"))
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	if err := db.Migrate(context.Background(), dstConn); err != nil {
		t.Fatalf("migrate dst: %v", err)
	}
	t.Cleanup(func() { dstConn.Close() })

	return srcPath, db.NewQueries(dstConn)
}

func TestImport_SkeletonRuns(t *testing.T) {
	srcPath, q := setupFixture(t)
	imp, err := New(srcPath, q)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer imp.Close()
	stats, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Stubs return zero. This test exists to verify the wiring compiles
	// and the pipeline runs end-to-end before per-step tests in Task 5.
	if stats.Errors != 0 {
		t.Errorf("unexpected errors: %d", stats.Errors)
	}
}

func TestImport_Contacts(t *testing.T) {
	srcPath, q := setupFixture(t)
	imp, _ := New(srcPath, q)
	defer imp.Close()
	stats, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if stats.Contacts != 3 {
		t.Errorf("Contacts = %d, want 3", stats.Contacts)
	}
	for _, jid := range []string{"111@s.whatsapp.net", "222@s.whatsapp.net", "333@s.whatsapp.net"} {
		c, err := q.GetContact(context.Background(), jid)
		if err != nil {
			t.Errorf("missing contact %s: %v", jid, err)
		}
		if c.JID != jid {
			t.Errorf("contact JID = %q, want %q", c.JID, jid)
		}
	}
}

func TestImport_Chats(t *testing.T) {
	srcPath, q := setupFixture(t)
	imp, _ := New(srcPath, q)
	defer imp.Close()
	if _, err := imp.Import(context.Background()); err != nil {
		t.Fatalf("Import: %v", err)
	}
	for _, c := range []struct {
		jid, kind string
	}{
		{"111@s.whatsapp.net", "dm"},
		{"222@s.whatsapp.net", "dm"},
		{"group1@g.us", "group"},
	} {
		got, err := q.GetChat(context.Background(), c.jid)
		if err != nil {
			t.Errorf("missing chat %s: %v", c.jid, err)
			continue
		}
		if got.Kind != c.kind {
			t.Errorf("chat %s kind = %q, want %q", c.jid, got.Kind, c.kind)
		}
	}
}

func TestImport_GroupsAndParticipants(t *testing.T) {
	srcPath, q := setupFixture(t)
	imp, _ := New(srcPath, q)
	defer imp.Close()
	stats, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if stats.Groups != 1 {
		t.Errorf("Groups = %d, want 1", stats.Groups)
	}
	if stats.Participants != 3 {
		t.Errorf("Participants = %d, want 3", stats.Participants)
	}
	parts, err := q.GetGroupParticipants(context.Background(), "group1@g.us")
	if err != nil {
		t.Fatalf("GetGroupParticipants: %v", err)
	}
	if len(parts) != 3 {
		t.Errorf("participants in DB = %d, want 3", len(parts))
	}
	adminCount := 0
	for _, p := range parts {
		if p.IsAdmin {
			adminCount++
		}
	}
	if adminCount != 1 {
		t.Errorf("admin count = %d, want 1", adminCount)
	}
}

func TestImport_Messages(t *testing.T) {
	srcPath, q := setupFixture(t)
	imp, _ := New(srcPath, q)
	defer imp.Close()
	stats, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Fixture has 8 ZWAMESSAGE rows; 1 has NULL ZSTANZAID and is skipped.
	if stats.Messages != 7 {
		t.Errorf("Messages = %d, want 7", stats.Messages)
	}
	if stats.SkippedMessages != 1 {
		t.Errorf("SkippedMessages = %d, want 1", stats.SkippedMessages)
	}

	// Text message
	m, err := q.GetMessage(context.Background(), "111@s.whatsapp.net", "MSG-DM-1")
	if err != nil {
		t.Fatalf("MSG-DM-1: %v", err)
	}
	if m.Kind != "text" || m.Text == nil || *m.Text != "hello" {
		t.Errorf("MSG-DM-1: %+v", m)
	}
	// Outbound: from_me = true
	m2, _ := q.GetMessage(context.Background(), "111@s.whatsapp.net", "MSG-DM-2")
	if !m2.FromMe {
		t.Errorf("MSG-DM-2 FromMe = false")
	}
	// Reply: quoted_id set
	m5, _ := q.GetMessage(context.Background(), "group1@g.us", "MSG-GR-2")
	if m5.QuotedID != "MSG-GR-1" {
		t.Errorf("MSG-GR-2 QuotedID = %q, want MSG-GR-1", m5.QuotedID)
	}
	// Unknown ZMESSAGETYPE → system
	mU, _ := q.GetMessage(context.Background(), "group1@g.us", "MSG-UNKN")
	if mU.Kind != "system" {
		t.Errorf("MSG-UNKN kind = %q, want system", mU.Kind)
	}
	// Image kind from ZMESSAGETYPE=1
	m3, _ := q.GetMessage(context.Background(), "222@s.whatsapp.net", "MSG-DM-3")
	if m3.Kind != "image" {
		t.Errorf("MSG-DM-3 kind = %q, want image", m3.Kind)
	}
}

func TestImport_Media(t *testing.T) {
	srcPath, q := setupFixture(t)
	imp, _ := New(srcPath, q)
	defer imp.Close()
	stats, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if stats.Media != 1 {
		t.Errorf("Media = %d, want 1", stats.Media)
	}
	m, err := q.MediaForMessage(context.Background(), "222@s.whatsapp.net", "MSG-DM-3")
	if err != nil {
		t.Fatalf("MediaForMessage: %v", err)
	}
	if m.Size != 102400 {
		t.Errorf("Size = %d, want 102400", m.Size)
	}
	if m.MimeType != "image/jpeg" {
		t.Errorf("MimeType = %q, want image/jpeg", m.MimeType)
	}
	if m.DownloadRef != "" {
		t.Errorf("DownloadRef = %q, want empty for imported media", m.DownloadRef)
	}
}

func TestImport_Idempotent(t *testing.T) {
	srcPath, q := setupFixture(t)
	imp, _ := New(srcPath, q)
	defer imp.Close()
	if _, err := imp.Import(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	stats, _ := q.Stats(context.Background())
	first := stats.Messages

	if _, err := imp.Import(context.Background()); err != nil {
		t.Fatalf("second: %v", err)
	}
	stats2, _ := q.Stats(context.Background())
	if stats2.Messages != first {
		t.Errorf("messages count grew on re-import: %d -> %d", first, stats2.Messages)
	}
}
