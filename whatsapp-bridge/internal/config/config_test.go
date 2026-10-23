package config

import (
	"path/filepath"
	"testing"
)

func TestPathSegmentPassesSafeIdsThrough(t *testing.T) {
	for _, id := range []string{"77f00f25ead04b3b90c67898431154dd", "pmslbno7b", "a-b_C9"} {
		if got := pathSegment(id); got != id {
			t.Errorf("pathSegment(%q) = %q, want unchanged", id, got)
		}
	}
}

func TestPathSegmentEncodesUnsafeIdsWithoutCollisions(t *testing.T) {
	a, b := pathSegment("adam@x.com"), pathSegment("adam.x.com")
	if a == b {
		t.Fatalf("collision: %q", a)
	}
	for _, s := range []string{a, b, pathSegment(""), pathSegment("b~looks-encoded")} {
		if len(s) < 2 || s[:2] != "b~" {
			t.Errorf("%q should carry the encoded prefix", s)
		}
		if filepath.Base(s) != s || s == "." || s == ".." {
			t.Errorf("%q is not a single safe path segment", s)
		}
	}
}

func TestStoreDirLayout(t *testing.T) {
	t.Setenv("STORE_DIR", "")
	t.Setenv("DATA_ROOT", "/data")
	t.Setenv("MCP_USER_ID", "user1")
	t.Setenv("MCP_PROFILE_ID", "work")
	if got, want := StoreDir(), filepath.Join("/data", "store", "user1", "work"); got != want {
		t.Errorf("StoreDir() = %q, want %q", got, want)
	}
	t.Setenv("MCP_PROFILE_ID", "")
	if got, want := StoreDir(), filepath.Join("/data", "store", "user1"); got != want {
		t.Errorf("StoreDir() without profile = %q, want %q", got, want)
	}
	t.Setenv("STORE_DIR", "/elsewhere")
	if got := StoreDir(); got != "/elsewhere" {
		t.Errorf("STORE_DIR override ignored: %q", got)
	}
	if got, want := MediaDir(), filepath.Join("/elsewhere", "media"); got != want {
		t.Errorf("MediaDir() = %q, want %q", got, want)
	}
}
