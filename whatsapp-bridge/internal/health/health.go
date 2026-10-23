// Package health is the liveness story for one container running many
// per-account processes. Each process writes a small status file in its store
// directory every few seconds; `whatsapp-mcp healthcheck` (the Kubernetes exec
// probe) reads them all and fails when a linked account has been disconnected
// for too long or a process has stopped writing. A process that has exited is
// not a failure: mcp-auth-wrapper starts a fresh one on the next tool call.
package health

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// FileName is the status file's name inside a store directory.
const FileName = "health.json"

// Status is what a running process reports.
type Status struct {
	PID                    int    `json:"pid"`
	Connected              bool   `json:"connected"`
	NeedsPairing           bool   `json:"needs_pairing"`
	DisconnectedForSeconds int64  `json:"disconnected_for_seconds"`
	UpdatedAt              string `json:"updated_at"`
}

// Limits for the probe.
const (
	WriteInterval     = 15 * time.Second
	StaleAfter        = 60 * time.Second // no write for this long with a live pid = hung
	DisconnectedLimit = 120 * time.Second
)

// Write stores the status atomically in dir.
func Write(dir string, s Status) error {
	s.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, FileName+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, FileName))
}

// Check walks DATA_ROOT/store for status files and returns the problems found.
// now and alive are parameters so the logic is testable without real processes.
func Check(root string, now time.Time, alive func(pid int) bool) []string {
	var problems []string
	_ = filepath.WalkDir(filepath.Join(root, "store"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != FileName {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var s Status
		if json.Unmarshal(b, &s) != nil {
			problems = append(problems, fmt.Sprintf("%s: unreadable", path))
			return nil
		}
		if !alive(s.PID) {
			return nil // exited; the wrapper respawns on demand
		}
		updated, err := time.Parse(time.RFC3339, s.UpdatedAt)
		if err != nil || now.Sub(updated) > StaleAfter {
			problems = append(problems, fmt.Sprintf("%s: pid %d alive but no status since %s", path, s.PID, s.UpdatedAt))
			return nil
		}
		if !s.NeedsPairing && !s.Connected && time.Duration(s.DisconnectedForSeconds)*time.Second > DisconnectedLimit {
			problems = append(problems, fmt.Sprintf("%s: pid %d disconnected for %ds", path, s.PID, s.DisconnectedForSeconds))
		}
		return nil
	})
	return problems
}

// Alive reports whether a process exists (signal 0).
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil || os.IsPermission(syscall.Kill(pid, 0))
}
