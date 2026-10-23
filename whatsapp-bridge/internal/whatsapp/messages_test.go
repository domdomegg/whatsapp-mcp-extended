package whatsapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestValidateMediaPath(t *testing.T) {
	// Only the account's store directory and the temp dir are readable: that
	// is where downloads and inline uploads land. No env override exists.
	// A store outside the temp dir, so the two allowed roots stay distinct;
	// validateMediaPath only compares paths, nothing is created.
	store := "/var/lib/whatsapp-mcp-test-store"
	t.Setenv("STORE_DIR", store)
	tmp := os.TempDir()
	tests := []struct {
		name        string
		path        string
		errContains string
	}{
		{"empty path", "", ""},
		{"inside the store", filepath.Join(store, "media", "file.jpg"), ""},
		{"inside the temp dir", filepath.Join(tmp, "upload.png"), ""},
		{"path traversal", store + "/../etc/passwd", "traversal"},
		{"store prefix but sibling dir", store + "-other/file.jpg", "outside allowed"},
		{"outside /etc", "/etc/passwd", "outside allowed"},
		{"outside /home", "/home/user/file.txt", "outside allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMediaPath(tt.path)
			if tt.errContains == "" {
				if err != nil {
					t.Errorf("validateMediaPath(%s) = %v, want nil", tt.path, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("validateMediaPath(%s) = %v, want error containing %q", tt.path, err, tt.errContains)
			}
		})
	}
}

func TestMediaTypeAndMimeType(t *testing.T) {
	tests := []struct {
		name          string
		mediaPath     string
		wantMediaType whatsmeow.MediaType
		wantMimeType  string
	}{
		{
			name:          "pdf document",
			mediaPath:     "/tmp/report.pdf",
			wantMediaType: whatsmeow.MediaDocument,
			wantMimeType:  "application/pdf",
		},
		{
			name:          "docx document",
			mediaPath:     "/tmp/spec.DOCX",
			wantMediaType: whatsmeow.MediaDocument,
			wantMimeType:  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			name:          "xlsx document",
			mediaPath:     "/tmp/budget.xlsx",
			wantMediaType: whatsmeow.MediaDocument,
			wantMimeType:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			name:          "cbz document",
			mediaPath:     "/tmp/comic.cbz",
			wantMediaType: whatsmeow.MediaDocument,
			wantMimeType:  "application/x-cbz",
		},
		{
			name:          "jpg image",
			mediaPath:     "/tmp/photo.jpg",
			wantMediaType: whatsmeow.MediaImage,
			wantMimeType:  "image/jpeg",
		},
		{
			name:          "unknown extension fallback",
			mediaPath:     "/tmp/archive.weird",
			wantMediaType: whatsmeow.MediaDocument,
			wantMimeType:  "application/octet-stream",
		},
		{
			name:          "no extension fallback",
			mediaPath:     "/tmp/readme",
			wantMediaType: whatsmeow.MediaDocument,
			wantMimeType:  "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMediaType, gotMimeType := mediaTypeAndMimeType(tt.mediaPath)
			if gotMediaType != tt.wantMediaType {
				t.Fatalf("mediaTypeAndMimeType() mediaType = %v, want %v", gotMediaType, tt.wantMediaType)
			}
			if gotMimeType != tt.wantMimeType {
				t.Fatalf("mediaTypeAndMimeType() mimeType = %q, want %q", gotMimeType, tt.wantMimeType)
			}
		})
	}
}

func TestDocumentFileName(t *testing.T) {
	tests := []struct {
		name      string
		mediaPath string
		want      string
	}{
		{
			name:      "unix path",
			mediaPath: "/tmp/Presidio-statement-May-2026.pdf",
			want:      "Presidio-statement-May-2026.pdf",
		},
		{
			name:      "windows path",
			mediaPath: `C:\tmp\Quarterly Report.xlsx`,
			want:      "Quarterly Report.xlsx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := documentFileName(tt.mediaPath)
			if got != tt.want {
				t.Fatalf("documentFileName() = %q, want %q", got, tt.want)
			}
		})
	}
}
