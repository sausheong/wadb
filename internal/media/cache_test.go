package media

import (
	"path/filepath"
	"testing"
)

func TestCache_Write_ByContentHash(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir)
	bytes := []byte("hello world")
	path, sha, err := c.Write(bytes, "image/png")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("path outside dir: %s", path)
	}
	if sha == "" {
		t.Error("empty sha")
	}
	// Second write of same bytes returns the same path (dedup).
	path2, sha2, err := c.Write(bytes, "image/png")
	if err != nil {
		t.Fatalf("re-write: %v", err)
	}
	if path != path2 || sha != sha2 {
		t.Errorf("expected dedup, got %q vs %q", path, path2)
	}
}

func TestCache_ExtensionFromMIME(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir)
	cases := []struct {
		mime, wantExt string
	}{
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"video/mp4", ".mp4"},
		{"application/pdf", ".pdf"},
		{"application/octet-stream", ".bin"},
		{"weird/unknown", ".bin"},
	}
	for _, c2 := range cases {
		path, _, err := c.Write([]byte(c2.mime), c2.mime)
		if err != nil {
			t.Fatalf("write %s: %v", c2.mime, err)
		}
		if filepath.Ext(path) != c2.wantExt {
			t.Errorf("mime %q -> ext %q, want %q", c2.mime, filepath.Ext(path), c2.wantExt)
		}
	}
}
