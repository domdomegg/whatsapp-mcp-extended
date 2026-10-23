# Security Policy

## Reporting a Vulnerability

To report a security vulnerability privately, use [GitHub's private vulnerability reporting](https://github.com/domdomegg/whatsapp-mcp-extended/security/advisories/new).

Upstream (`FelixIsaac/whatsapp-mcp-extended`) has its own policy; report issues in code that came from upstream there too.

## Threat Model

The binary has no network listener of its own: it speaks MCP on stdio and is spawned per authenticated user by [mcp-auth-wrapper](https://www.npmjs.com/package/mcp-auth-wrapper), which terminates OIDC and is the only thing exposed (port 3000). Each user's WhatsApp session and message store sit in their own directory under `DATA_ROOT/store/<user>/<profile>/`; a process only ever opens the directory for the user it was spawned for.

### What is out of scope
- **Prompt injection via incoming messages** — an attacker who can send you WhatsApp messages may craft a message that influences the agent's behaviour. Mitigate at the agent layer (human approval before destructive actions).
- **Malicious media files** — downloaded media is written to the store. Scan before opening untrusted files.
- **WhatsApp account bans** — this is an unofficial client. Do not use for mass messaging.

## Supported Versions

Only the latest release is supported. Upgrade to the latest version before reporting.
