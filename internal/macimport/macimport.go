package macimport

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"

	"github.com/sausheong/wadb/internal/db"
)

// ImportStats reports per-table counts of rows the importer wrote.
// Duplicate rows (skipped via INSERT OR IGNORE) are not counted in the
// per-table fields. SkippedMessages counts rows we deliberately rejected
// (e.g., ZSTANZAID was NULL). Errors counts per-row failures we logged
// at warn and continued past — never fatal.
type ImportStats struct {
	Contacts        int
	Chats           int
	Groups          int
	Participants    int
	Messages        int
	Media           int
	SkippedMessages int
	Errors          int
}

// Importer copies WhatsApp Desktop history into wadb.db. The source DB
// is opened read-only; the destination is the caller's *db.Queries.
type Importer struct {
	src *sql.DB
	dst *db.Queries
	log *slog.Logger
}

// New opens srcPath read-only and returns an Importer that writes through
// dst. The caller owns dst; the Importer closes src on Close().
func New(srcPath string, dst *db.Queries) (*Importer, error) {
	// mode=ro + immutable=1 = we physically cannot write the source DB.
	// Also avoids creating -wal/-shm sidecars next to the desktop app's.
	dsn := fmt.Sprintf("file:%s?mode=ro&immutable=1", srcPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	if err := conn.PingContext(context.Background()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping source: %w", err)
	}
	return &Importer{src: conn, dst: dst, log: slog.Default()}, nil
}

// Close closes the source DB. Idempotent.
func (i *Importer) Close() error {
	if i.src == nil {
		return nil
	}
	err := i.src.Close()
	i.src = nil
	return err
}

// SetLogger overrides the default slog logger.
func (i *Importer) SetLogger(l *slog.Logger) { i.log = l }

// Import runs the full pipeline in dependency order so FK constraints
// hold. Each step writes through db.Queries and logs (but doesn't fail
// the whole run on) per-row errors.
func (i *Importer) Import(ctx context.Context) (ImportStats, error) {
	var stats ImportStats

	// 1. Contacts — distinct senders + DM partners + group participants.
	c, err := i.importContacts(ctx)
	if err != nil {
		return stats, fmt.Errorf("contacts: %w", err)
	}
	stats.Contacts = c

	// 2. Chats — every ZWACHATSESSION row.
	n, err := i.importChats(ctx)
	if err != nil {
		return stats, fmt.Errorf("chats: %w", err)
	}
	stats.Chats = n

	// 3. Groups + participants — only for ZSESSIONTYPE=1.
	g, p, err := i.importGroupsAndParticipants(ctx)
	if err != nil {
		return stats, fmt.Errorf("groups: %w", err)
	}
	stats.Groups = g
	stats.Participants = p

	// 4. Messages — the bulk.
	m, skipped, errCount, err := i.importMessages(ctx)
	if err != nil {
		return stats, fmt.Errorf("messages: %w", err)
	}
	stats.Messages = m
	stats.SkippedMessages = skipped
	stats.Errors += errCount

	// 5. Media — only for messages we successfully wrote.
	md, err := i.importMedia(ctx)
	if err != nil {
		return stats, fmt.Errorf("media: %w", err)
	}
	stats.Media = md

	return stats, nil
}

// --- per-table writers; stubs filled out in Task 5 ---

func (i *Importer) importContacts(ctx context.Context) (int, error) {
	return 0, nil
}

func (i *Importer) importChats(ctx context.Context) (int, error) {
	return 0, nil
}

func (i *Importer) importGroupsAndParticipants(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

func (i *Importer) importMessages(ctx context.Context) (int, int, int, error) {
	return 0, 0, 0, nil
}

func (i *Importer) importMedia(ctx context.Context) (int, error) {
	return 0, nil
}
