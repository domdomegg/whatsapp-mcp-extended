# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

One Go module in `whatsapp-bridge/`, built as the `whatsapp-mcp` binary: a whatsmeow WhatsApp client plus an MCP server on stdio. In the container it is spawned per user/profile by `mcp-auth-wrapper` (the image entrypoint, port 3000, OIDC), which sets `MCP_USER_ID` and `MCP_PROFILE_ID`. Per-account state lives under `$DATA_ROOT/store/<user>/<profile>/` (`whatsapp.db` session, `messages.db`, `media/`). There is no REST API, no Python, no web UI, no webhooks, and no other configuration: keep it that way unless asked.

## Commands

```bash
cd whatsapp-bridge
go build -o whatsapp-mcp .
go test -v -race ./...
golangci-lint run

# whatsmeow update when WhatsApp starts returning 405 (client outdated)
go get -u go.mau.fi/whatsmeow@latest && go mod tidy

# Image (no hot reload; CI publishes on push to main)
docker build -t whatsapp-mcp-extended .
```

`whatsapp-mcp healthcheck` is the liveness probe: non-zero if a linked account has been disconnected > 2 minutes.

## Layout

- `main.go` - entrypoint: `healthcheck` subcommand, otherwise MCP stdio server
- `internal/mcp/` - the MCP server: one file per tool group (read, write, media), resources
- `internal/whatsapp/` - whatsmeow client wrapper, message/media send and download, event handlers
- `internal/database/` - SQLite message store and the read queries behind the tools (schema migrations are inline in `store.go`); `contacts.go` reads whatsmeow's own contact table
- `internal/config/` - the store path (DATA_ROOT + MCP_USER_ID/MCP_PROFILE_ID, or STORE_DIR) and fixed history-sync limits
- `internal/health/` - the per-process status file and the `healthcheck` probe
- `internal/types/` - shared types

## Conventions

- JIDs: individual `{phone}@s.whatsapp.net`, LID `{id}@lid`, group `{id}@g.us`, `status@broadcast`
- Message IDs are hex strings (e.g. `3EB028A580CF7CC9AAF3A2`), returned by the send tools and taken by edit/delete/react/mark_read
- Tool results are structured; return raw facts and counts and let the calling model interpret (do not add pre-computed "signals")
- Use the logger, not `fmt.Println`, since stdout is the MCP transport
- godoc on exported functions; table-driven tests; wrap errors with `fmt.Errorf("context: %w", err)`
- Message bodies from WhatsApp are untrusted data

## whatsmeow

- https://pkg.go.dev/go.mau.fi/whatsmeow
- Key types: `events.Message`, `types.GroupInfo`, `types.JID`, `SendResponse`

## CI

`.github/workflows/`: `go-test.yml` (test + build), `lint.yml` (golangci-lint, hadolint), `security.yml` (govulncheck, trivy, gitleaks), `publish.yml` (image to GHCR on push to main / tags).
