# wadb — WhatsApp MCP Server

**Status:** Approved design
**Date:** 2026-05-28
**Owner:** sausheong

## Summary

`wadb` is a single Go binary that links to WhatsApp as a Web "linked device" via [`whatsmeow`](https://github.com/tulir/whatsmeow), ingests messages/contacts/groups into a local SQLite database, and exposes that data plus send/react/download capabilities over a stdio MCP server. Intended for use by Claude (Code, Desktop, or any MCP client) for on-demand queries and actions against the user's own WhatsApp account.

Scope is **on-demand only**: tools fire only when called by an MCP client. No background rules, no agent loop, no event subscriptions exposed over MCP. The server *does* keep ingesting events in the background so SQLite stays current.

History before link day is out of scope for v1 (deferred to a phase 2 that mines the local Mac WhatsApp SQLite + Keychain key). From link day forward, the local archive grows continuously.

## Goals

- Link once via QR; thereafter the server runs headlessly.
- Query messages, contacts, and groups via MCP from Claude.
- Send text and media, react, mark read via MCP.
- Lazy-download media on demand and cache locally.
- Survive WhatsApp updates without code changes for known event types; degrade gracefully for unknown ones.
- Hermetic test suite — `go test ./...` passes without network or a linked device.

## Non-goals (v1)

- Mining historical messages from the local Mac WhatsApp encrypted SQLite (phase 2).
- Background automation, scheduled digests, auto-replies, agent loops.
- Group admin actions (kick, promote, rename, invite link management).
- Typing/presence indicators, status/stories, broadcast lists, starring/archiving.
- Multi-account support — one linked device per `WADB_HOME`.
- A GUI, web UI, or non-stdio MCP transport.

## Architecture

Single Go binary, two subcommands sharing the same codebase:

- **`wadb link`** — interactive. Prints a QR code in the terminal, user scans from WhatsApp → Linked Devices, session is persisted to disk. Exits when paired.
- **`wadb serve`** — runs the stdio MCP server. On startup: opens persisted session, connects to WhatsApp, starts ingesting events, exposes MCP tools.

Inside `serve`, three concurrent components in one process:

1. **WhatsApp client** — `whatsmeow.Client`, owns the connection and session, emits events.
2. **Event ingester** — single goroutine, drains `whatsmeow`'s event channel, writes to SQLite in a transaction per event.
3. **MCP server** — stdio JSON-RPC, exposes tools. Read tools query SQLite directly; send/download tools call into the WhatsApp client.

Read tools never touch the network — they're cheap and fast. Write/fetch tools share the live connection. Single process means no IPC and the WhatsApp socket stays warm.

### Filesystem layout

Everything lives under `WADB_HOME` (default `~/.wadb/`):

```
~/.wadb/
  session.db      # whatsmeow's own SQLite store (device keys, session state)
  wadb.db         # our application DB (messages, contacts, groups, media metadata)
  media/          # lazily downloaded media blobs, named by sha256
  logs/           # optional structured logs (when not writing to stderr)
```

## Data model

SQLite, WAL mode. JIDs (`...@s.whatsapp.net`, `...@g.us`) are used as primary keys — they're stable, globally unique, and avoid surrogate-key joins.

### Tables

**`contacts`**
- `jid` TEXT PRIMARY KEY
- `push_name` TEXT
- `business_name` TEXT
- `phone` TEXT
- `is_blocked` BOOLEAN NOT NULL DEFAULT 0
- `updated_at` INTEGER NOT NULL (unix seconds)

**`groups`**
- `jid` TEXT PRIMARY KEY
- `name` TEXT
- `topic` TEXT
- `owner_jid` TEXT REFERENCES contacts(jid)
- `created_at` INTEGER
- `updated_at` INTEGER NOT NULL

**`group_participants`**
- `group_jid` TEXT NOT NULL REFERENCES groups(jid) ON DELETE CASCADE
- `contact_jid` TEXT NOT NULL REFERENCES contacts(jid)
- `is_admin` BOOLEAN NOT NULL DEFAULT 0
- `joined_at` INTEGER
- PRIMARY KEY (`group_jid`, `contact_jid`)

**`chats`** — convenience table, derivable from messages but expensive to recompute.
- `jid` TEXT PRIMARY KEY
- `kind` TEXT NOT NULL CHECK (kind IN ('dm','group'))
- `last_message_at` INTEGER
- `unread_count` INTEGER NOT NULL DEFAULT 0
- `archived` BOOLEAN NOT NULL DEFAULT 0
- `pinned` BOOLEAN NOT NULL DEFAULT 0
- `muted_until` INTEGER

**`messages`** — the main table.
- `id` TEXT NOT NULL (WhatsApp message ID; unique per chat)
- `chat_jid` TEXT NOT NULL REFERENCES chats(jid)
- `sender_jid` TEXT NOT NULL REFERENCES contacts(jid)
- `from_me` BOOLEAN NOT NULL
- `timestamp` INTEGER NOT NULL
- `kind` TEXT NOT NULL CHECK (kind IN ('text','image','video','audio','voice','document','sticker','location','contact','system'))
- `text` TEXT — body or caption; NULL for pure media or system messages without text
- `quoted_id` TEXT — replies; nullable, references messages.id within the same chat
- `reactions` TEXT — JSON array: `[{"from_jid":"...","emoji":"👍","ts":1234567890}]`
- `edited_at` INTEGER — set when we receive an edit event
- `deleted_at` INTEGER — set when sender deletes; `text` is cleared on delete (tombstone)
- `raw` TEXT — JSON of the original whatsmeow event for future-proofing
- PRIMARY KEY (`chat_jid`, `id`)

Note: there is no `media_id` column. The relationship is owned by `media` via its FK to `messages`; one query joins them. Avoids the consistency burden of a bidirectional FK.

**`media`** — at most one row per message; FK is the linkage in both directions.
- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `message_chat_jid` TEXT NOT NULL
- `message_id` TEXT NOT NULL
- UNIQUE (`message_chat_jid`, `message_id`)
- `mime_type` TEXT NOT NULL
- `size` INTEGER
- `sha256` TEXT
- `width` INTEGER
- `height` INTEGER
- `duration_sec` INTEGER
- `download_ref` TEXT NOT NULL — encoded whatsmeow reference (URL + decryption keys) for later fetch
- `local_path` TEXT — NULL until downloaded
- `downloaded_at` INTEGER
- FOREIGN KEY (`message_chat_jid`, `message_id`) REFERENCES messages(chat_jid, id)

**`fts_messages`** — FTS5 virtual table over `messages.text`. Kept in sync via AFTER INSERT/UPDATE/DELETE triggers on `messages`.

### Indices

- `messages(chat_jid, timestamp DESC)` — for chat history pagination
- `messages(sender_jid, timestamp DESC)` — for "messages from person X"
- `messages(timestamp DESC)` — for global recency
- `chats(last_message_at DESC)` — for `list_chats`
- `contacts(push_name)`, `groups(name)` — for substring search

### Migrations

Versioned SQL files under `internal/db/migrations/`, applied in order on startup. Each file is `NNN_description.up.sql`. Schema version tracked in a `schema_version` table. No down migrations — forward-only; if a migration is wrong, write a corrective one.

## MCP tools

13 tools total. All inputs validated; all return JSON.

### Discovery (read SQLite)

- **`list_chats(kind?, limit?, cursor?)`** — recent chats, newest activity first. `kind` filters `dm`/`group`. Returns `{chats: [{jid, name, kind, last_message_preview, last_message_at, unread_count}], next_cursor}`.
- **`list_contacts(query?, limit?, cursor?)`** — substring search over `push_name`/`business_name`/`phone`.
- **`list_groups(query?, limit?, cursor?)`** — groups you're in, optional name substring filter.
- **`get_chat(jid)`** — full chat detail. For groups: participants with admin flags.

### Messages (read SQLite)

- **`get_messages(chat_jid, before?, after?, limit?)`** — page through a conversation. `before`/`after` are message IDs or timestamps. Default newest 50.
- **`search_messages(query, chat_jid?, sender_jid?, since?, until?, limit?)`** — FTS5 over text and caption. All filters optional and composable.
- **`get_message(chat_jid, id)`** — one message with reactions expanded, quoted message expanded one level, media metadata included.

### Media (lazy via whatsmeow)

- **`download_media(chat_jid, message_id)`** — fetches the encrypted blob via the live connection, decrypts, writes to `~/.wadb/media/<sha256>.<ext>`, updates the `media` row, returns `{local_path, mime_type, size}`. Idempotent: returns cached path if already present.

### Send (via whatsmeow)

- **`send_text(chat_jid, text, reply_to_id?)`** — send a text message, optionally as a reply to a prior message.
- **`send_media(chat_jid, local_path, caption?, reply_to_id?)`** — send image/video/document/voice. Kind inferred from the file's MIME type. Returns the new message's ID.
- **`react(chat_jid, message_id, emoji)`** — add a reaction. Empty `emoji` removes the reaction.
- **`mark_read(chat_jid, up_to_message_id?)`** — mark chat read up to a point; default latest.

### Status

- **`status()`** — `{connected, linked_device, last_event_at, db: {messages, contacts, groups, oldest_message_at, newest_message_at}}`. Cheap health check.

### Design rationale

- **`send_media` takes a local path, not bytes.** MCP can carry bytes via base64, but megabytes through JSON is painful. Path matches how Claude already references files.
- **No subscription/webhook tool.** On-demand mode means the MCP doesn't push events to the client. The server still ingests in the background so reads are current.
- **Explicitly deferred:** initiating edits/deletes of your own messages (we *record* others' edits/deletes), group admin actions, typing/presence. Easy adds once the v1 surface settles.

## Event handling

`whatsmeow` delivers events on a Go channel. A single goroutine drains it; one transaction per event. Order matters (a reaction references a prior message), so no parallelism.

| Event | Action |
|-------|--------|
| `*events.Message` | Upsert `messages` row + `media` row if attached. For replies, set `quoted_id`. |
| Edit events | Update the existing row's `text` and set `edited_at`. |
| Delete events | Clear `text`, set `deleted_at` (tombstone, do not delete the row). |
| `*events.Receipt` | Update `chats.unread_count` / read state. Per-chat, not per-message. |
| `*events.Reaction` | Append to or remove from the target message's `reactions` JSON. |
| `*events.Contact` / `*events.PushName` | Upsert `contacts`. |
| `*events.GroupInfo` / participant changes | Update `groups` and `group_participants`. |
| `*events.Connected` / `*events.Disconnected` | Log; update an in-memory connection state read by `status()`. |
| `*events.LoggedOut` | Log clear error, exit non-zero. User must re-run `wadb link`. |
| Unknown | Log at debug; no DB write. |

**Duplicates.** All message inserts use `INSERT OR IGNORE` on `(chat_jid, id)`. Catch-up syncs after a reconnect re-deliver messages we may already have.

**Catch-up.** When `serve` reconnects after a gap, `whatsmeow` requests recent history. Those arrive as normal `*events.Message` events through the same ingester.

## Error model

MCP tool errors fall into three categories:

- **Validation** (bad JID, missing required arg, malformed input) → MCP error response, no retry value.
- **Not found** (`get_message` for an unknown ID) → empty/null result, *not* an error. Cheap existence check for Claude.
- **WhatsApp errors** (send failed, not connected, rate-limited, media decryption failed) → MCP error response with the underlying reason and a `retryable` boolean.

## Lifecycle

- **Startup:** open `session.db` and `wadb.db`, run pending migrations, connect via `whatsmeow`, start ingester goroutine, start MCP server on stdio.
- **Reconnect:** handled internally by `whatsmeow` with backoff. Not our concern.
- **Logout** (`*events.LoggedOut`): log, exit 1. User re-links.
- **Shutdown** (SIGINT/SIGTERM): stop accepting MCP requests → drain in-flight DB writes → call `whatsmeow.Client.Disconnect()` for a clean socket close → exit 0. Clean disconnect matters; repeated abrupt drops can flag the linked device.

## Logging

Structured logs (Go `log/slog`) to **stderr only**. Stdout is reserved for MCP JSON-RPC — any stray write breaks the protocol. This is the single most common implementation footgun for stdio MCP servers; called out here so it doesn't get lost.

Log levels via `WADB_LOG_LEVEL` (`debug`|`info`|`warn`|`error`, default `info`).

## Concurrency

- SQLite in WAL mode.
- Ingester is the **sole writer** to `wadb.db`.
- MCP read tool handlers read freely (WAL allows concurrent readers).
- Send/download tools are network-bound and serialized naturally by `whatsmeow`'s internal locking.

## Testing strategy

**Heavily tested (unit, hermetic):**
- Event ingestion — table-driven tests per event type, real in-memory SQLite, no mocks.
- MCP tool handlers — pre-seeded SQLite fixtures; verify validation, query correctness, pagination, error mapping.
- Schema migrations — apply in order against fresh DB, assert resulting schema.

**Lightly tested (integration):**
- Send tools — a small Go interface around the `whatsmeow` methods we use (`SendMessage`, `Download`, `MarkRead`, `SendReaction`); a fake implementation in tests verifies we call it with the right args. We don't test `whatsmeow` itself.
- MCP plumbing — one end-to-end test: spin up the server, connect a stdio MCP client, call `list_chats`, assert response shape.

**Not tested automatically:**
- Live WhatsApp. No `live`-tagged tests. Manual smoke test on real WhatsApp after major changes, documented in README.
- `whatsmeow` internals. It's a dependency.

**Conventions:**
- `go test ./...` runs everything hermetically — no network, no linked device.
- In-memory SQLite (`:memory:`) for unit tests; tempdir DBs for integration tests.
- Fixtures as Go structs, not JSON files.

**Manual verification checklist (README):** link device, send/receive text in DM and group, send/receive image, react, reply, observe an incoming edit and delete, restart server and confirm catch-up.

## Project layout

```
wadb/
  cmd/wadb/                  # main; subcommands link, serve
  internal/
    config/                  # WADB_HOME resolution, env vars
    db/                      # sqlite open, migrations/, queries
      migrations/
    waclient/                # whatsmeow wrapper + interface for testing
    ingest/                  # event → DB writer
    mcp/                     # tool handlers, MCP server wiring
      tools/                 # one file per tool group (discovery, messages, media, send)
    media/                   # download + cache logic
  docs/superpowers/specs/    # this design
  README.md
  go.mod
```

## Open questions / future work

- **Phase 2: historical mining** of the local Mac WhatsApp encrypted SQLite via Keychain key extraction. Tracked separately.
- **Pagination cursor format.** Opaque base64 of `(timestamp, id)` is the obvious pick; finalize during implementation.
- **MCP library.** Decide between `mark3labs/mcp-go` and rolling stdio JSON-RPC by hand. Spec-level: either works; pick the one with the cleanest fit during plan writing.
