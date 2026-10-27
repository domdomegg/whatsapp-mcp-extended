# WhatsApp MCP Extended

One Go binary, `whatsapp-mcp`: a [whatsmeow](https://github.com/tulir/whatsmeow) WhatsApp client and a Model Context Protocol server on stdio. It is run once per user by [mcp-auth-wrapper](https://www.npmjs.com/package/mcp-auth-wrapper), which terminates OIDC and exposes streamable HTTP; an agent or MCP aggregator connects to the wrapper, and each user links their own WhatsApp account from inside the MCP session.

> Fork of [FelixIsaac/whatsapp-mcp-extended](https://github.com/FelixIsaac/whatsapp-mcp-extended), itself built on [AdamRussak/whatsapp-mcp](https://github.com/AdamRussak/whatsapp-mcp), which forked [lharries/whatsapp-mcp](https://github.com/lharries/whatsapp-mcp) (the original). This fork drops the REST API, Python server, web UI and webhooks and keeps only the multi-user MCP server.

## Running it

The image is `ghcr.io/domdomegg/whatsapp-mcp-extended`. It has no configuration of its own: everything is the wrapper's config, passed as `MCP_AUTH_WRAPPER_CONFIG`.

```sh
docker run -p 3000:3000 -v whatsapp-data:/app/data \
  -e MCP_AUTH_WRAPPER_CONFIG='{
    "command": ["/app/whatsapp-mcp"],
    "auth": {"issuer": "https://oidc.example.com"},
    "allowedRedirectUris": ["https://agent.example.com/oauth/callback"],
    "storage": "/app/data/mcp.sqlite",
    "issuerUrl": "https://whatsapp.mcp.example.com",
    "secret": "<random>"
  }' \
  ghcr.io/domdomegg/whatsapp-mcp-extended
```

The wrapper listens on port 3000 and spawns `/app/whatsapp-mcp` per authenticated user and profile, setting `MCP_USER_ID` and `MCP_PROFILE_ID`. The binary reads `DATA_ROOT` (default `/app/data` in the image) and nothing else.

### Linking an account

Call the `get_setup_qr` tool. It returns the pairing QR as an image; scan it in WhatsApp > Linked Devices > Link a Device. `setup_status` reports whether the account is linked, connected and how far history sync has got; `unlink` logs out.

### Data

```
$DATA_ROOT/store/<user>/<profile>/
  whatsapp.db   whatsmeow session (the device link; losing it means re-pairing)
  messages.db   messages, chats, contacts
  media/        downloaded media
```

Back up the whole `store/` directory; nothing else is state. Each profile is one WhatsApp account, so one user can link several (`default` is the profile when the wrapper sets none).

### Health

`whatsapp-mcp healthcheck` exits non-zero when a linked account has been disconnected from WhatsApp for more than 2 minutes, and zero otherwise (unlinked accounts are ignored). It is the Kubernetes liveness probe for the container.

## Tools

| Group | Tools |
|---|---|
| Core | `search_contacts`, `list_messages`, `list_chats`, `get_chat`, `get_message_context`, `list_all_contacts`, `get_contact_context`, `get_direct_chat_by_contact`, `get_group_info`, `get_profile_picture` |
| Send | `send_message`, `send_reaction`, `create_poll` |
| Media | `send_file`, `send_audio_message`, `download_media` |
| Message admin | `edit_message`, `delete_message`, `mark_read` |
| Contacts | `manage_nickname` |
| Groups | `manage_group` |
| History | `request_history` |
| Presence | `set_presence`, `subscribe_presence` |
| Account | `get_setup_qr`, `setup_status`, `unlink`, `get_blocklist`, `manage_blocklist` |
| Newsletter | `manage_newsletter` |

Every tool returns structured content and declares an `outputSchema`; tools that return a list or a nullable lookup wrap it as `{"result": ...}` (MCP requires an object there).

Every tool returns structured content and declares an `outputSchema`; tools that return a list or a nullable lookup wrap it as `{"result": ...}` (MCP requires an object there).

Resources: `whatsapp://status` (connection and link state) and `whatsapp://sync-status` (message and chat counts, newest message time).

JIDs: individuals are `<phone>@s.whatsapp.net` (or `<id>@lid`), groups `<id>@g.us`. Message IDs are the hex strings returned by the send tools and are what `edit_message`, `delete_message`, `send_reaction` and `mark_read` take.

## Account risk

This is a third-party client on an unofficial API. WhatsApp requires the phone to come online at least every 14 days and the linked device every 30, and it bans automation it detects. Use a dedicated number if you can; for anything business-critical the [WhatsApp Business API](https://developers.facebook.com/docs/whatsapp/cloud-api) is the only compliant path.

## Development

```sh
cd whatsapp-bridge
go build -o whatsapp-mcp . && go test -race ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md). When whatsmeow starts returning `Client outdated (405)`: `go get -u go.mau.fi/whatsmeow@latest && go mod tidy`.

## Credits

**Fork chain:**
- [lharries/whatsapp-mcp](https://github.com/lharries/whatsapp-mcp) - Original MCP server (12 tools)
- [AdamRussak/whatsapp-mcp](https://github.com/AdamRussak/whatsapp-mcp) - Added webhooks, container split, webhook UI
- [FelixIsaac/whatsapp-mcp-extended](https://github.com/FelixIsaac/whatsapp-mcp-extended) - Added reactions, edit/delete, groups, polls, presence, newsletters, and a curated MCP tool surface
- This repo - Per-user store keying, self-service QR onboarding, and the single-binary Go MCP server

**Libraries:**
- [whatsmeow](https://github.com/tulir/whatsmeow) - Go WhatsApp Web API

Community fixes folded in upstream and kept here: [simonseifert](https://github.com/simonseifert/whatsapp-mcp-extended-pro) (CDN fallback via `direct_path`, inline images in `download_media`), [laudite](https://github.com/laudite/whatsapp-mcp-extended) (media captions, `@mention` detection, `request_history` target fix), [kasperpeulen](https://github.com/kasperpeulen/whatsapp-mcp-extended) (contact name resolution, LID to phone JID via `GetAltJID()`), [Coriatel](https://github.com/Coriatel/whatsapp-mcp-extended) (first working media download), [jedijashwa](https://github.com/jedijashwa/whatsapp-mcp-extended) (reactions sender JID fix, MIME types), [slarrain](https://github.com/slarrain/whatsapp-mcp-extended) (LID JID normalisation).

## License

MIT License - see [LICENSE](LICENSE) file.
