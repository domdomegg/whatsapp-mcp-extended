package health

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func write(t *testing.T, root, user, profile string, s Status, at time.Time) string {
	t.Helper()
	dir := filepath.Join(root, "store", user, profile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, s); err != nil {
		t.Fatal(err)
	}
	// Rewrite updated_at to the requested time.
	b := []byte(`{"pid":` + itoa(s.PID) + `,"connected":` + boolStr(s.Connected) + `,"needs_pairing":` + boolStr(s.NeedsPairing) +
		`,"disconnected_for_seconds":` + itoa(int(s.DisconnectedForSeconds)) + `,"updated_at":"` + at.UTC().Format(time.RFC3339) + `"}`)
	if err := os.WriteFile(filepath.Join(dir, FileName), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func itoa(i int) string { return strconv.Itoa(i) }
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestCheck(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	alivePIDs := map[int]bool{1: true, 2: true, 3: true, 4: true}
	alive := func(pid int) bool { return alivePIDs[pid] }

	write(t, root, "u1", "p1", Status{PID: 1, Connected: true}, now)                                                // healthy
	write(t, root, "u1", "p2", Status{PID: 2, NeedsPairing: true, DisconnectedForSeconds: 9999}, now)               // unpaired: fine
	write(t, root, "u2", "p1", Status{PID: 3, Connected: false, DisconnectedForSeconds: 300}, now)                  // disconnected too long
	write(t, root, "u3", "p1", Status{PID: 4, Connected: true}, now.Add(-5*time.Minute))                            // hung
	write(t, root, "u4", "p1", Status{PID: 9, Connected: false, DisconnectedForSeconds: 9999}, now.Add(-time.Hour)) // exited: ignored

	problems := Check(root, now, alive)
	if len(problems) != 2 {
		t.Fatalf("want 2 problems, got %v", problems)
	}
	if !contains(problems, "pid 3 disconnected for 300s") || !contains(problems, "pid 4 alive but no status") {
		t.Errorf("unexpected problems: %v", problems)
	}
	if got := Check(filepath.Join(root, "missing"), now, alive); len(got) != 0 {
		t.Errorf("missing root should be healthy, got %v", got)
	}
}

func TestWriteSetsUpdatedAt(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Status{PID: 1, Connected: true}); err != nil {
		t.Fatal(err)
	}
	if got := Check(filepath.Dir(dir), time.Now(), func(int) bool { return true }); len(got) != 0 {
		t.Errorf("fresh file should be healthy: %v", got)
	}
}

func contains(ss []string, sub string) bool {
	for _, s := range ss {
		if len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
