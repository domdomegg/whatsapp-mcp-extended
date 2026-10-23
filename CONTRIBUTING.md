# Contributing to WhatsApp MCP Extended

Thanks for your interest in contributing!

## Development

Everything lives in `whatsapp-bridge/` (one Go module, built as the `whatsapp-mcp` binary).

```bash
cd whatsapp-bridge
go build -o whatsapp-mcp .
go test -race ./...
go vet ./...
```

The binary is an MCP server on stdio, so you can point any MCP client at it directly for local testing (set `DATA_ROOT` to a scratch directory; `MCP_USER_ID`/`MCP_PROFILE_ID` pick the account subdirectory).

Build the container image with `docker build -t whatsapp-mcp-extended .`.

## Code Style

- Run `go fmt` and `golangci-lint run` before committing
- Follow standard Go conventions; table-driven tests preferred

## Pull Requests

1. Fork the repo
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Run tests and checks
5. Commit with clear messages
6. Push and open a PR

### PR Guidelines

- Keep PRs focused on a single feature/fix
- Update the tool list in `README.md` if you add or remove a tool
- Add tests for new functionality
- Ensure all checks pass

## Reporting Issues

- Check existing issues first
- Include steps to reproduce
- Include error logs if applicable
- Specify your environment (OS, Docker version, etc.)

## Questions?

Open an issue or discussion on GitHub.
