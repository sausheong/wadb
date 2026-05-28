// Package waclient is the seam between wadb and the WhatsApp library.
// Production uses whatsmeow.go; tests use fake.go.
package waclient

import (
	"context"
	"time"
)

// Event is what whatsmeow delivers; we keep it as `any` here because the
// concrete types live in go.mau.fi/whatsmeow/types/events and we don't
// want to leak that dependency into every package that handles events.
// The ingester does a type switch.
type Event any

// SendResult is what every successful send returns.
type SendResult struct {
	MessageID string
	Timestamp time.Time
}

// DownloadResult is the decrypted payload + metadata returned by Download.
type DownloadResult struct {
	Bytes    []byte
	MimeType string
	SHA256   []byte
}

// Client is the only WhatsApp-touching interface in wadb.
type Client interface {
	// Events returns a channel that receives whatsmeow events until ctx is cancelled.
	Events(ctx context.Context) <-chan Event

	// Connected reports whether the underlying socket is currently connected.
	Connected() bool

	// DeviceJID returns the JID of the linked device (empty if not paired).
	DeviceJID() string

	// SendText sends a text message; replyToID may be empty.
	SendText(ctx context.Context, chatJID, text, replyToID string) (SendResult, error)

	// SendMedia uploads bytes and sends a media message. Kind is whatsmeow's
	// MediaType ("image","video","document","audio").
	SendMedia(ctx context.Context, chatJID, kind string, data []byte, mimeType, caption, replyToID string) (SendResult, error)

	// React adds (or removes, if emoji is "") a reaction on a target message.
	React(ctx context.Context, chatJID, messageID, emoji string) error

	// MarkRead marks a chat read up to upToMessageID (or latest if empty).
	MarkRead(ctx context.Context, chatJID, upToMessageID string) error

	// Download fetches an encrypted media blob given the whatsmeow download
	// reference recorded in the `media` table. Implementation decodes the ref.
	Download(ctx context.Context, downloadRef string) (DownloadResult, error)

	// Disconnect closes the socket cleanly.
	Disconnect()
}
