# wadb — WhatsApp MCP server

A single Go binary that links to your WhatsApp account as a Web "linked device" via [whatsmeow](https://github.com/tulir/whatsmeow), mirrors messages/contacts/groups into a local SQLite DB, and exposes that DB plus send/react/download capabilities over a stdio Model Context Protocol server.

See [`docs/superpowers/specs/2026-05-28-wadb-design.md`](docs/superpowers/specs/2026-05-28-wadb-design.md) for the full design.

## Install

```bash
go build -o wadb ./cmd/wadb
```

## Link

```bash
./wadb link
```

A QR appears on stderr. Open WhatsApp → Settings → Linked Devices → Link a device, scan the QR. The command exits as soon as pairing completes. The session is persisted under `~/.wadb/`.

## Run

```bash
./wadb serve
```

Speaks MCP on stdio. Configure your MCP client (Claude Desktop, Claude Code, etc.) to launch `./wadb serve` as a subprocess.

## Environment

| Variable          | Default     | Meaning                                                  |
|-------------------|-------------|----------------------------------------------------------|
| `WADB_HOME`       | `~/.wadb`   | Data directory: session DB, app DB, media cache.         |
| `WADB_LOG_LEVEL`  | `info`      | `debug`, `info`, `warn`, `error`. Logs go to stderr.     |

## Tools

| Tool              | Reads/Writes  | Purpose                                                       |
|-------------------|---------------|---------------------------------------------------------------|
| `status`          | DB            | Connection state and DB stats.                                |
| `list_chats`      | DB            | Recent chats, newest first; filter by kind.                   |
| `list_contacts`   | DB            | Substring search over contacts.                               |
| `list_groups`     | DB            | Substring search over groups you're in.                       |
| `get_chat`        | DB            | One chat; includes group participants for groups.             |
| `get_messages`    | DB            | Page through a chat's messages.                               |
| `search_messages` | DB (FTS5)     | Full-text search across all message text/captions.            |
| `get_message`     | DB            | One message with quoted message and media metadata expanded.  |
| `download_media`  | WhatsApp + FS | Decrypt and cache a media blob locally.                       |
| `send_text`       | WhatsApp      | Send a text message; supports replies.                        |
| `send_media`      | WhatsApp      | Send a file from disk; kind inferred from MIME.               |
| `react`           | WhatsApp      | Add/remove a reaction.                                        |
| `mark_read`       | WhatsApp      | Mark a chat read.                                             |

## Manual verification

After major changes, run through:

- [ ] `wadb link` — pair from a clean `WADB_HOME`
- [ ] `wadb serve` — connects without errors
- [ ] Send a text in a DM → `get_messages` shows it
- [ ] Receive a text in a DM → ingester writes the row
- [ ] Send an image in a DM with caption
- [ ] Receive an image → `download_media` fetches and caches
- [ ] React to a message → `get_message` shows the reaction
- [ ] Reply to a message → `get_message` shows the quoted message
- [ ] Observe an incoming edit → row's `text` updates, `edited_at` set
- [ ] Observe an incoming delete → row's `text` cleared, `deleted_at` set
- [ ] Restart `serve` → catch-up brings in messages received during downtime

## Testing

```bash
go test ./...      # hermetic; no network, no WhatsApp
go vet ./...
go build ./...
```

## Architecture

See the design doc. In short: single Go process; `whatsmeow.Client` owns the socket; a single goroutine drains events and writes to SQLite; the MCP server reads SQLite for queries and calls into the client for sends/downloads. SQLite is WAL-mode; the ingester is the sole writer.

### Layout

```
cmd/wadb/                   # main; subcommands link, serve
internal/
  config/                   # WADB_HOME resolution, paths, log level
  db/                       # sqlite open, migrations, typed Queries
    migrations/             # forward-only versioned SQL
  waclient/                 # WhatsApp Client interface + whatsmeow wrapper + test fake
  waevent/                  # normalized event types shared by ingest and waclient
  ingest/                   # event → DB writer (single goroutine, sole writer)
  mcp/                      # MCP server wiring
    cursor/                 # opaque pagination cursors
    tools/                  # one file per tool group
  media/                    # sha256-keyed download cache
```
