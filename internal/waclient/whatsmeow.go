package waclient

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // SQL driver registered as "sqlite"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/sausheong/wadb/internal/waevent"
)

// WhatsmeowClient is the production Client backed by go.mau.fi/whatsmeow.
type WhatsmeowClient struct {
	cli       *whatsmeow.Client
	container *sqlstore.Container
}

// NewWhatsmeow opens (or creates) the whatsmeow store at sessionDBPath and
// returns a WhatsmeowClient. If no device is paired yet, DeviceJID() will
// return "" and Connect will refuse — caller must run `wadb link` first.
func NewWhatsmeow(ctx context.Context, sessionDBPath string) (*WhatsmeowClient, error) {
	dbLog := newSlogAdapter("whatsmeow/db")
	// modernc.org/sqlite registers itself as "sqlite". dbutil.ParseDialect
	// accepts any string starting with "sqlite" as the SQLite dialect.
	dsn := "file:" + sessionDBPath + "?_pragma=foreign_keys(1)"
	container, err := sqlstore.New(ctx, "sqlite", dsn, dbLog)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}
	cli := whatsmeow.NewClient(device, newSlogAdapter("whatsmeow/client"))
	return &WhatsmeowClient{cli: cli, container: container}, nil
}

// Underlying exposes the raw *whatsmeow.Client. The `wadb link` command uses
// this to drive the QR-pairing flow directly.
func (w *WhatsmeowClient) Underlying() *whatsmeow.Client { return w.cli }

// Connect connects the underlying socket. Returns an error if no device is
// paired yet — caller must run `wadb link` first.
func (w *WhatsmeowClient) Connect(_ context.Context) error {
	if w.cli.Store.ID == nil {
		return errors.New("no paired device; run `wadb link` first")
	}
	return w.cli.Connect()
}

func (w *WhatsmeowClient) Events(ctx context.Context) <-chan Event {
	ch := make(chan Event, 32)
	handlerID := w.cli.AddEventHandler(func(rawEvt any) {
		select {
		case ch <- translateEvent(rawEvt):
		case <-ctx.Done():
		}
	})
	go func() {
		<-ctx.Done()
		w.cli.RemoveEventHandler(handlerID)
		close(ch)
	}()
	return ch
}

func (w *WhatsmeowClient) Connected() bool {
	return w.cli.IsConnected() && w.cli.IsLoggedIn()
}

func (w *WhatsmeowClient) DeviceJID() string {
	if w.cli.Store.ID == nil {
		return ""
	}
	return w.cli.Store.ID.String()
}

func (w *WhatsmeowClient) SendText(ctx context.Context, chatJID, text, replyToID string) (SendResult, error) {
	if !w.cli.IsConnected() {
		return SendResult{}, ErrNotConnected
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return SendResult{}, fmt.Errorf("parse chat_jid: %w", err)
	}
	var msg *waE2E.Message
	if replyToID == "" {
		msg = &waE2E.Message{Conversation: proto.String(text)}
	} else {
		msg = &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(text),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:    proto.String(replyToID),
					Participant: proto.String(jid.String()),
				},
			},
		}
	}
	res, err := w.cli.SendMessage(ctx, jid, msg)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{MessageID: res.ID, Timestamp: res.Timestamp}, nil
}

func (w *WhatsmeowClient) SendMedia(ctx context.Context, chatJID, kind string, data []byte, mimeType, caption, replyToID string) (SendResult, error) {
	if !w.cli.IsConnected() {
		return SendResult{}, ErrNotConnected
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return SendResult{}, fmt.Errorf("parse chat_jid: %w", err)
	}
	mediaType, err := uploadMediaType(kind)
	if err != nil {
		return SendResult{}, err
	}
	uploaded, err := w.cli.Upload(ctx, data, mediaType)
	if err != nil {
		return SendResult{}, fmt.Errorf("upload: %w", err)
	}
	msg := buildMediaMessage(kind, &uploaded, mimeType, caption, replyToID, jid.String())
	if msg == nil {
		return SendResult{}, fmt.Errorf("unsupported media kind: %s", kind)
	}
	res, err := w.cli.SendMessage(ctx, jid, msg)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{MessageID: res.ID, Timestamp: res.Timestamp}, nil
}

func uploadMediaType(kind string) (whatsmeow.MediaType, error) {
	switch kind {
	case "image":
		return whatsmeow.MediaImage, nil
	case "video":
		return whatsmeow.MediaVideo, nil
	case "audio", "voice":
		return whatsmeow.MediaAudio, nil
	case "document":
		return whatsmeow.MediaDocument, nil
	}
	return "", fmt.Errorf("unsupported media kind: %s", kind)
}

func (w *WhatsmeowClient) React(ctx context.Context, chatJID, messageID, emoji string) error {
	if !w.cli.IsConnected() {
		return ErrNotConnected
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("parse chat_jid: %w", err)
	}
	if w.cli.Store.ID == nil {
		return errors.New("no paired device")
	}
	rxn := w.cli.BuildReaction(jid, *w.cli.Store.ID, messageID, emoji)
	_, err = w.cli.SendMessage(ctx, jid, rxn)
	return err
}

func (w *WhatsmeowClient) MarkRead(ctx context.Context, chatJID, upToMessageID string) error {
	if !w.cli.IsConnected() {
		return ErrNotConnected
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("parse chat_jid: %w", err)
	}
	if w.cli.Store.ID == nil {
		return errors.New("no paired device")
	}
	ids := []types.MessageID{}
	if upToMessageID != "" {
		ids = append(ids, types.MessageID(upToMessageID))
	}
	// chat = chat JID; sender = self in DMs; for group messages this would
	// ideally be the message sender, but we don't have it from the MCP layer.
	// Passing self is acceptable for marking your own view of the chat.
	return w.cli.MarkRead(ctx, ids, time.Now(), jid, *w.cli.Store.ID)
}

func (w *WhatsmeowClient) Download(ctx context.Context, downloadRef string) (DownloadResult, error) {
	if !w.cli.IsConnected() {
		return DownloadResult{}, ErrNotConnected
	}
	raw, err := base64.StdEncoding.DecodeString(downloadRef)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("decode ref: %w", err)
	}
	var dm waE2E.Message
	if err := proto.Unmarshal(raw, &dm); err != nil {
		return DownloadResult{}, fmt.Errorf("unmarshal ref: %w", err)
	}
	bytes, err := w.cli.DownloadAny(ctx, &dm)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("download: %w", err)
	}
	return DownloadResult{Bytes: bytes, MimeType: extractMIME(&dm)}, nil
}

func (w *WhatsmeowClient) Disconnect() { w.cli.Disconnect() }

// --- Event translation. Convert whatsmeow event types to the ingest
// package's normalized types so the ingester can type-switch on them
// without depending on whatsmeow. ---

func translateEvent(raw any) Event {
	switch e := raw.(type) {
	case *events.Message:
		return translateMessageEvent(e)
	case *events.Contact:
		// events.Contact carries an opaque ContactAction. Best-effort:
		// surface the JID + timestamp so the ingester upserts a row. The
		// PushName arrives via the PushName event instead.
		c := waevent.TestContact{
			JID:       e.JID.String(),
			UpdatedAt: e.Timestamp,
		}
		if c.UpdatedAt.IsZero() {
			c.UpdatedAt = time.Now()
		}
		return c
	case *events.PushName:
		return waevent.TestContact{
			JID:       e.JID.String(),
			PushName:  e.NewPushName,
			UpdatedAt: time.Now(),
		}
	case *events.GroupInfo:
		return translateGroupInfo(e)
	default:
		// Connected, Disconnected, Receipt, LoggedOut, PairSuccess, etc.
		// The ingester logs unhandled events at debug level.
		return raw
	}
}

// translateMessageEvent picks apart an events.Message into one of the
// normalized ingest types. Order matters: ProtocolMessage (edit/delete)
// and Reaction must be checked before falling through to text/media.
func translateMessageEvent(e *events.Message) Event {
	msg := e.Message
	if msg == nil {
		return e
	}

	chatJID := e.Info.Chat.String()
	senderJID := e.Info.Sender.String()
	timestamp := e.Info.Timestamp
	id := e.Info.ID
	fromMe := e.Info.IsFromMe

	// Edits and revokes (deletes) arrive as ProtocolMessage submessages.
	if pm := msg.GetProtocolMessage(); pm != nil {
		key := pm.GetKey()
		if key == nil {
			return e
		}
		targetID := key.GetID()
		switch pm.GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			return waevent.TestDelete{
				ChatJID:   chatJID,
				ID:        targetID,
				DeletedAt: timestamp,
			}
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			edited := pm.GetEditedMessage()
			if edited != nil {
				return waevent.TestEdit{
					ChatJID:  chatJID,
					ID:       targetID,
					NewText:  textOf(edited),
					EditedAt: timestamp,
				}
			}
		}
		return e
	}

	// Reactions
	if rxn := msg.GetReactionMessage(); rxn != nil {
		return waevent.TestReaction{
			ChatJID:   chatJID,
			TargetID:  rxn.GetKey().GetID(),
			FromJID:   senderJID,
			Emoji:     rxn.GetText(),
			Timestamp: timestamp,
		}
	}

	// Media kinds
	if im := msg.GetImageMessage(); im != nil {
		return mediaEventFromSub(e, "image", im, im.GetCaption())
	}
	if vm := msg.GetVideoMessage(); vm != nil {
		return mediaEventFromSub(e, "video", vm, vm.GetCaption())
	}
	if am := msg.GetAudioMessage(); am != nil {
		kind := "audio"
		if am.GetPTT() {
			kind = "voice"
		}
		return mediaEventFromSub(e, kind, am, "")
	}
	if dm := msg.GetDocumentMessage(); dm != nil {
		return mediaEventFromSub(e, "document", dm, dm.GetCaption())
	}
	if sm := msg.GetStickerMessage(); sm != nil {
		return mediaEventFromSub(e, "sticker", sm, "")
	}

	// Text variants — Conversation (plain) or ExtendedTextMessage (with context).
	text := textOf(msg)
	quotedID := ""
	if et := msg.GetExtendedTextMessage(); et != nil {
		if ci := et.GetContextInfo(); ci != nil {
			quotedID = ci.GetStanzaID()
		}
	}
	return waevent.TestMessage{
		ChatJID:   chatJID,
		SenderJID: senderJID,
		ID:        id,
		Text:      text,
		Timestamp: timestamp,
		FromMe:    fromMe,
		QuotedID:  quotedID,
	}
}

// textOf extracts a plain-text body from a Message, considering both
// Conversation and ExtendedTextMessage variants.
func textOf(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if c := msg.GetConversation(); c != "" {
		return c
	}
	if et := msg.GetExtendedTextMessage(); et != nil {
		return et.GetText()
	}
	return ""
}

// mediaEventFromSub builds an waevent.TestMedia from any media submessage.
// The submessage is wrapped in a parent *waE2E.Message and proto-marshaled
// + base64-encoded as DownloadRef so a later Download call can decrypt it.
func mediaEventFromSub(e *events.Message, kind string, sub any, caption string) Event {
	parent := &waE2E.Message{}
	switch sm := sub.(type) {
	case *waE2E.ImageMessage:
		parent.ImageMessage = sm
	case *waE2E.VideoMessage:
		parent.VideoMessage = sm
	case *waE2E.AudioMessage:
		parent.AudioMessage = sm
	case *waE2E.DocumentMessage:
		parent.DocumentMessage = sm
	case *waE2E.StickerMessage:
		parent.StickerMessage = sm
	default:
		return e
	}
	raw, err := proto.Marshal(parent)
	if err != nil {
		return e
	}
	downloadRef := base64.StdEncoding.EncodeToString(raw)

	mime, size, width, height, dur := mediaMeta(sub)
	return waevent.TestMedia{
		ChatJID:     e.Info.Chat.String(),
		SenderJID:   e.Info.Sender.String(),
		ID:          e.Info.ID,
		Timestamp:   e.Info.Timestamp,
		FromMe:      e.Info.IsFromMe,
		Kind:        kind,
		Caption:     caption,
		MimeType:    mime,
		Size:        size,
		Width:       width,
		Height:      height,
		DurationSec: dur,
		DownloadRef: downloadRef,
	}
}

// mediaMeta extracts mime/size/dimensions/duration from any media submessage.
func mediaMeta(sub any) (mime string, size int64, w, h, dur int) {
	switch m := sub.(type) {
	case *waE2E.ImageMessage:
		return m.GetMimetype(), int64(m.GetFileLength()), int(m.GetWidth()), int(m.GetHeight()), 0
	case *waE2E.VideoMessage:
		return m.GetMimetype(), int64(m.GetFileLength()), int(m.GetWidth()), int(m.GetHeight()), int(m.GetSeconds())
	case *waE2E.AudioMessage:
		return m.GetMimetype(), int64(m.GetFileLength()), 0, 0, int(m.GetSeconds())
	case *waE2E.DocumentMessage:
		return m.GetMimetype(), int64(m.GetFileLength()), 0, 0, 0
	case *waE2E.StickerMessage:
		return m.GetMimetype(), int64(m.GetFileLength()), int(m.GetWidth()), int(m.GetHeight()), 0
	}
	return "", 0, 0, 0, 0
}

// extractMIME recovers the MIME type from a download-ref parent message
// (used by Download to populate DownloadResult.MimeType).
func extractMIME(dm *waE2E.Message) string {
	if m := dm.GetImageMessage(); m != nil {
		return m.GetMimetype()
	}
	if m := dm.GetVideoMessage(); m != nil {
		return m.GetMimetype()
	}
	if m := dm.GetAudioMessage(); m != nil {
		return m.GetMimetype()
	}
	if m := dm.GetDocumentMessage(); m != nil {
		return m.GetMimetype()
	}
	if m := dm.GetStickerMessage(); m != nil {
		return m.GetMimetype()
	}
	return "application/octet-stream"
}

// translateGroupInfo flattens an events.GroupInfo into an waevent.TestGroupInfo.
// Join/Promote are surfaced as participants. WhatsApp delivers these as
// deltas (only the changed participants are present in any one event) rather
// than full rosters; the ingester's group-participant handler is upsert-based
// so partial updates are fine. TODO: Leave/Demote require a separate ingest
// event type to remove/downgrade rows.
func translateGroupInfo(e *events.GroupInfo) Event {
	g := waevent.TestGroupInfo{
		JID:       e.JID.String(),
		UpdatedAt: e.Timestamp,
	}
	if g.UpdatedAt.IsZero() {
		g.UpdatedAt = time.Now()
	}
	if e.Name != nil {
		g.Name = e.Name.Name
	}
	if e.Topic != nil {
		g.Topic = e.Topic.Topic
	}
	for _, j := range e.Join {
		g.Participants = append(g.Participants, waevent.TestGroupParticipant{
			JID:      j.String(),
			JoinedAt: g.UpdatedAt,
		})
	}
	for _, j := range e.Promote {
		g.Participants = append(g.Participants, waevent.TestGroupParticipant{
			JID:      j.String(),
			IsAdmin:  true,
			JoinedAt: g.UpdatedAt,
		})
	}
	return g
}

// buildMediaMessage constructs the *waE2E.Message for outbound media of
// the given kind, populated from the Upload response.
func buildMediaMessage(kind string, up *whatsmeow.UploadResponse, mime, caption, replyToID, replyParticipant string) *waE2E.Message {
	ctxInfo := buildContextInfo(replyToID, replyParticipant)
	switch kind {
	case "image":
		m := &waE2E.ImageMessage{
			Mimetype:      proto.String(mime),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
		}
		if caption != "" {
			m.Caption = proto.String(caption)
		}
		if ctxInfo != nil {
			m.ContextInfo = ctxInfo
		}
		return &waE2E.Message{ImageMessage: m}
	case "video":
		m := &waE2E.VideoMessage{
			Mimetype:      proto.String(mime),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
		}
		if caption != "" {
			m.Caption = proto.String(caption)
		}
		if ctxInfo != nil {
			m.ContextInfo = ctxInfo
		}
		return &waE2E.Message{VideoMessage: m}
	case "audio", "voice":
		m := &waE2E.AudioMessage{
			Mimetype:      proto.String(mime),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
		}
		if kind == "voice" {
			m.PTT = proto.Bool(true)
		}
		if ctxInfo != nil {
			m.ContextInfo = ctxInfo
		}
		return &waE2E.Message{AudioMessage: m}
	case "document":
		m := &waE2E.DocumentMessage{
			Mimetype:      proto.String(mime),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
		}
		if caption != "" {
			m.Caption = proto.String(caption)
		}
		if ctxInfo != nil {
			m.ContextInfo = ctxInfo
		}
		return &waE2E.Message{DocumentMessage: m}
	}
	return nil
}

func buildContextInfo(replyToID, replyParticipant string) *waE2E.ContextInfo {
	if replyToID == "" {
		return nil
	}
	return &waE2E.ContextInfo{
		StanzaID:    proto.String(replyToID),
		Participant: proto.String(replyParticipant),
	}
}

// Compile-time check that Fake and WhatsmeowClient both satisfy Client.
var _ Client = (*Fake)(nil)
var _ Client = (*WhatsmeowClient)(nil)
