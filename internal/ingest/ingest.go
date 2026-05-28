// Package ingest writes WhatsApp events into the local SQLite database.
package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/waclient"
)

// Normalized events the handlers operate on. The translator (in waclient
// package, Task 11) converts concrete whatsmeow types into these so tests
// don't need to depend on whatsmeow.

type TestMessage struct {
	ChatJID, SenderJID, ID string
	Text                   string
	Timestamp              time.Time
	Kind                   string // "" defaults to "text"
	QuotedID               string
	FromMe                 bool
}

type TestMedia struct {
	ChatJID, SenderJID, ID string
	Caption                string
	Timestamp              time.Time
	Kind                   string // "image"|"video"|"audio"|"voice"|"document"|"sticker"
	MimeType               string
	Size                   int64
	Width, Height          int
	DurationSec            int
	DownloadRef            string
	FromMe                 bool
}

type TestEdit struct {
	ChatJID, ID string
	NewText     string
	EditedAt    time.Time
}

type TestDelete struct {
	ChatJID, ID string
	DeletedAt   time.Time
}

type TestReaction struct {
	ChatJID, TargetID string
	FromJID           string
	Emoji             string // "" removes reaction from FromJID
	Timestamp         time.Time
}

type TestContact struct {
	JID, PushName, BusinessName, Phone string
	UpdatedAt                          time.Time
}

type TestGroupInfo struct {
	JID, Name, Topic, OwnerJID string
	CreatedAt, UpdatedAt       time.Time
	Participants               []TestGroupParticipant
}

type TestGroupParticipant struct {
	JID      string
	IsAdmin  bool
	JoinedAt time.Time
}

// Ingester drains the WhatsApp event channel and writes to SQLite.
// Single goroutine, single writer to the application DB.
type Ingester struct {
	q      *db.Queries
	client waclient.Client
	log    *slog.Logger

	mu          sync.Mutex
	lastEventAt time.Time
}

func New(q *db.Queries, client waclient.Client) *Ingester {
	return &Ingester{q: q, client: client, log: slog.Default()}
}

func (i *Ingester) SetLogger(l *slog.Logger) { i.log = l }

// Run drains events until ctx is cancelled or the events channel is closed.
func (i *Ingester) Run(ctx context.Context) {
	ch := i.client.Events(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			i.handle(ctx, ev)
		}
	}
}

// LastEventAt returns the wall-clock time the ingester last processed an
// event. Zero time if none yet. Used by the status MCP tool.
func (i *Ingester) LastEventAt() time.Time {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.lastEventAt
}

func (i *Ingester) markEvent() {
	i.mu.Lock()
	i.lastEventAt = time.Now()
	i.mu.Unlock()
}
