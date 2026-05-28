// Package ingest writes WhatsApp events into the local SQLite database.
package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/waclient"
	"github.com/sausheong/wadb/internal/waevent"
)

// Normalized events the handlers operate on. The canonical definitions
// live in internal/waevent so the waclient translator can produce them
// without creating an import cycle (ingest → waclient → ingest). These
// type aliases keep existing handlers and tests unchanged.

type (
	TestMessage          = waevent.TestMessage
	TestMedia            = waevent.TestMedia
	TestEdit             = waevent.TestEdit
	TestDelete           = waevent.TestDelete
	TestReaction         = waevent.TestReaction
	TestContact          = waevent.TestContact
	TestGroupInfo        = waevent.TestGroupInfo
	TestGroupParticipant = waevent.TestGroupParticipant
)

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
