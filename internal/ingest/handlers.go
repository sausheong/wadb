package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sausheong/wadb/internal/db"
)

// handle dispatches one event. Unknown event types are logged at debug.
func (i *Ingester) handle(ctx context.Context, ev any) {
	i.markEvent()
	switch e := ev.(type) {
	case TestMessage:
		i.onMessage(ctx, e)
	case TestMedia:
		i.onMedia(ctx, e)
	case TestEdit:
		i.onEdit(ctx, e)
	case TestDelete:
		i.onDelete(ctx, e)
	case TestReaction:
		i.onReaction(ctx, e)
	case TestContact:
		i.onContact(ctx, e)
	case TestGroupInfo:
		i.onGroupInfo(ctx, e)
	default:
		i.log.Debug("ingest: unhandled event", "type", fmt.Sprintf("%T", ev))
	}
}

func (i *Ingester) onMessage(ctx context.Context, e TestMessage) {
	if err := i.q.UpsertChat(ctx, db.Chat{
		JID: e.ChatJID, Kind: chatKindForJID(e.ChatJID), LastMessageAt: e.Timestamp.Unix(),
	}); err != nil {
		i.log.Error("upsert chat", "err", err)
		return
	}
	if err := i.q.UpsertContact(ctx, db.Contact{JID: e.SenderJID, UpdatedAt: e.Timestamp.Unix()}); err != nil {
		i.log.Error("upsert contact", "err", err)
		return
	}
	kind := e.Kind
	if kind == "" {
		kind = "text"
	}
	var text *string
	if e.Text != "" {
		t := e.Text
		text = &t
	}
	if err := i.q.InsertMessage(ctx, db.Message{
		ID: e.ID, ChatJID: e.ChatJID, SenderJID: e.SenderJID,
		FromMe: e.FromMe, Timestamp: e.Timestamp.Unix(),
		Kind: kind, Text: text, QuotedID: e.QuotedID,
	}); err != nil {
		i.log.Error("insert message", "err", err)
	}
}

func (i *Ingester) onMedia(ctx context.Context, e TestMedia) {
	// Insert the message row first (caption becomes text).
	i.onMessage(ctx, TestMessage{
		ChatJID: e.ChatJID, SenderJID: e.SenderJID, ID: e.ID,
		Text: e.Caption, Timestamp: e.Timestamp, Kind: e.Kind, FromMe: e.FromMe,
	})
	if err := i.q.UpsertMedia(ctx, db.Media{
		MessageChatJID: e.ChatJID, MessageID: e.ID,
		MimeType: e.MimeType, Size: e.Size,
		Width: e.Width, Height: e.Height, DurationSec: e.DurationSec,
		DownloadRef: e.DownloadRef,
	}); err != nil {
		i.log.Error("upsert media", "err", err)
	}
}

func (i *Ingester) onEdit(ctx context.Context, e TestEdit) {
	if err := i.q.UpdateMessageEdited(ctx, e.ChatJID, e.ID, e.NewText, e.EditedAt.Unix()); err != nil {
		i.log.Error("update edited", "err", err)
	}
}

func (i *Ingester) onDelete(ctx context.Context, e TestDelete) {
	if err := i.q.TombstoneMessage(ctx, e.ChatJID, e.ID, e.DeletedAt.Unix()); err != nil {
		i.log.Error("tombstone message", "err", err)
	}
}

type reactionEntry struct {
	FromJID string `json:"from_jid"`
	Emoji   string `json:"emoji"`
	Ts      int64  `json:"ts"`
}

func (i *Ingester) onReaction(ctx context.Context, e TestReaction) {
	m, err := i.q.GetMessage(ctx, e.ChatJID, e.TargetID)
	if err != nil {
		i.log.Warn("reaction for unknown message", "chat", e.ChatJID, "id", e.TargetID, "err", err)
		return
	}
	var arr []reactionEntry
	if m.Reactions != "" {
		_ = json.Unmarshal([]byte(m.Reactions), &arr)
	}
	// Remove any existing entry from this sender.
	out := arr[:0]
	for _, r := range arr {
		if r.FromJID != e.FromJID {
			out = append(out, r)
		}
	}
	if e.Emoji != "" {
		out = append(out, reactionEntry{FromJID: e.FromJID, Emoji: e.Emoji, Ts: e.Timestamp.Unix()})
	}
	b, err := json.Marshal(out)
	if err != nil {
		i.log.Error("marshal reactions", "err", err)
		return
	}
	if err := i.q.UpdateMessageReactions(ctx, e.ChatJID, e.TargetID, string(b)); err != nil {
		i.log.Error("update reactions", "err", err)
	}
}

func (i *Ingester) onContact(ctx context.Context, e TestContact) {
	if err := i.q.UpsertContact(ctx, db.Contact{
		JID: e.JID, PushName: e.PushName, BusinessName: e.BusinessName, Phone: e.Phone,
		UpdatedAt: e.UpdatedAt.Unix(),
	}); err != nil {
		i.log.Error("upsert contact", "err", err)
	}
}

func (i *Ingester) onGroupInfo(ctx context.Context, e TestGroupInfo) {
	created := int64(0)
	if !e.CreatedAt.IsZero() {
		created = e.CreatedAt.Unix()
	}
	if err := i.q.UpsertGroup(ctx, db.Group{
		JID: e.JID, Name: e.Name, Topic: e.Topic, OwnerJID: e.OwnerJID,
		CreatedAt: created, UpdatedAt: e.UpdatedAt.Unix(),
	}); err != nil {
		i.log.Error("upsert group", "err", err)
		return
	}
	if err := i.q.UpsertChat(ctx, db.Chat{JID: e.JID, Kind: "group"}); err != nil {
		i.log.Error("upsert group chat", "err", err)
	}
	parts := make([]db.GroupParticipant, 0, len(e.Participants))
	for _, p := range e.Participants {
		joined := int64(0)
		if !p.JoinedAt.IsZero() {
			joined = p.JoinedAt.Unix()
		}
		if err := i.q.UpsertContact(ctx, db.Contact{JID: p.JID, UpdatedAt: e.UpdatedAt.Unix()}); err != nil {
			i.log.Warn("upsert participant contact", "jid", p.JID, "err", err)
		}
		parts = append(parts, db.GroupParticipant{
			GroupJID: e.JID, ContactJID: p.JID, IsAdmin: p.IsAdmin, JoinedAt: joined,
		})
	}
	if err := i.q.SetGroupParticipants(ctx, e.JID, parts); err != nil {
		i.log.Error("set participants", "err", err)
	}
}

func chatKindForJID(jid string) string {
	if strings.HasSuffix(jid, "@g.us") {
		return "group"
	}
	return "dm"
}
