// Package config resolves where this account's state lives. There is one
// deployment shape — mcp-auth-wrapper spawns one process per user profile with
// MCP_USER_ID and MCP_PROFILE_ID in the environment — so there are no knobs
// beyond the two paths.
package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

// History sync limits requested from the phone on first link. Fixed: they only
// matter once, and the old env overrides were never used.
const (
	HistorySyncDaysLimit uint32 = 365
	HistorySyncSizeMB    uint32 = 5000
	StorageQuotaMB       uint32 = 10240
)

const encodedPrefix = "b~"

// pathSegment maps an identity to a filesystem-safe directory name without
// collisions. Safe ids pass through unchanged (so existing store directories
// keep their names); anything else is base64url behind a prefix that a safe id
// cannot start with, because "~" is not in the safe set. Same scheme as the
// previous Python launcher, so stores created by it are found.
func pathSegment(value string) string {
	safe := value != "" && !strings.HasPrefix(value, encodedPrefix)
	for _, c := range value {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			safe = false
			break
		}
	}
	if safe {
		return value
	}
	return encodedPrefix + strings.TrimRight(base64.URLEncoding.EncodeToString([]byte(value)), "=")
}

// StoreDir is where this account's whatsapp.db, messages.db and media live:
// STORE_DIR when set (local runs), otherwise
// DATA_ROOT/store/<MCP_USER_ID>/<MCP_PROFILE_ID>, which is the layout
// mcp-auth-wrapper documents. DATA_ROOT defaults to /app/data (the image's
// volume); MCP_USER_ID defaults to "default" so a bare `whatsapp-mcp` still
// has somewhere to go.
func StoreDir() string {
	if d := os.Getenv("STORE_DIR"); d != "" {
		return d
	}
	root := os.Getenv("DATA_ROOT")
	if root == "" {
		root = "/app/data"
	}
	user := os.Getenv("MCP_USER_ID")
	if user == "" {
		user = "default"
	}
	dir := filepath.Join(root, "store", pathSegment(user))
	if profile := os.Getenv("MCP_PROFILE_ID"); profile != "" {
		dir = filepath.Join(dir, pathSegment(profile))
	}
	return dir
}

// MediaDir is where downloaded media is written, under the store so it is
// per-account and covered by the same volume.
func MediaDir() string {
	return filepath.Join(StoreDir(), "media")
}
