// Package mcp is the MCP (stdio) face of the WhatsApp client: one tool per
// thing a client can do, named and shaped exactly as the previous Python
// server's tools were, so prompts and clients written against it keep working.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"whatsapp-bridge/internal/database"
	"whatsapp-bridge/internal/whatsapp"
)

// Server holds what the tools need.
type Server struct {
	client   *whatsapp.Client
	store    *database.MessageStore
	contacts *database.ContactStore
	mediaDir string
	mcp      *sdk.Server
}

// New builds the MCP server with every tool and resource registered.
func New(client *whatsapp.Client, store *database.MessageStore, contacts *database.ContactStore, mediaDir string) *Server {
	s := &Server{client: client, store: store, contacts: contacts, mediaDir: mediaDir}
	s.mcp = sdk.NewServer(&sdk.Implementation{Name: "whatsapp-extended", Version: "2.0.0"}, &sdk.ServerOptions{
		Instructions: "WhatsApp for one linked account. Call setup_status first; if needs_pairing is true, call get_setup_qr and scan it from WhatsApp > Linked Devices.",
	})
	s.registerReadTools()
	s.registerWriteTools()
	s.registerMediaTools()
	s.registerResources()
	return s
}

// Run serves MCP over stdin/stdout until the client goes away.
func (s *Server) Run(ctx context.Context) error {
	return s.mcp.Run(ctx, &sdk.StdioTransport{})
}

// annotations describes a tool the way the Python decorator did.
type annotations struct {
	readOnly, destructive, idempotent, openWorld bool
}

func readOnly() annotations    { return annotations{readOnly: true, idempotent: true} }
func writes() annotations      { return annotations{openWorld: true} }
func destructive() annotations { return annotations{destructive: true, openWorld: true} }
func (a annotations) toSDK(title string) *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{
		Title: title, ReadOnlyHint: a.readOnly, IdempotentHint: a.idempotent,
		DestructiveHint: boolPtr(a.destructive), OpenWorldHint: boolPtr(a.openWorld),
	}
}

func boolPtr(b bool) *bool { return &b }

// tool registers a handler that returns a JSON-serialisable value, which goes
// out both as text and as structured content.
func tool[In any](s *Server, name, title, description string, a annotations, h func(ctx context.Context, in In) (any, error)) {
	sdk.AddTool(s.mcp, &sdk.Tool{Name: name, Title: title, Description: description, Annotations: a.toSDK(title)},
		func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
			out, err := h(ctx, in)
			if err != nil {
				return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: err.Error()}}}, nil, nil
			}
			return jsonResult(out), nil, nil
		})
}

// jsonResult renders a value as a text content block (pretty JSON) and, when
// it is a JSON object, also as structured content. MCP's structuredContent
// must be an object: mcp-auth-wrapper rejected the first deploy's list results
// ("expected record, received array"), so arrays and null travel as text only,
// which is how FastMCP's list returns reached clients anyway.
func jsonResult(v any) *sdk.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: fmt.Sprintf("encode result: %v", err)}}}
	}
	result := &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}}
	if len(b) > 0 && b[0] == '{' {
		result.StructuredContent = v
	}
	return result
}

// m is shorthand for an ad-hoc JSON object, the shape most action tools return.
type m = map[string]any

func fail(format string, args ...any) m {
	return m{"success": false, "error": fmt.Sprintf(format, args...)}
}

func invalidAction(action string, allowed []string, tool string) m {
	return m{
		"success":         false,
		"error":           fmt.Sprintf("Invalid action '%s'. Use one of: %s", action, join(allowed)),
		"allowed_actions": allowed,
		"use_tool":        tool,
	}
}

func join(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

func (s *Server) registerResources() {
	s.mcp.AddResource(&sdk.Resource{URI: "whatsapp://status", Name: "status", MIMEType: "application/json",
		Description: "Live connection state — connected, needs_pairing, disconnected_for."},
		func(_ context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			return resourceJSON(req.Params.URI, s.connectionStatus()), nil
		})
	s.mcp.AddResource(&sdk.Resource{URI: "whatsapp://sync-status", Name: "sync-status", MIMEType: "application/json",
		Description: "Sync statistics — message/chat counts and the newest message time."},
		func(_ context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			return resourceJSON(req.Params.URI, s.syncStatus()), nil
		})
}

func resourceJSON(uri string, v any) *sdk.ReadResourceResult {
	b, _ := json.Marshal(v)
	return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(b)}}}
}

// connectionStatus is the whatsapp://status resource and the core of setup_status.
func (s *Server) connectionStatus() m {
	connected := s.client.IsConnected()
	linked := s.client.Store.ID != nil
	startedAt, lastConn, discAt, reconnErrs := s.client.ConnectionState()
	out := m{
		"success":               true,
		"connected":             connected,
		"linked":                linked,
		"needs_pairing":         !linked,
		"uptime":                sinceString(startedAt),
		"auto_reconnect_errors": reconnErrs,
	}
	if linked {
		out["jid"] = s.client.Store.ID.String()
	}
	if !lastConn.IsZero() {
		out["last_connected"] = lastConn.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	if !discAt.IsZero() {
		out["disconnected_for"] = sinceString(discAt)
	}
	return out
}

func (s *Server) syncStatus() m {
	messages, chats, lastSync := s.store.SyncStatus()
	return m{
		"success":            true,
		"syncing":            false,
		"sync_progress":      100,
		"last_sync":          lastSync,
		"message_count":      messages,
		"conversation_count": chats,
	}
}
