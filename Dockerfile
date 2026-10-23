# One image: the whatsapp-mcp Go binary (whatsmeow client + MCP stdio server)
# plus mcp-auth-wrapper, which handles OIDC on port 3000 and spawns
# /app/whatsapp-mcp once per authenticated user/profile.
#
# node:lts-slim (Debian/glibc) because the wrapper needs Node 22+ (node:sqlite)
# and the cgo-built binary needs glibc.

# --- stage 1: build the Go binary -------------------------------------------
FROM golang:latest AS builder
ENV CGO_ENABLED=1
WORKDIR /src
COPY ./whatsapp-bridge .
RUN go build -o whatsapp-mcp .

# --- stage 2: runtime --------------------------------------------------------
FROM node:lts-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Pinned so startup needs no network. 2.0+ adds profiles (MCP_PROFILE_ID).
RUN npm install -g mcp-auth-wrapper@2.0.2 && npm cache clean --force

WORKDIR /app
COPY --from=builder /src/whatsapp-mcp /app/whatsapp-mcp
RUN mkdir -p /app/data && chown -R node:node /app

USER node
ENV HOME=/home/node
# Per-account state: DATA_ROOT/store/<MCP_USER_ID>/<MCP_PROFILE_ID>/
ENV DATA_ROOT=/app/data
VOLUME ["/app/data"]

# The wrapper reads MCP_AUTH_WRAPPER_CONFIG and spawns /app/whatsapp-mcp per
# user/profile; the binary speaks MCP on stdio.
EXPOSE 3000
ENTRYPOINT ["mcp-auth-wrapper"]
