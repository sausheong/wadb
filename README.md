# wadb

`wadb` is a [Model Context Protocol](https://modelcontextprotocol.io) server that gives an LLM like Claude on-demand access to your WhatsApp account. It links once as a Web "linked device" via [whatsmeow](https://github.com/tulir/whatsmeow), mirrors messages, contacts, and groups into a local SQLite database as they arrive, and exposes that data — plus send / react / download capabilities — through 13 stdio MCP tools.

Single Go binary. No background server. No cloud. Your data stays on disk under `~/.wadb/`.

See [`docs/superpowers/specs/2026-05-28-wadb-design.md`](docs/superpowers/specs/2026-05-28-wadb-design.md) for the full design and the rationale behind the boundaries.

## Status

v1. Hermetic test suite (42 tests) passes. The link / send / receive / search / download paths work against real WhatsApp. See [Known limitations](#known-limitations) for what's deferred.

## Install

Requires Go 1.25+.

```bash
git clone https://github.com/sausheong/wadb.git
cd wadb
go build -o wadb ./cmd/wadb
```

The binary is fully self-contained — `modernc.org/sqlite` is pure Go, no CGO toolchain required.

## Quick start

**1. Link your WhatsApp account.** Open WhatsApp on your phone → Settings → Linked Devices → Link a device, then run:

```bash
./wadb link
```

A QR code prints on stderr. Scan it. The command exits as soon as pairing completes. The session is persisted under `~/.wadb/session.db` — you only do this once.

**2. Wire it into your MCP client.** For Claude Desktop, add this to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS):

```json
{
  "mcpServers": {
    "wadb": {
      "command": "/absolute/path/to/wadb",
      "args": ["serve"]
    }
  }
}
```

For Claude Code:

```bash
claude mcp add wadb /absolute/path/to/wadb serve
```

Restart your MCP client. Claude can now see your WhatsApp.

**3. Try it.** Ask Claude something like *"What are my last 10 WhatsApp chats?"* or *"Search my messages for anything mentioning 'flight booking' this month."*

## Import history (macOS only)

`wadb serve` only sees messages that arrive *while it's running*. To backfill from the WhatsApp Desktop app you already have on this Mac:

```bash
./wadb import-history
```

Reads the desktop app's local SQLite (`~/Library/Group Containers/group.net.whatsapp.WhatsApp.shared/ChatStorage.sqlite`, plaintext — no decryption needed) and copies messages, contacts, groups, and media metadata into `~/.wadb/wadb.db`. Idempotent — re-running adds new rows without duplicating existing ones.

Flags:

| Flag | Purpose |
|---|---|
| `--source PATH` | Override the default ChatStorage.sqlite path. |
| `--dry-run` | Verify the source is readable and report its size; don't write. |

Caveats:

- **macOS WhatsApp Desktop only.** Linux/Windows desktop apps use different storage; not supported.
- **Media metadata is imported, but the binary blobs are not.** Historical attachments appear in `get_message` results with size/MIME/dimensions, but `download_media` on a historical row currently can't fetch the bytes (whatsmeow needs a download reference we don't have for imported rows). Tracked as a follow-up.
- The desktop app may be running concurrently. SQLite WAL handles concurrent readers; worst case a few in-flight messages aren't picked up — re-run to catch them.
- Outbound messages (you sent) record the chat JID as the sender placeholder, not your own JID. Filter `from_me = true` if you want to find what you sent.

## Tools

`wadb serve` exposes 13 tools over stdio JSON-RPC. Read-side tools query the local SQLite directly and never touch the network; write-side tools call into the live WhatsApp connection.

### Discovery

| Tool | Purpose |
|---|---|
| `status` | Connection state, linked device, ingestion freshness, DB row counts. |
| `list_chats` | Recent chats, newest activity first. Optional `kind` filter (`dm` \| `group`). Paginated. |
| `list_contacts` | Substring search across push name, business name, phone. |
| `list_groups` | Substring search over groups you're in. |
| `get_chat` | Full chat detail. For groups, includes participants and admin flags. |

### Messages

| Tool | Purpose |
|---|---|
| `get_messages` | Page through a chat, newest first. Cursor-based pagination. |
| `search_messages` | FTS5 full-text search across all message text and media captions. Optional `chat_jid`, `sender_jid`, `since`, `until` filters. |
| `get_message` | One message with reactions, quoted message (one level), and media metadata expanded. |

### Media

| Tool | Purpose |
|---|---|
| `download_media` | Fetch a media blob via the live connection, decrypt, cache at `~/.wadb/media/<sha256>.<ext>`. Idempotent — second call returns the cached path without re-downloading. |

### Send

| Tool | Purpose |
|---|---|
| `send_text` | Send a text message. Supports replies via `reply_to_id`. |
| `send_media` | Send a file from disk. Kind (image / video / audio / document) inferred from MIME. |
| `react` | Add or remove a reaction. Empty `emoji` removes. |
| `mark_read` | Mark a chat read up to a message ID (or latest if omitted). |

### Error envelope

Tools that call WhatsApp return errors as `{"error": "...", "retryable": bool}` so the LLM can decide whether to retry. Network / rate-limit / not-connected errors are flagged retryable; validation errors aren't.

## Environment

| Variable | Default | Meaning |
|---|---|---|
| `WADB_HOME` | `~/.wadb` | Data directory: session DB, application DB, media cache. Created at `0700`. |
| `WADB_LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error`. Logs go to **stderr** only. |

The home directory contains:

```
~/.wadb/
  session.db    # whatsmeow's session and device keys
  wadb.db       # application DB: messages, contacts, groups, media metadata (WAL)
  media/        # downloaded media blobs, sha256-named
```

## Architecture

Single Go process. Three concurrent components share the address space:

1. **WhatsApp client** (`whatsmeow.Client`) — owns the socket and session, emits events.
2. **Event ingester** — single goroutine, drains the events channel, writes to SQLite. **Sole writer to `wadb.db`** (except for `download_media`'s targeted UPDATE on its own media row).
3. **MCP server** (`mark3labs/mcp-go`) — stdio JSON-RPC, exposes the 13 tools. Read tools query SQLite directly (WAL allows many concurrent readers); send / download tools call into the WhatsApp client.

```
                      +-----------------+
                      |  WhatsApp Web   |
                      +--------+--------+
                               |
                               | whatsmeow socket
                               v
   +------+   stdio MCP   +----+-----+         +----------+
   | LLM  |<------------->|  wadb    | events  | SQLite   |
   +------+   JSON-RPC    |  serve   |-------->| wadb.db  |
                          +----+-----+ ingest  +----------+
                               |                     ^
                               | read tools          |
                               +---------------------+
```

**Why this shape:**

- Single process means no IPC and the WhatsApp socket stays warm.
- The ingester being the sole writer lets the MCP server read with no coordination — WAL mode handles the rest.
- Tool handlers never touch the network (with one exception): cheap, fast, and Claude can ask for a chat's history without you paying a round trip.
- Stdout is reserved for MCP JSON-RPC. Logs go to stderr. The whatsmeow library's logger is routed through `log/slog` via an adapter for the same reason — one stray write breaks the protocol.

### Repository layout

```
cmd/wadb/                   # main; subcommands link, serve
internal/
  config/                   # WADB_HOME resolution, paths, log level
  db/                       # SQLite open + WAL, embedded migrations, typed Queries
    migrations/             # forward-only versioned SQL
  waclient/                 # WhatsApp Client interface, whatsmeow wrapper, test Fake
  waevent/                  # normalized event types (shared by ingest and waclient)
  ingest/                   # event -> DB writer (single goroutine)
  mcp/                      # MCP server wiring + e2e test
    cursor/                 # opaque (ts, id) pagination cursors
    tools/                  # one file per tool group: status / discovery / messages / send / media
  media/                    # sha256-keyed download cache
docs/superpowers/
  specs/                    # design spec
  plans/                    # implementation plan
```

## Known limitations

These are tracked and intentionally deferred from v1, not bugs:

- **No unread-count tracking.** `*events.Receipt` arrives but isn't translated. `list_chats` always reports `unread_count: 0`.
- **`mark_read` in groups** passes self as the sender argument, which is correct for DMs but not ideal for group blue-tick semantics. Fix needs the original sender JID looked up from the messages table.
- **No historical mining.** Messages before link day aren't in the DB. A future phase 2 will extract them from the local Mac WhatsApp encrypted SQLite via Keychain.
- **No background automation.** Tools fire on-demand only. There's no agent loop, scheduled digest, or auto-reply.
- **No group admin actions** (kick, promote, rename, invite link management).
- **No typing / presence / status / broadcast lists / starring / archiving** as tools (some of these arrive as events and are stored when relevant).
- **Single account per `WADB_HOME`.** Run a second `wadb` with `WADB_HOME=~/.wadb-alt` if you need a second account.

## Manual verification

The hermetic test suite (`go test ./...`) covers everything except live WhatsApp. Run this checklist against a real account after non-trivial changes:

- [ ] `wadb link` — pair from a clean `WADB_HOME`
- [ ] `wadb serve` — connects without errors
- [ ] Send a text in a DM → `get_messages` shows it
- [ ] Receive a text in a DM → ingester writes the row
- [ ] Send an image with caption → message appears in WhatsApp
- [ ] Receive an image → `download_media` fetches, decrypts, and caches
- [ ] React to a message → `get_message` shows the reaction
- [ ] Reply to a message → `get_message` shows the quoted message expanded
- [ ] Observe an incoming edit → row's `text` updates, `edited_at` set
- [ ] Observe an incoming delete → row's `text` is `NULL`, `deleted_at` set
- [ ] Restart `wadb serve` → catch-up brings in messages received while down

## Testing

```bash
go test ./...      # hermetic; no network, no linked device
go vet ./...
go build ./...
```

The test suite uses an in-memory `waclient.Fake` to drive the ingester and MCP handlers without touching whatsmeow. The MCP e2e test (`internal/mcp/e2e_test.go`) runs the real server in-process and drives it through the actual JSON-RPC transport.

## Security

- Session credentials and the full message archive live under `WADB_HOME` (default `~/.wadb/`). The directory is created at `0700`; files at `0600`.
- The same advice as for the WhatsApp Web app applies: anyone with read access to that directory can impersonate your linked device.
- `wadb` only writes to stdout to speak MCP — there's no telemetry, no remote logging, no automatic update check.

## License

MIT.
