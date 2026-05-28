// Package waevent defines the normalized WhatsApp event types produced by
// the waclient translator and consumed by the ingester.
//
// These types live in their own package so both `waclient` (the producer)
// and `ingest` (the consumer) can import them without an import cycle.
// They're named with a "Test" prefix for historical reasons (the type
// switch in the ingester predates the production translator); the names
// are stable across packages so existing tests continue to compile.
package waevent

import "time"

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
