# wadb

`wadb` is a [Model Context Protocol](https://modelcontextprotocol.io) server that gives AI assistant or agent (like Claude Desktop or OpenClaw) on-demand access to your WhatsApp account. It links once as a Web "linked device" via [whatsmeow](https://github.com/tulir/whatsmeow), mirrors messages, contacts, and groups into a local SQLite database as they arrive, and exposes that data — plus send / react / download capabilities — through 13 stdio MCP tools.

Single Go binary. No background server. No cloud. Your data stays on disk under `~/.wadb/`.


## Install

### Pre-built binary (recommended)

Grab a static binary for your platform from the [latest release](https://github.com/sausheong/wadb/releases/latest):

| Platform | File |
|---|---|
| macOS, Apple Silicon | `wadb-<version>-darwin-arm64` |
| macOS, Intel | `wadb-<version>-darwin-amd64` |
| Linux, x86_64 | `wadb-<version>-linux-amd64` |
| Linux, ARM64 | `wadb-<version>-linux-arm64` |
| Windows, x86_64 | `wadb-<version>-windows-amd64.exe` |
| Windows, ARM64 | `wadb-<version>-windows-arm64.exe` |

Each binary is ~18 MB, fully static (`CGO_ENABLED=0`), with no runtime dependencies. `SHA256SUMS` is published with every release — verify before running:

```bash
shasum -a 256 -c SHA256SUMS   # macOS / Linux
# or, on Windows:
Get-FileHash wadb-<version>-windows-amd64.exe -Algorithm SHA256
```

Install (macOS / Linux):

```bash
chmod +x wadb-<version>-darwin-arm64
# macOS only: clear the Gatekeeper quarantine flag if the binary was downloaded via a browser
xattr -d com.apple.quarantine wadb-<version>-darwin-arm64 2>/dev/null || true
sudo mv wadb-<version>-darwin-arm64 /usr/local/bin/wadb
wadb link
```

On Windows, drop the `.exe` somewhere on `PATH` and call it from PowerShell or cmd.

### Build from source

Requires Go 1.25+.

```bash
git clone https://github.com/sausheong/wadb.git
cd wadb
go build -o wadb ./cmd/wadb
```

The binary is fully self-contained — `modernc.org/sqlite` is pure Go, no CGO toolchain required. `make build` works too.

## Quick start

**1. Link your WhatsApp account.** Open WhatsApp on your phone → Settings → Linked Devices → Link a device, then run:

```bash
./wadb link
```

A QR code prints on your screen. Scan it. The command exits as soon as pairing completes. The session is persisted under `~/.wadb/session.db` — you only do this once.

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

For [OpenClaw](https://openclaw.ai) (register `wadb` in the Gateway's MCP registry so any OpenClaw runtime can consume it):

```bash
openclaw mcp set wadb '{"command":"/absolute/path/to/wadb","args":["serve"]}'
```

Verify with `openclaw mcp list` (should show `wadb`) and `openclaw mcp show wadb --json`. To remove later: `openclaw mcp unset wadb`.

For [NanoClaw v2](https://github.com/qwibitai/nanoclaw), add `wadb` to a group's `container.json` and mount `~/.wadb` so the container can see your session and database:

```jsonc
{
  "mcpServers": {
    "wadb": {
      "command": "/workspace/extra/wadb/wadb",
      "args": ["serve"],
      "env": {
        "WADB_HOME": "/workspace/extra/.wadb"
      }
    }
  },
  "additionalMounts": [
    {
      "hostPath": "/Users/<you>/.wadb",
      "containerPath": ".wadb",
      "readonly": false
    },
    {
      "hostPath": "/absolute/path/to/wadb-binary-dir",
      "containerPath": "wadb",
      "readonly": true
    }
  ]
}
```

Substitute `<you>` with `$HOME`'s basename. The container mounts `~/.wadb` read-write (the binary needs to write `wadb.db` and `media/`) and the binary directory read-only. Confirm both paths are under an `allowedRoots` entry in `~/.config/nanoclaw/mount-allowlist.json` first — run `/manage-mounts` if not. After editing, rebuild with `pnpm run build` and restart NanoClaw. The agent will see tools as `mcp__wadb__*`; add `'mcp__wadb__*'` to `TOOL_ALLOWLIST` in `container/agent-runner/src/providers/claude.ts` if your provider config gates them.

Restart your MCP client. Claude Desktop/Claude Code/OpenClaw/NanoClaw v2 can now see your WhatsApp.

**3. Try it.** Ask Claude Desktop/Claude Code/OpenClaw/NanoClaw v2 something like *"What are my last 10 WhatsApp chats?"* or *"Search my messages for anything mentioning 'flight booking' this month."*

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
| `list_chats` | Recent chats, newest activity first. Optional `kind` filter (`dm` \| `group`). Paginated. Each chat carries a `name` field (group name for groups, contact push name for DMs). |
| `list_contacts` | Substring search across push name, business name, phone. |
| `list_groups` | Substring search over groups you're in. |
| `get_chat` | Full chat detail. For groups, includes participants with their resolved `name` and admin flags. |

### Messages

| Tool | Purpose |
|---|---|
| `get_messages` | Page through a chat, newest first. Cursor-based pagination. Each message carries `sender_jid` and a resolved `sender_name` (`"You"` when `from_me` is true). |
| `search_messages` | FTS5 full-text search across all message text and media captions. Optional `chat_jid`, `sender_jid`, `since`, `until` filters. Returns the same `sender_name`-enriched rows. |
| `get_message` | One message with reactions, quoted message (one level), and media metadata expanded. |

`sender_name` is resolved best-effort from the contacts table. WhatsApp's `@lid` (Linked Identity) JIDs come in two shapes — `XYZ@lid` (base) and `XYZ:N@lid` (per-device, used by senders) — and the resolver strips the `:N` device suffix before lookup so device-form senders match the base-form profile names stored by `import-history`. A sender with no profile name in either form returns `sender_name: ""`.

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

## Design

### Process model

`wadb` is one OS process holding three cooperating components in the same address space, and one SQLite file they all coordinate around.

```
                       +---------------------+
                       |   WhatsApp Web      |
                       +----------+----------+
                                  |
                                  | whatsmeow socket (TLS)
                                  v
   +-------+   stdio JSON-RPC  +-----------------------------+
   |  LLM  |<----------------->|  wadb serve  (one process)  |
   +-------+    MCP protocol   |                             |
                               |  +-----------------------+  |
                               |  |  waclient.Client      |  |
                               |  |  (whatsmeow wrapper)  |  |
                               |  +----------+------------+  |
                               |             | events ch     |
                               |             v               |
                               |  +-----------------------+  |
                               |  |  ingest.Ingester      |  |
                               |  |  (single goroutine,   |  |
                               |  |   only writer)        |  |
                               |  +----------+------------+  |
                               |             | writes        |
                               |             v               |
                               |  +-----------------------+  |
                               |  |  db.Queries (SQLite,  |  |
                               |  |  WAL, FTS5)           |  |
                               |  +-----------------------+  |
                               |             ^               |
                               |             | read tools    |
                               |  +----------+------------+  |
                               |  |  mcp/tools (13 tools) |  |
                               |  +-----------------------+  |
                               +-----------------------------+
```

The three components:

1. **`waclient.Client`** wraps `whatsmeow.Client`. Owns the WhatsApp Web socket, holds session keys in `session.db`, exposes a normalized `Events(ctx) <-chan waevent.Event` channel and a send/download API. There's also a `waclient.Fake` for hermetic tests that drives synthetic events without going through whatsmeow.
2. **`ingest.Ingester`** runs one goroutine that drains the events channel and translates events into SQL writes. It is the **sole writer to `wadb.db`** — the `download_media` tool is the only other writer, and only ever updates its own media row. This means no in-process write coordination: the ingester doesn't even take a lock around its work, and read-side tool handlers don't compete for write locks.
3. **`mcp.Server`** is the stdio MCP transport (`mark3labs/mcp-go`). It registers the 13 tools and serves JSON-RPC on stdin/stdout. Read-side tools open `q.db.QueryContext` directly — no fan-out into the WhatsApp client. Send-side and `download_media` call back into `waclient.Client`.

A separate read-only path:

- **`macimport.Importer`** is what `wadb import-history` runs. It reads the macOS WhatsApp Desktop SQLite (`ChatStorage.sqlite`) with `mode=ro`, translates the Core Data schema (`Z_`-prefixed tables, Core-Data-epoch timestamps) into `wadb.db`'s schema, and exits. Independent of `serve`; not part of the long-running process.

### Why this shape

- **Single process, no IPC.** The WhatsApp socket stays warm and the LLM sees no startup latency on each tool call. Restarting the binary is the only way the socket goes down; whatsmeow handles catch-up automatically on reconnect.
- **One writer + WAL mode = no in-process locking.** SQLite's WAL allows many concurrent readers while a single writer is active. Because only the ingester writes the bulk tables, read tools never block and never see partial state.
- **Read-side tools never touch the network.** A tool call for "list my 50 most recent chats" is a single indexed SQLite query — sub-millisecond, with no WhatsApp round trip. This matters because LLM agents will fire dozens of these in a planning loop.
- **Stdout is sacred.** Stdout is reserved for MCP JSON-RPC frames. All logging — including the whatsmeow library's own logs — goes to stderr via a `log/slog` adapter (`internal/waclient/slog_log.go`). One stray `fmt.Println` to stdout corrupts the protocol and the MCP client disconnects.

### Data model

`wadb.db` is a single SQLite file with WAL journaling. Two migrations bring it up:

- `001_initial.up.sql` — six application tables.
- `002_fts.up.sql` — an FTS5 virtual table over `messages.text` plus AFTER INSERT/UPDATE/DELETE triggers that keep it in sync.

The six application tables:

| Table | Purpose | Primary key |
|---|---|---|
| `contacts` | One row per JID we've ever seen. `push_name`, `business_name`, `phone`. | `jid` |
| `groups` | Group metadata (name, topic, owner, created_at). | `jid` |
| `group_participants` | Membership + admin flag. Composite PK (group_jid, contact_jid). | `(group_jid, contact_jid)` |
| `chats` | One row per conversation (DM or group). `last_message_at`, `unread_count`, `archived`, `pinned`, `muted_until`. | `jid` |
| `messages` | The bulk table. Composite PK; nullable `text`; `kind` constrained to one of 10 values; `reactions` is a JSON blob; `raw` holds the original whatsmeow event JSON for forensics. | `(chat_jid, id)` |
| `media` | Per-message media metadata (mime, size, sha256, dimensions, duration, download_ref, cached `local_path`). | autoincrement `id`, unique `(message_chat_jid, message_id)` |

Indexes that actually matter at query time:

- `messages_chat_ts_idx (chat_jid, timestamp DESC)` — drives `get_messages` paging (newest-first by chat).
- `messages_sender_ts_idx (sender_jid, timestamp DESC)` — drives `search_messages` when filtered by sender.
- `messages_ts_idx (timestamp DESC)` — global recency.
- `chats_last_msg_idx (last_message_at DESC)` — drives `list_chats`.
- FTS5 `fts_messages` table — drives `search_messages` full-text path.

Timestamps are Unix seconds (`int64`). `from_me` is `0|1`. Nullable `text` distinguishes deleted/tombstoned messages from empty ones (deletion sets `text=NULL` and `deleted_at=<unix>`).

### Ingestion pipeline

`waclient.WhatsmeowClient` translates `whatsmeow.events.*` into a small set of normalized event types in `internal/waevent` — `TestMessage`, `TestMedia`, `TestEdit`, `TestDelete`, `TestReaction`, `TestContact`, `TestGroupInfo`, `TestGroupParticipant`. The "Test" prefix is historical (the types started as test fixtures and were lifted into a shared package to break the `ingest ↔ waclient` import cycle); they are the production event shape.

`ingest.Ingester.Run` is a `select { case <-ctx.Done(): … case ev := <-ch: i.handle(ctx, ev) }` loop. `handle` dispatches by concrete type and calls into `db.Queries`:

| Event | Effect on DB |
|---|---|
| `TestMessage` | `UpsertChat` (kind from JID suffix), `UpsertContact` for sender, `InsertMessage` (INSERT OR IGNORE — live ingester wins on duplicate). |
| `TestMedia` | `UpsertMedia` keyed by `(chat_jid, msg_id)`. |
| `TestEdit` | `UpdateMessageEdited` — sets new text and `edited_at`. |
| `TestDelete` | `TombstoneMessage` — sets `text=NULL` and `deleted_at`. |
| `TestReaction` | Reads the existing reactions JSON, mutates the array, writes back via `UpdateMessageReactions`. |
| `TestContact` | `UpsertContact` (newer `updated_at` wins). |
| `TestGroupInfo` | `UpsertGroup`. |
| `TestGroupParticipant` | `SetGroupParticipants` (full replace in a tx). |

After every event, `markEvent()` stamps `lastEventAt` — the `status` tool reads this to report "ingestion freshness."

### Send and download paths

The four send tools (`send_text`, `send_media`, `react`, `mark_read`) live in `internal/mcp/tools/send.go`. They:

1. Validate arguments (`validateRequired`).
2. Call into `waclient.Client.SendX(...)`.
3. Translate the result into the standard `{result, error, retryable}` envelope so the LLM can decide whether to retry.

`download_media` is in `internal/mcp/tools/media.go`. It:

1. Looks up the `media` row by `(chat_jid, message_id)`.
2. If `local_path` already exists and the file is present, returns the cached path — never re-downloads.
3. Otherwise calls `waclient.Client.DownloadMedia(ref)`, computes sha256, writes the bytes to `~/.wadb/media/<sha256>.<ext>`, and updates the media row's `local_path`, `size`, `sha256`, `downloaded_at`.

Errors flagged retryable: `not connected`, `rate limited`, `i/o timeout`, network errors. Validation and "no such message" errors are non-retryable.

### Linking flow (`wadb link`)

The pairing handshake is two phases and `wadb link` waits for both. Exiting between them silently drops the pairing — WhatsApp's "Linked Devices" list never shows the device.

```
  Phone scans QR
        │
        ▼
  qrCh emits "code" (display)  ──► repeat until scanned
        │
        ▼
  qrCh emits "success"  ◄──── QR accepted, local device row written
        │
        ▼
  *events.PairSuccess  ◄────── server confirms identity exchange
        │
        ▼
  reconnect + authenticate
        │
        ▼
  *events.Connected   ◄─────── new authenticated socket up
        │
        ▼
  hold socket open 5s  ◄────── lets initial app-state sync (contacts,
        │                       chat list) complete before we close
        ▼
  Disconnect, exit 0
```

Two practical consequences baked into `cmd/wadb/link.go`:

- The event handler is registered *before* `GetQRChannel` opens — otherwise `PairSuccess` can race ahead of the listener.
- `session.db` is opened with `journal_mode=WAL` + `busy_timeout=5000` in `internal/waclient/whatsmeow.go`. During post-pair sync, whatsmeow runs prekey upload, identity-store writes, and history-sync writes in parallel goroutines. Without WAL they serialize and the contention surfaces as `SQLITE_BUSY`, silently failing the handshake.

### Importing history (`wadb import-history`)

The importer is independent of `serve`. Pipeline (in `internal/macimport/macimport.go`):

1. **Open source read-only.** `file:<path>?mode=ro`. Deliberately NOT `immutable=1` — that flag tells SQLite to skip WAL coordination, which causes `database disk image is malformed` when WhatsApp Desktop is running and writing concurrently.
2. **Pin destination pool to 1 connection.** `db.Queries`' methods all use `q.db.ExecContext`; without `SetMaxOpenConns(1)`, the explicit `BEGIN` lands on connection A while writers fire on B/C/D and the transaction does nothing. The fix is `SetMaxOpenConns(1)` for the duration of the import.
3. **Five phases in dependency order**, each wrapped in a single `BEGIN`/`COMMIT` so 100K+ inserts pay one fsync instead of one per row:

   | Phase | Source tables | Notes |
   |---|---|---|
   | Contacts | `ZWAMESSAGE.ZFROMJID`, `ZWACHATSESSION.ZCONTACTJID`, `ZWAGROUPMEMBER.ZMEMBERJID` (UNION DISTINCT) | Push names pre-built into a JID→name map in two scans: `ZWAPROFILEPUSHNAME` (primary) and `ZWACHATSESSION.ZPARTNERNAME` (fallback). `ZWAMESSAGE.ZPUSHNAME` is **not** used — on this schema version it holds serialized protobuf identity hints, not human names. |
   | Chats | `ZWACHATSESSION` | `ZSESSIONTYPE = 1` → group, else DM. |
   | Groups + participants | `ZWAGROUPINFO` joined to `ZWACHATSESSION`, then per-group `ZWAGROUPMEMBER` | This phase runs **without** an outer transaction because `db.Queries.SetGroupParticipants` opens its own delete-then-insert tx and SQLite rejects nested `BEGIN`s. |
   | Messages | Per-chat paged `SELECT … FROM ZWAMESSAGE LIMIT 5000 OFFSET N` | Uses `InsertMessageFillNulls` (not `InsertMessage`) so a prior failed import's `NULL text` rows get back-filled via `COALESCE` instead of being silently skipped by `INSERT OR IGNORE`. Per-row `UpsertContact` writes the sender with `updated_at=1` so it loses any "newer wins" race against the contacts-phase upserts. |
   | Media | `ZWAMEDIAITEM` joined to `ZWAMESSAGE` and `ZWACHATSESSION` | Metadata only — historical attachments don't carry a download reference whatsmeow can resolve, so `download_media` on historical rows can't fetch bytes (tracked as a follow-up). |

4. **Timestamp conversion.** Core Data stores seconds since `2001-01-01 UTC`. `coreDataToUnix(t)` adds 978307200.

Re-running the importer is safe. The schema's primary keys and `ON CONFLICT … DO UPDATE` clauses make every write idempotent.

### Name resolution and the `@lid` quirk

WhatsApp identifies entities with JIDs in several namespaces:

- `<phone>@s.whatsapp.net` — standard contacts.
- `<groupid>@g.us` — groups.
- `<id>@broadcast` — broadcast/status lists.
- `<id>@lid` — Linked Identity (privacy-preserving handles for users who've enabled the new identity system).

For `@lid` specifically there are two shapes:

- **Base form:** `236592602042491@lid`
- **Device form:** `236592602042491:64@lid` — per-sending-device suffix

Only the base form appears in `ZWAPROFILEPUSHNAME` (which is what populates `contacts.push_name` during import). But messages cite the device form as `sender_jid`. A naive `GetContact(sender_jid)` lookup misses every `@lid` sender that has a real push name.

`internal/mcp/tools/messages.go::lookupContactName` handles this by trying both forms:

```
lookupContactName(jid)
  → GetContact(jid)                       # try as-is
  → GetContact(stripDeviceSuffix(jid))    # try base form (":NN" removed)
  → ""                                    # neither hit
```

Push names go through `cleanPushName` (in `discovery.go`) as a defense-in-depth filter: strings that are 12+ chars and contain only base64-alphabet characters are dropped (treated as serialized protobuf identity hints rather than human names). Real names — `"Alice"`, `"Bob Barker"`, `"+65 9298 0156"` — pass through unchanged.

The same enrichment runs in two places:

- `chatDisplayName(jid, kind)` in `discovery.go` enriches `list_chats` and `get_chat` with a `name` field (group name for groups, contact push name for DMs).
- `messageToMap(ctx, q, m)` in `messages.go` adds `sender_name` to every message row (`"You"` for `from_me`, otherwise resolved via `lookupContactName`).

### On-disk layout

```
~/.wadb/                            (mode 0700)
  session.db                        whatsmeow's session + identity + prekeys (WAL)
  session.db-wal                    WAL journal
  session.db-shm                    shared-memory index
  wadb.db                           application DB (WAL + FTS5)
  wadb.db-wal
  wadb.db-shm
  media/                            (mode 0700)
    <sha256>.<ext>                  downloaded blobs, content-addressed
```

Files are created at `0600`, directories at `0700`. There are no other state files; deleting `~/.wadb/` and re-running `wadb link` is a complete reset.

### Repository layout

```
cmd/wadb/                   # main; subcommands link, serve, import-history
  main.go                   # subcommand dispatch
  link.go                   # QR + two-phase handshake wait
  serve.go                  # MCP server + ingester wiring
  import.go                 # import-history CLI

internal/
  config/                   # WADB_HOME resolution, paths, log level
  db/                       # SQLite open + WAL, embedded migrations, typed Queries
    migrations/             # forward-only versioned SQL (001_initial, 002_fts)
  waclient/                 # WhatsApp Client interface, whatsmeow wrapper, slog adapter, test Fake
  waevent/                  # normalized event types (shared by ingest and waclient)
  ingest/                   # event → DB writer (single goroutine, sole writer)
  macimport/                # macOS WhatsApp Desktop → wadb.db importer
  mcp/                      # MCP server wiring + e2e test
    cursor/                 # opaque (ts, id) pagination cursors
    tools/                  # one file per tool group: status / discovery / messages / send / media
  media/                    # sha256-keyed download cache
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
