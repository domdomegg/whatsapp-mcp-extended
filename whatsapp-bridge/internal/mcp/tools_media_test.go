package mcp

import (
	"testing"
)

// A tool that declares an outputSchema must return structuredContent, or
// clients reject the whole result. imageAndJSON used to drop it.
func TestImageAndJSONCarriesStructuredContent(t *testing.T) {
	r := imageAndJSON([]byte{1, 2, 3}, "image/png", m{"success": true, "file_path": "/x.png"})
	if r.StructuredContent == nil {
		t.Fatalf("StructuredContent is nil")
	}
	got, ok := r.StructuredContent.(m)
	if !ok || got["file_path"] != "/x.png" || got["success"] != true {
		t.Fatalf("unexpected structured content: %#v", r.StructuredContent)
	}
	if len(r.Content) != 2 {
		t.Fatalf("want image + text content, got %d blocks", len(r.Content))
	}
}
