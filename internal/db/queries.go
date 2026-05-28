package db

import (
	"context"
	"database/sql"
	"fmt"
)

type Contact struct {
	JID, PushName, BusinessName, Phone string
	IsBlocked                          bool
	UpdatedAt                          int64
}

type Chat struct {
	Kind             string // "dm" | "group"
	JID              string
	LastMessageAt    int64
	UnreadCount      int
	Archived, Pinned bool
	MutedUntil       int64
}

type Group struct {
	JID, Name, Topic, OwnerJID string
	CreatedAt, UpdatedAt       int64
}

type GroupParticipant struct {
	GroupJID, ContactJID string
	IsAdmin              bool
	JoinedAt             int64
}

type Message struct {
	ID, ChatJID, SenderJID string
	FromMe                 bool
	Timestamp              int64
	Kind                   string
	Text                   *string // nullable
	QuotedID               string  // empty = none
	Reactions              string  // JSON, empty = none
	EditedAt, DeletedAt    int64   // 0 = not set
	Raw                    string  // JSON
}

type Media struct {
	ID                         int64
	MessageChatJID, MessageID  string
	MimeType                   string
	Size                       int64
	SHA256                     string
	Width, Height, DurationSec int
	DownloadRef                string
	LocalPath                  string
	DownloadedAt               int64
}

type Queries struct{ db *sql.DB }

func NewQueries(db *sql.DB) *Queries { return &Queries{db: db} }

// DB exposes the underlying connection for ingester transactions.
func (q *Queries) DB() *sql.DB { return q.db }

// --- Contacts ---

func (q *Queries) UpsertContact(ctx context.Context, c Contact) error {
	_, err := q.db.ExecContext(ctx, `
INSERT INTO contacts (jid, push_name, business_name, phone, is_blocked, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(jid) DO UPDATE SET
  push_name     = excluded.push_name,
  business_name = excluded.business_name,
  phone         = excluded.phone,
  is_blocked    = excluded.is_blocked,
  updated_at    = excluded.updated_at
WHERE excluded.updated_at >= contacts.updated_at`,
		c.JID, nullStr(c.PushName), nullStr(c.BusinessName), nullStr(c.Phone), c.IsBlocked, c.UpdatedAt)
	return err
}

func (q *Queries) GetContact(ctx context.Context, jid string) (Contact, error) {
	var c Contact
	var pn, bn, ph sql.NullString
	err := q.db.QueryRowContext(ctx,
		`SELECT jid, push_name, business_name, phone, is_blocked, updated_at FROM contacts WHERE jid=?`, jid).
		Scan(&c.JID, &pn, &bn, &ph, &c.IsBlocked, &c.UpdatedAt)
	c.PushName, c.BusinessName, c.Phone = pn.String, bn.String, ph.String
	return c, err
}

func (q *Queries) SearchContacts(ctx context.Context, query string, limit int) ([]Contact, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT jid, push_name, business_name, phone, is_blocked, updated_at
FROM contacts
WHERE (? = '' OR push_name LIKE ? OR business_name LIKE ? OR phone LIKE ?)
ORDER BY push_name ASC
LIMIT ?`, query, "%"+query+"%", "%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Contact
	for rows.Next() {
		var c Contact
		var pn, bn, ph sql.NullString
		if err := rows.Scan(&c.JID, &pn, &bn, &ph, &c.IsBlocked, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.PushName, c.BusinessName, c.Phone = pn.String, bn.String, ph.String
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- Chats ---

// UpsertChat inserts a new chat row or bumps last_message_at on an existing one.
// Flag fields (archived, pinned, muted_until, unread_count) are NOT overwritten
// on conflict — use UpdateChatFlags for that. This is what the ingester wants:
// it sees a new message and shouldn't unarchive/unpin a chat as a side effect.
func (q *Queries) UpsertChat(ctx context.Context, c Chat) error {
	_, err := q.db.ExecContext(ctx, `
INSERT INTO chats (jid, kind, last_message_at, unread_count, archived, pinned, muted_until)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(jid) DO UPDATE SET
  last_message_at = MAX(IFNULL(chats.last_message_at,0), IFNULL(excluded.last_message_at,0)),
  kind            = excluded.kind`,
		c.JID, c.Kind, nullInt(c.LastMessageAt), c.UnreadCount, c.Archived, c.Pinned, nullInt(c.MutedUntil))
	return err
}

// UpdateChatFlags overwrites the user-facing chat flags. Use this when an
// explicit Archive/Pin/Mute event arrives — not on every inbound message.
func (q *Queries) UpdateChatFlags(ctx context.Context, jid string, archived, pinned bool, mutedUntil int64) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE chats SET archived=?, pinned=?, muted_until=? WHERE jid=?`,
		archived, pinned, nullInt(mutedUntil), jid)
	return err
}

func (q *Queries) ListChats(ctx context.Context, kind string, beforeTs int64, beforeID string, limit int) ([]Chat, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT jid, kind, last_message_at, unread_count, archived, pinned, muted_until
FROM chats
WHERE (? = '' OR kind = ?)
  AND (? = 0 OR IFNULL(last_message_at,0) < ?)
ORDER BY IFNULL(last_message_at,0) DESC, jid DESC
LIMIT ?`, kind, kind, beforeTs, beforeTs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chat
	for rows.Next() {
		var c Chat
		var lma, mu sql.NullInt64
		if err := rows.Scan(&c.JID, &c.Kind, &lma, &c.UnreadCount, &c.Archived, &c.Pinned, &mu); err != nil {
			return nil, err
		}
		c.LastMessageAt, c.MutedUntil = lma.Int64, mu.Int64
		out = append(out, c)
	}
	return out, rows.Err()
}

func (q *Queries) GetChat(ctx context.Context, jid string) (Chat, error) {
	var c Chat
	var lma, mu sql.NullInt64
	err := q.db.QueryRowContext(ctx,
		`SELECT jid, kind, last_message_at, unread_count, archived, pinned, muted_until FROM chats WHERE jid=?`, jid).
		Scan(&c.JID, &c.Kind, &lma, &c.UnreadCount, &c.Archived, &c.Pinned, &mu)
	c.LastMessageAt, c.MutedUntil = lma.Int64, mu.Int64
	return c, err
}

// --- Groups & participants ---

func (q *Queries) UpsertGroup(ctx context.Context, g Group) error {
	_, err := q.db.ExecContext(ctx, `
INSERT INTO groups (jid, name, topic, owner_jid, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(jid) DO UPDATE SET
  name = excluded.name, topic = excluded.topic, owner_jid = excluded.owner_jid,
  updated_at = excluded.updated_at
WHERE excluded.updated_at >= groups.updated_at`,
		g.JID, nullStr(g.Name), nullStr(g.Topic), nullStr(g.OwnerJID), nullInt(g.CreatedAt), g.UpdatedAt)
	return err
}

func (q *Queries) SearchGroups(ctx context.Context, query string, limit int) ([]Group, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT jid, name, topic, owner_jid, created_at, updated_at
FROM groups
WHERE (? = '' OR name LIKE ?)
ORDER BY name ASC
LIMIT ?`, query, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		var name, topic, owner sql.NullString
		var created sql.NullInt64
		if err := rows.Scan(&g.JID, &name, &topic, &owner, &created, &g.UpdatedAt); err != nil {
			return nil, err
		}
		g.Name, g.Topic, g.OwnerJID, g.CreatedAt = name.String, topic.String, owner.String, created.Int64
		out = append(out, g)
	}
	return out, rows.Err()
}

func (q *Queries) SetGroupParticipants(ctx context.Context, groupJID string, parts []GroupParticipant) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_participants WHERE group_jid=?`, groupJID); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO group_participants(group_jid, contact_jid, is_admin, joined_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, p := range parts {
		if _, err := stmt.ExecContext(ctx, p.GroupJID, p.ContactJID, p.IsAdmin, nullInt(p.JoinedAt)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (q *Queries) GetGroupParticipants(ctx context.Context, groupJID string) ([]GroupParticipant, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT group_jid, contact_jid, is_admin, joined_at FROM group_participants WHERE group_jid=?`, groupJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GroupParticipant
	for rows.Next() {
		var p GroupParticipant
		var joined sql.NullInt64
		if err := rows.Scan(&p.GroupJID, &p.ContactJID, &p.IsAdmin, &joined); err != nil {
			return nil, err
		}
		p.JoinedAt = joined.Int64
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- Messages ---

func (q *Queries) InsertMessage(ctx context.Context, m Message) error {
	res, err := q.db.ExecContext(ctx, `
INSERT OR IGNORE INTO messages
  (id, chat_jid, sender_jid, from_me, timestamp, kind, text, quoted_id, reactions, edited_at, deleted_at, raw)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ChatJID, m.SenderJID, m.FromMe, m.Timestamp, m.Kind,
		nullStrPtr(m.Text), nullStr(m.QuotedID), nullStr(m.Reactions),
		nullInt(m.EditedAt), nullInt(m.DeletedAt), nullStr(m.Raw))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		_, err = q.db.ExecContext(ctx, `
UPDATE chats
SET last_message_at = MAX(IFNULL(last_message_at,0), ?)
WHERE jid = ?`, m.Timestamp, m.ChatJID)
	}
	return err
}

func (q *Queries) UpdateMessageEdited(ctx context.Context, chatJID, id string, newText string, editedAt int64) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE messages SET text=?, edited_at=? WHERE chat_jid=? AND id=?`,
		newText, editedAt, chatJID, id)
	return err
}

func (q *Queries) TombstoneMessage(ctx context.Context, chatJID, id string, deletedAt int64) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE messages SET text=NULL, deleted_at=? WHERE chat_jid=? AND id=?`,
		deletedAt, chatJID, id)
	return err
}

func (q *Queries) UpdateMessageReactions(ctx context.Context, chatJID, id, reactionsJSON string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE messages SET reactions=? WHERE chat_jid=? AND id=?`,
		reactionsJSON, chatJID, id)
	return err
}

func (q *Queries) GetMessage(ctx context.Context, chatJID, id string) (Message, error) {
	var m Message
	var text, quoted, reactions, raw sql.NullString
	var edited, deleted sql.NullInt64
	err := q.db.QueryRowContext(ctx, `
SELECT id, chat_jid, sender_jid, from_me, timestamp, kind, text, quoted_id, reactions, edited_at, deleted_at, raw
FROM messages WHERE chat_jid=? AND id=?`, chatJID, id).
		Scan(&m.ID, &m.ChatJID, &m.SenderJID, &m.FromMe, &m.Timestamp, &m.Kind,
			&text, &quoted, &reactions, &edited, &deleted, &raw)
	if err != nil {
		return m, err
	}
	if text.Valid {
		s := text.String
		m.Text = &s
	}
	m.QuotedID, m.Reactions, m.Raw = quoted.String, reactions.String, raw.String
	m.EditedAt, m.DeletedAt = edited.Int64, deleted.Int64
	return m, nil
}

func (q *Queries) GetMessages(ctx context.Context, chatJID string, beforeTs int64, beforeID string, limit int) ([]Message, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT id, chat_jid, sender_jid, from_me, timestamp, kind, text, quoted_id, reactions, edited_at, deleted_at, raw
FROM messages
WHERE chat_jid=?
  AND (? = 0 OR (timestamp, id) < (?, ?))
ORDER BY timestamp DESC, id DESC
LIMIT ?`, chatJID, beforeTs, beforeTs, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (q *Queries) SearchMessages(ctx context.Context, query, chatJID, senderJID string, since, until int64, limit int) ([]Message, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT m.id, m.chat_jid, m.sender_jid, m.from_me, m.timestamp, m.kind, m.text, m.quoted_id, m.reactions, m.edited_at, m.deleted_at, m.raw
FROM fts_messages f JOIN messages m ON m.rowid = f.rowid
WHERE f.text MATCH ?
  AND (? = '' OR m.chat_jid = ?)
  AND (? = '' OR m.sender_jid = ?)
  AND (? = 0  OR m.timestamp >= ?)
  AND (? = 0  OR m.timestamp <= ?)
ORDER BY m.timestamp DESC, m.id DESC
LIMIT ?`, query, chatJID, chatJID, senderJID, senderJID, since, since, until, until, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var out []Message
	for rows.Next() {
		var m Message
		var text, quoted, reactions, raw sql.NullString
		var edited, deleted sql.NullInt64
		if err := rows.Scan(&m.ID, &m.ChatJID, &m.SenderJID, &m.FromMe, &m.Timestamp, &m.Kind,
			&text, &quoted, &reactions, &edited, &deleted, &raw); err != nil {
			return nil, err
		}
		if text.Valid {
			s := text.String
			m.Text = &s
		}
		m.QuotedID, m.Reactions, m.Raw = quoted.String, reactions.String, raw.String
		m.EditedAt, m.DeletedAt = edited.Int64, deleted.Int64
		out = append(out, m)
	}
	return out, rows.Err()
}

// --- Media ---

func (q *Queries) UpsertMedia(ctx context.Context, m Media) error {
	_, err := q.db.ExecContext(ctx, `
INSERT INTO media (message_chat_jid, message_id, mime_type, size, sha256, width, height, duration_sec, download_ref, local_path, downloaded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(message_chat_jid, message_id) DO UPDATE SET
  mime_type=excluded.mime_type, size=excluded.size, sha256=excluded.sha256,
  width=excluded.width, height=excluded.height, duration_sec=excluded.duration_sec,
  download_ref=excluded.download_ref`,
		m.MessageChatJID, m.MessageID, m.MimeType, nullInt(m.Size), nullStr(m.SHA256),
		nullIntI(m.Width), nullIntI(m.Height), nullIntI(m.DurationSec), m.DownloadRef,
		nullStr(m.LocalPath), nullInt(m.DownloadedAt))
	return err
}

func (q *Queries) MediaForMessage(ctx context.Context, chatJID, msgID string) (Media, error) {
	var m Media
	var sha, local sql.NullString
	var size, dl sql.NullInt64
	var w, h, d sql.NullInt64
	err := q.db.QueryRowContext(ctx, `
SELECT id, message_chat_jid, message_id, mime_type, size, sha256, width, height, duration_sec, download_ref, local_path, downloaded_at
FROM media WHERE message_chat_jid=? AND message_id=?`, chatJID, msgID).
		Scan(&m.ID, &m.MessageChatJID, &m.MessageID, &m.MimeType, &size, &sha, &w, &h, &d, &m.DownloadRef, &local, &dl)
	if err != nil {
		return m, err
	}
	m.Size = size.Int64
	m.SHA256, m.LocalPath = sha.String, local.String
	m.Width, m.Height, m.DurationSec = int(w.Int64), int(h.Int64), int(d.Int64)
	m.DownloadedAt = dl.Int64
	return m, nil
}

func (q *Queries) RecordMediaDownload(ctx context.Context, chatJID, msgID, localPath, sha256 string, size int64, downloadedAt int64) error {
	_, err := q.db.ExecContext(ctx, `
UPDATE media SET local_path=?, sha256=COALESCE(?, sha256), size=COALESCE(?, size), downloaded_at=?
WHERE message_chat_jid=? AND message_id=?`,
		localPath, nullStr(sha256), nullInt(size), downloadedAt, chatJID, msgID)
	return err
}

// --- Stats / status ---

type DBStats struct {
	Messages, Contacts, Groups       int
	OldestMessageAt, NewestMessageAt int64
}

func (q *Queries) Stats(ctx context.Context) (DBStats, error) {
	var s DBStats
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&s.Messages); err != nil {
		return s, fmt.Errorf("count messages: %w", err)
	}
	_ = q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contacts`).Scan(&s.Contacts)
	_ = q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups`).Scan(&s.Groups)
	var oldest, newest sql.NullInt64
	_ = q.db.QueryRowContext(ctx, `SELECT MIN(timestamp), MAX(timestamp) FROM messages`).Scan(&oldest, &newest)
	s.OldestMessageAt, s.NewestMessageAt = oldest.Int64, newest.Int64
	return s, nil
}

// --- helpers ---

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func nullStrPtr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
func nullInt(i int64) any {
	if i == 0 {
		return nil
	}
	return i
}
func nullIntI(i int) any {
	if i == 0 {
		return nil
	}
	return i
}
