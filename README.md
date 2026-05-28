# wadb

WhatsApp MCP server. Links to your WhatsApp account as a Web linked device, mirrors messages into a local SQLite DB, and exposes that DB plus send/react/download capabilities via the Model Context Protocol.

See `docs/superpowers/specs/2026-05-28-wadb-design.md` for the full design.

## Quick start

```bash
go build -o wadb ./cmd/wadb
./wadb link            # scan QR with WhatsApp → Linked Devices
./wadb serve           # speaks MCP on stdio
```
