// Package media handles downloaded-media caching.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

type Cache struct{ dir string }

func NewCache(dir string) *Cache { return &Cache{dir: dir} }

// Write stores data under <dir>/<sha256>.<ext>. Returns the absolute path,
// hex-sha256, and any error. Idempotent — if the file exists already, no
// re-write happens.
func (c *Cache) Write(data []byte, mime string) (path string, sha string, err error) {
	sum := sha256.Sum256(data)
	sha = hex.EncodeToString(sum[:])
	name := sha + extForMIME(mime)
	path = filepath.Join(c.dir, name)
	if _, err := os.Stat(path); err == nil {
		return path, sha, nil
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", "", err
	}
	return path, sha, nil
}

func extForMIME(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mime, "image/png"):
		return ".png"
	case strings.HasPrefix(mime, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mime, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mime, "video/mp4"):
		return ".mp4"
	case strings.HasPrefix(mime, "video/quicktime"):
		return ".mov"
	case strings.HasPrefix(mime, "video/webm"):
		return ".webm"
	case strings.HasPrefix(mime, "audio/mpeg"):
		return ".mp3"
	case strings.HasPrefix(mime, "audio/ogg"):
		return ".ogg"
	case mime == "application/pdf":
		return ".pdf"
	}
	return ".bin"
}
