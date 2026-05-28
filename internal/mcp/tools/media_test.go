package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/media"
	"github.com/sausheong/wadb/internal/waclient"
)

func TestDownloadMedia_FetchAndCache(t *testing.T) {
	q := seedDB(t)
	ctx := context.Background()
	if err := q.UpsertContact(ctx, db.Contact{JID: "s@s.whatsapp.net", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertChat(ctx, db.Chat{JID: "s@s.whatsapp.net", Kind: "dm"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertMessage(ctx, db.Message{
		ID: "M1", ChatJID: "s@s.whatsapp.net", SenderJID: "s@s.whatsapp.net",
		Timestamp: int64(time.Now().Unix()), Kind: "image",
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertMedia(ctx, db.Media{
		MessageChatJID: "s@s.whatsapp.net", MessageID: "M1",
		MimeType: "image/png", DownloadRef: "ref-xyz",
	}); err != nil {
		t.Fatal(err)
	}

	fake := waclient.NewFake()
	fake.DownloadFn = func(ref string) (waclient.DownloadResult, error) {
		return waclient.DownloadResult{Bytes: []byte("png-bytes"), MimeType: "image/png"}, nil
	}
	cache := media.NewCache(filepath.Join(t.TempDir(), "media"))
	h := NewDownloadMediaHandler(q, fake, cache)
	res, err := h(ctx, callReq(map[string]any{"chat_jid": "s@s.whatsapp.net", "message_id": "M1"}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.IsError {
		t.Fatalf("isError: %+v", res)
	}
	var out struct {
		LocalPath string `json:"local_path"`
		MimeType  string `json:"mime_type"`
		Size      int64  `json:"size"`
	}
	json.Unmarshal([]byte(firstTextContent(t, res)), &out)
	if out.LocalPath == "" {
		t.Fatal("LocalPath empty")
	}
	if _, err := os.Stat(out.LocalPath); err != nil {
		t.Errorf("file not on disk: %v", err)
	}
	if out.MimeType != "image/png" || out.Size != int64(len("png-bytes")) {
		t.Errorf("metadata wrong: %+v", out)
	}
	// Second call returns the cached path without re-invoking download.
	called := 0
	fake.DownloadFn = func(_ string) (waclient.DownloadResult, error) {
		called++
		return waclient.DownloadResult{}, nil
	}
	res2, _ := h(ctx, callReq(map[string]any{"chat_jid": "s@s.whatsapp.net", "message_id": "M1"}))
	var out2 struct {
		LocalPath string `json:"local_path"`
	}
	json.Unmarshal([]byte(firstTextContent(t, res2)), &out2)
	if out2.LocalPath != out.LocalPath {
		t.Errorf("path changed: %q -> %q", out.LocalPath, out2.LocalPath)
	}
	if called != 0 {
		t.Errorf("downloader was called %d times on cached path", called)
	}
}

func TestDownloadMedia_NoMediaRow_IsRetryableEnvelope(t *testing.T) {
	q := seedDB(t)
	ctx := context.Background()
	fake := waclient.NewFake()
	cache := media.NewCache(filepath.Join(t.TempDir(), "media"))
	h := NewDownloadMediaHandler(q, fake, cache)
	res, _ := h(ctx, callReq(map[string]any{"chat_jid": "x@s.whatsapp.net", "message_id": "M1"}))
	if !res.IsError {
		t.Fatal("expected error")
	}
	var got struct {
		Error     string `json:"error"`
		Retryable bool   `json:"retryable"`
	}
	if err := json.Unmarshal([]byte(firstTextContent(t, res)), &got); err != nil {
		t.Fatalf("envelope not JSON: %v", err)
	}
	if got.Error == "" {
		t.Error("error field empty")
	}
	if got.Retryable {
		t.Error("no-media-row should not be retryable")
	}
}
