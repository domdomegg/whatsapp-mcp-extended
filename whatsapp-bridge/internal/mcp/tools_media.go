package mcp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	qrcode "github.com/skip2/go-qrcode"

	"whatsapp-bridge/internal/whatsapp"
)

type sendFileIn struct {
	Recipient         string `json:"recipient" jsonschema:"Phone number with country code (digits only) or a JID"`
	MediaPath         string `json:"media_path,omitempty" jsonschema:"Absolute path to the file on the server (image, video, audio or document)"`
	FileContentBase64 string `json:"file_content_base64,omitempty" jsonschema:"The file's bytes, base64-encoded; use this for a file the client holds, since the server cannot read the client's filesystem. Requires filename."`
	Filename          string `json:"filename,omitempty" jsonschema:"Name for the file including extension (e.g. screenshot.png); required with file_content_base64, the extension decides how WhatsApp shows it"`
}

type sendAudioIn struct {
	Recipient string `json:"recipient" jsonschema:"Phone number with country code (digits only) or a JID"`
	MediaPath string `json:"media_path" jsonschema:"Absolute path to the audio file on the server; converted to Opus .ogg with ffmpeg unless it already is one"`
}

type downloadMediaIn struct {
	MessageID string `json:"message_id" jsonschema:"The ID of the message containing the media (from list_messages)"`
	ChatJID   string `json:"chat_jid" jsonschema:"The JID of the chat containing the message"`
}

var inlineImageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}

func mimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return "application/octet-stream"
}

// imageAndJSON is the shape FastMCP produced for "[Image, dict]" returns: an
// image block followed by the status as text.
func imageAndJSON(png []byte, mime string, status any) *sdk.CallToolResult {
	text := jsonResult(status)
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.ImageContent{Data: png, MIMEType: mime}, text.Content[0]}}
}

// rawTool is for handlers that build their own content (images).
func rawTool[In any](s *Server, name, title, description string, a annotations, h func(ctx context.Context, in In) (*sdk.CallToolResult, error)) {
	sdk.AddTool(s.mcp, &sdk.Tool{Name: name, Title: title, Description: description, Annotations: a.toSDK(title), OutputSchema: a.outputSchema()},
		func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
			r, err := h(ctx, in)
			if err != nil {
				return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: err.Error()}}}, nil, nil
			}
			return r, nil, nil
		})
}

func (s *Server) registerMediaTools() {
	tool(s, "send_file", "Send File",
		"Send a file such as a picture, raw audio, video or document via WhatsApp. Provide the file either as a path on the server (media_path) or inline as base64 (file_content_base64 + filename). Exactly one of the two is required.",
		writes(), func(_ context.Context, in sendFileIn) (any, error) {
			if in.Recipient == "" {
				return fail("Recipient must be provided"), nil
			}
			if in.MediaPath != "" && in.FileContentBase64 != "" {
				return fail("Provide either media_path or file_content_base64, not both"), nil
			}
			path := in.MediaPath
			if in.FileContentBase64 != "" {
				if in.Filename == "" {
					return fail("filename must be provided with file_content_base64"), nil
				}
				content, err := base64.StdEncoding.DecodeString(in.FileContentBase64)
				if err != nil {
					return fail("Invalid base64 content: %v", err), nil
				}
				dir := filepath.Join(s.mediaDir, "outgoing")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fail("prepare outgoing dir: %v", err), nil
				}
				path = filepath.Join(dir, filepath.Base(in.Filename))
				if err := os.WriteFile(path, content, 0o644); err != nil {
					return fail("write file: %v", err), nil
				}
				defer os.Remove(path)
			}
			if path == "" {
				return fail("Exactly one of media_path or file_content_base64 is required"), nil
			}
			r := s.client.SendMessage(s.store, in.Recipient, "", path)
			return sendOut(in.Recipient, r.Success, r.MessageID, r.Timestamp.Format(timeLayout), r.Error), nil
		})

	tool(s, "send_audio_message", "Send Audio Message",
		"Send an audio file as a WhatsApp voice message (converted to Opus .ogg with ffmpeg if needed). If ffmpeg is unavailable, use send_file instead.",
		writes(), func(_ context.Context, in sendAudioIn) (any, error) {
			if in.Recipient == "" {
				return fail("Recipient must be provided"), nil
			}
			if in.MediaPath == "" {
				return fail("Media path must be provided"), nil
			}
			if st, err := os.Stat(in.MediaPath); err != nil || st.IsDir() {
				return fail("Media file not found: %s", in.MediaPath), nil
			}
			path := in.MediaPath
			if !strings.HasSuffix(strings.ToLower(path), ".ogg") {
				converted, err := convertToOpusOgg(path, s.mediaDir)
				if err != nil {
					return fail("Error converting file to opus ogg. You likely need to install ffmpeg: %v", err), nil
				}
				defer os.Remove(converted)
				path = converted
			}
			r := s.client.SendMessage(s.store, in.Recipient, "", path)
			return sendOut(in.Recipient, r.Success, r.MessageID, r.Timestamp.Format(timeLayout), r.Error), nil
		})

	rawTool(s, "download_media", "Download Media",
		"Download media from a WhatsApp message and get the local file path. Images are also returned inline as an image block. Use list_messages first to find messages with media_type set.",
		readOnly(), func(ctx context.Context, in downloadMediaIn) (*sdk.CallToolResult, error) {
			path, _, err := s.client.DownloadMessageMedia(ctx, s.store, s.mediaDir, in.MessageID, in.ChatJID)
			if err != nil {
				msg := "Failed to download media"
				if errors.Is(err, whatsapp.ErrMessageNotFound) || errors.Is(err, whatsapp.ErrNoMedia) {
					msg = err.Error()
				}
				return jsonResult(m{"success": false, "message": msg}), nil
			}
			status := m{"success": true, "message": "Media downloaded successfully", "file_path": path}
			ext := strings.ToLower(filepath.Ext(path))
			if inlineImageExts[ext] {
				if data, err := os.ReadFile(path); err == nil {
					return imageAndJSON(data, mimeForExt(ext), status), nil
				}
			}
			return jsonResult(status), nil
		})

	rawTool(s, "get_setup_qr", "Get Setup QR",
		"Get the WhatsApp linking QR code to connect this account. Call when setup_status says needs_pairing; scan it from WhatsApp on your phone: Settings > Linked Devices > Link a Device. The QR rotates — call again for a fresh one.",
		readOnly(), func(_ context.Context, _ struct{}) (*sdk.CallToolResult, error) {
			if s.client.Store.ID != nil {
				return jsonResult(m{"success": true, "needs_pairing": false, "message": "Account already linked."}), nil
			}
			code := s.client.CurrentQR()
			if code == "" {
				return jsonResult(m{"success": false, "needs_pairing": true, "error": "No QR code available yet; the client is still connecting. Try again in a few seconds."}), nil
			}
			png, err := qrcode.Encode(code, qrcode.Medium, 512)
			if err != nil {
				return nil, fmt.Errorf("render QR: %w", err)
			}
			return imageAndJSON(png, "image/png", m{
				"success": true, "needs_pairing": true,
				"message": "Scan this QR in WhatsApp > Linked Devices > Link a Device. It rotates; call again for a fresh code.",
			}), nil
		})

	tool(s, "setup_status", "Setup Status",
		"Check whether this WhatsApp account is linked and connected: needs_pairing (true means scan get_setup_qr), connected, and sync statistics.",
		readOnly(), func(_ context.Context, _ struct{}) (any, error) {
			out := s.syncStatus()
			for k, v := range s.connectionStatus() {
				out[k] = v
			}
			return out, nil
		})

	tool(s, "unlink", "Unlink Account",
		"Log out and unlink this WhatsApp account, wiping the local session. A new QR scan via get_setup_qr is needed to use it again. Use with care.",
		destructive(), func(ctx context.Context, _ struct{}) (any, error) {
			if err := s.client.Client.Logout(ctx); err != nil {
				return fail("Failed to log out: %v", err), nil
			}
			return m{"success": true}, nil
		})
}

// convertToOpusOgg runs ffmpeg with the voice-note settings the Python server used.
func convertToOpusOgg(input, mediaDir string) (string, error) {
	dir := filepath.Join(mediaDir, "outgoing")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(dir, strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))+".ogg")
	cmd := exec.Command("ffmpeg", "-i", input, "-c:a", "libopus", "-b:a", "32k", "-ar", "24000", "-application", "voip",
		"-vbr", "on", "-compression_level", "10", "-frame_duration", "60", "-y", out)
	if stderr, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(stderr)))
	}
	return out, nil
}
