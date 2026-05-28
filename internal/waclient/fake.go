package waclient

import (
	"context"
	"sync"
	"time"
)

// Fake is a recording test double. All methods return the canned values
// set on the fields; calls are recorded on the corresponding slices.
type Fake struct {
	mu sync.Mutex

	EventsCh chan Event

	IsConnected bool
	JID         string

	// Send recorders
	SentText  []SentText
	SentMedia []SentMedia
	Reactions []Reaction
	MarkReads []MarkRead

	// Canned responses
	SendErr        error
	NextSendResult SendResult

	DownloadFn func(ref string) (DownloadResult, error)

	DisconnectCalls int
}

type SentText struct {
	ChatJID, Text, ReplyToID string
}
type SentMedia struct {
	ChatJID, Kind, MimeType, Caption, ReplyToID string
	Data                                        []byte
}
type Reaction struct {
	ChatJID, MessageID, Emoji string
}
type MarkRead struct {
	ChatJID, UpToMessageID string
}

func NewFake() *Fake {
	return &Fake{EventsCh: make(chan Event, 16), IsConnected: true}
}

func (f *Fake) Events(ctx context.Context) <-chan Event { return f.EventsCh }
func (f *Fake) Connected() bool                         { return f.IsConnected }
func (f *Fake) DeviceJID() string                       { return f.JID }

func (f *Fake) SendText(_ context.Context, chatJID, text, reply string) (SendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SentText = append(f.SentText, SentText{chatJID, text, reply})
	if f.SendErr != nil {
		return SendResult{}, f.SendErr
	}
	r := f.NextSendResult
	if r.MessageID == "" {
		r = SendResult{MessageID: "FAKE-MSG-ID", Timestamp: time.Unix(1716800000, 0)}
	}
	return r, nil
}

func (f *Fake) SendMedia(_ context.Context, chatJID, kind string, data []byte, mime, caption, reply string) (SendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SentMedia = append(f.SentMedia, SentMedia{chatJID, kind, mime, caption, reply, append([]byte(nil), data...)})
	if f.SendErr != nil {
		return SendResult{}, f.SendErr
	}
	r := f.NextSendResult
	if r.MessageID == "" {
		r = SendResult{MessageID: "FAKE-MEDIA-ID", Timestamp: time.Unix(1716800000, 0)}
	}
	return r, nil
}

func (f *Fake) React(_ context.Context, chatJID, messageID, emoji string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Reactions = append(f.Reactions, Reaction{chatJID, messageID, emoji})
	return f.SendErr
}

func (f *Fake) MarkRead(_ context.Context, chatJID, upTo string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.MarkReads = append(f.MarkReads, MarkRead{chatJID, upTo})
	return f.SendErr
}

func (f *Fake) Download(_ context.Context, ref string) (DownloadResult, error) {
	if f.DownloadFn != nil {
		return f.DownloadFn(ref)
	}
	return DownloadResult{Bytes: []byte("fake-bytes"), MimeType: "application/octet-stream"}, nil
}

func (f *Fake) Disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DisconnectCalls++
}
