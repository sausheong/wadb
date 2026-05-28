package macimport

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sausheong/wadb/internal/db"
)

// nowUnix is a package-level var so tests can override it for deterministic
// timestamps.
var nowUnix = func() int64 { return time.Now().Unix() }

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
	rows, err := i.src.QueryContext(ctx, `
SELECT DISTINCT jid FROM (
    SELECT ZFROMJID  AS jid FROM ZWAMESSAGE       WHERE ZFROMJID IS NOT NULL AND ZFROMJID <> ''
    UNION
    SELECT ZCONTACTJID FROM ZWACHATSESSION WHERE ZSESSIONTYPE = 0 AND ZCONTACTJID IS NOT NULL
    UNION
    SELECT ZMEMBERJID  FROM ZWAGROUPMEMBER WHERE ZMEMBERJID IS NOT NULL
)
WHERE jid <> ''
ORDER BY jid`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	importedAt := nowUnix()
	n := 0
	for rows.Next() {
		var jid string
		if err := rows.Scan(&jid); err != nil {
			i.log.Warn("scan contact", "err", err)
			continue
		}
		// Best-effort push name from the most recent message we saw from this JID.
		var pushName sql.NullString
		_ = i.src.QueryRowContext(ctx, `
SELECT ZPUSHNAME FROM ZWAMESSAGE
WHERE ZFROMJID = ? AND ZPUSHNAME IS NOT NULL AND ZPUSHNAME <> ''
ORDER BY ZMESSAGEDATE DESC LIMIT 1`, jid).Scan(&pushName)

		if err := i.dst.UpsertContact(ctx, db.Contact{
			JID:       jid,
			PushName:  nullStringValue(pushName),
			UpdatedAt: importedAt,
		}); err != nil {
			i.log.Warn("upsert contact", "jid", jid, "err", err)
			continue
		}
		n++
	}
	return n, rows.Err()
}

func (i *Importer) importChats(ctx context.Context) (int, error) {
	rows, err := i.src.QueryContext(ctx, `
SELECT ZCONTACTJID, ZSESSIONTYPE, ZARCHIVED, ZUNREADCOUNT, ZLASTMESSAGEDATE
FROM ZWACHATSESSION
WHERE ZCONTACTJID IS NOT NULL AND ZCONTACTJID <> ''`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var (
			jid         string
			sessionType sql.NullInt64
			archived    sql.NullInt64
			unread      sql.NullInt64
			lastMsgDate sql.NullFloat64
		)
		if err := rows.Scan(&jid, &sessionType, &archived, &unread, &lastMsgDate); err != nil {
			i.log.Warn("scan chat", "err", err)
			continue
		}
		kind := "dm"
		if sessionType.Valid && sessionType.Int64 == 1 {
			kind = "group"
		}
		if err := i.dst.UpsertChat(ctx, db.Chat{
			JID:           jid,
			Kind:          kind,
			LastMessageAt: coreDataToUnix(nullFloat64Value(lastMsgDate)),
			UnreadCount:   int(nullInt64Value(unread)),
			Archived:      nullInt64Value(archived) == 1,
		}); err != nil {
			i.log.Warn("upsert chat", "jid", jid, "err", err)
			continue
		}
		n++
	}
	return n, rows.Err()
}

func (i *Importer) importGroupsAndParticipants(ctx context.Context) (int, int, error) {
	rows, err := i.src.QueryContext(ctx, `
SELECT cs.Z_PK, cs.ZCONTACTJID, cs.ZPARTNERNAME, gi.ZOWNERJID, gi.ZCREATIONDATE
FROM ZWAGROUPINFO gi
JOIN ZWACHATSESSION cs ON cs.Z_PK = gi.ZCHATSESSION
WHERE cs.ZCONTACTJID IS NOT NULL`)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	type groupRow struct {
		chatPK  int64
		jid     string
		name    string
		owner   string
		created int64
	}
	var groups []groupRow
	for rows.Next() {
		var (
			pk      int64
			jid     string
			name    sql.NullString
			owner   sql.NullString
			created sql.NullFloat64
		)
		if err := rows.Scan(&pk, &jid, &name, &owner, &created); err != nil {
			i.log.Warn("scan group", "err", err)
			continue
		}
		groups = append(groups, groupRow{
			chatPK:  pk,
			jid:     jid,
			name:    nullStringValue(name),
			owner:   nullStringValue(owner),
			created: coreDataToUnix(nullFloat64Value(created)),
		})
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	updatedAt := nowUnix()
	groupsWritten := 0
	for _, g := range groups {
		if err := i.dst.UpsertGroup(ctx, db.Group{
			JID:       g.jid,
			Name:      g.name,
			OwnerJID:  g.owner,
			CreatedAt: g.created,
			UpdatedAt: updatedAt,
		}); err != nil {
			i.log.Warn("upsert group", "jid", g.jid, "err", err)
			continue
		}
		groupsWritten++
	}

	// Participants — skip if dst already has rows for this group
	// (spec idempotency: don't stomp live state).
	partsWritten := 0
	for _, g := range groups {
		existing, _ := i.dst.GetGroupParticipants(ctx, g.jid)
		if len(existing) > 0 {
			continue
		}
		pRows, err := i.src.QueryContext(ctx, `
SELECT ZMEMBERJID, ZISADMIN
FROM ZWAGROUPMEMBER
WHERE ZCHATSESSION = ? AND ZMEMBERJID IS NOT NULL AND ZMEMBERJID <> ''`, g.chatPK)
		if err != nil {
			i.log.Warn("query participants", "group", g.jid, "err", err)
			continue
		}
		var parts []db.GroupParticipant
		for pRows.Next() {
			var (
				jid     string
				isAdmin sql.NullInt64
			)
			if err := pRows.Scan(&jid, &isAdmin); err != nil {
				i.log.Warn("scan participant", "err", err)
				continue
			}
			parts = append(parts, db.GroupParticipant{
				GroupJID:   g.jid,
				ContactJID: jid,
				IsAdmin:    nullInt64Value(isAdmin) == 1,
			})
		}
		pRows.Close()
		if err := i.dst.SetGroupParticipants(ctx, g.jid, parts); err != nil {
			i.log.Warn("set participants", "group", g.jid, "err", err)
			continue
		}
		partsWritten += len(parts)
	}
	return groupsWritten, partsWritten, nil
}

func (i *Importer) importMessages(ctx context.Context) (int, int, int, error) {
	chatRows, err := i.src.QueryContext(ctx, `
SELECT Z_PK, ZCONTACTJID FROM ZWACHATSESSION WHERE ZCONTACTJID IS NOT NULL`)
	if err != nil {
		return 0, 0, 0, err
	}
	var chats []struct {
		pk  int64
		jid string
	}
	for chatRows.Next() {
		var pk int64
		var jid string
		if err := chatRows.Scan(&pk, &jid); err != nil {
			chatRows.Close()
			return 0, 0, 0, err
		}
		chats = append(chats, struct {
			pk  int64
			jid string
		}{pk, jid})
	}
	chatRows.Close()

	const pageSize = 5000
	written, skipped, errCount := 0, 0, 0
	for _, c := range chats {
		offset := 0
		for {
			rows, err := i.src.QueryContext(ctx, `
SELECT m.Z_PK, m.ZSTANZAID, m.ZFROMJID, m.ZTOJID, m.ZISFROMME, m.ZMESSAGEDATE,
       m.ZMESSAGETYPE, m.ZTEXT, m.ZPUSHNAME,
       parent.ZSTANZAID AS parent_stanza
FROM ZWAMESSAGE m
LEFT JOIN ZWAMESSAGE parent ON parent.Z_PK = m.ZPARENTMESSAGE
WHERE m.ZCHATSESSION = ?
ORDER BY m.Z_PK
LIMIT ? OFFSET ?`, c.pk, pageSize, offset)
			if err != nil {
				return written, skipped, errCount, err
			}
			pageCount := 0
			for rows.Next() {
				pageCount++
				var (
					pk        int64
					stanzaID  sql.NullString
					fromJID   sql.NullString
					toJID     sql.NullString
					isFromMe  sql.NullInt64
					msgDate   sql.NullFloat64
					msgType   sql.NullInt64
					text      sql.NullString
					pushName  sql.NullString
					parentSID sql.NullString
				)
				if err := rows.Scan(&pk, &stanzaID, &fromJID, &toJID, &isFromMe,
					&msgDate, &msgType, &text, &pushName, &parentSID); err != nil {
					errCount++
					i.log.Warn("scan message", "err", err)
					continue
				}
				if !stanzaID.Valid || stanzaID.String == "" {
					skipped++
					continue
				}
				fromMe := nullInt64Value(isFromMe) == 1
				var senderJID string
				if fromMe {
					// Outbound: we don't have the linked-device JID here,
					// so use the chat JID as a placeholder sender — matches
					// what the live ingester does in that case.
					senderJID = c.jid
				} else {
					senderJID = nullStringValue(fromJID)
					if senderJID == "" {
						senderJID = c.jid
					}
				}

				kind := kindForMessageType(int(nullInt64Value(msgType)))
				var textPtr *string
				if text.Valid && text.String != "" {
					s := text.String
					textPtr = &s
				}

				// Ensure sender contact exists.
				_ = i.dst.UpsertContact(ctx, db.Contact{
					JID:       senderJID,
					PushName:  nullStringValue(pushName),
					UpdatedAt: nowUnix(),
				})

				if err := i.dst.InsertMessage(ctx, db.Message{
					ID:        stanzaID.String,
					ChatJID:   c.jid,
					SenderJID: senderJID,
					FromMe:    fromMe,
					Timestamp: coreDataToUnix(nullFloat64Value(msgDate)),
					Kind:      kind,
					Text:      textPtr,
					QuotedID:  nullStringValue(parentSID),
				}); err != nil {
					errCount++
					i.log.Warn("insert message", "stanza", stanzaID.String, "err", err)
					continue
				}
				written++
			}
			rows.Close()
			if pageCount < pageSize {
				break
			}
			offset += pageSize
		}
	}
	return written, skipped, errCount, nil
}

func (i *Importer) importMedia(ctx context.Context) (int, error) {
	rows, err := i.src.QueryContext(ctx, `
SELECT cs.ZCONTACTJID, m.ZSTANZAID, md.ZFILESIZE, md.ZMOVIEDURATION, md.ZMEDIALOCALPATH
FROM ZWAMEDIAITEM md
JOIN ZWAMESSAGE m ON m.Z_PK = md.ZMESSAGE
JOIN ZWACHATSESSION cs ON cs.Z_PK = m.ZCHATSESSION
WHERE m.ZSTANZAID IS NOT NULL AND m.ZSTANZAID <> ''
  AND cs.ZCONTACTJID IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var (
			chatJID   string
			stanzaID  string
			size      sql.NullInt64
			duration  sql.NullInt64
			localPath sql.NullString
		)
		if err := rows.Scan(&chatJID, &stanzaID, &size, &duration, &localPath); err != nil {
			i.log.Warn("scan media", "err", err)
			continue
		}
		mime := mimeFromPath(nullStringValue(localPath))
		if err := i.dst.UpsertMedia(ctx, db.Media{
			MessageChatJID: chatJID,
			MessageID:      stanzaID,
			MimeType:       mime,
			Size:           nullInt64Value(size),
			DurationSec:    int(nullInt64Value(duration)),
			DownloadRef:    "", // see spec open question #1
		}); err != nil {
			i.log.Warn("upsert media", "stanza", stanzaID, "err", err)
			continue
		}
		n++
	}
	return n, rows.Err()
}

// mimeFromPath returns a best-effort MIME type from a file extension.
// Duplicated from internal/mcp/tools/send.go to avoid pulling a tools
// dep from macimport.
func mimeFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
