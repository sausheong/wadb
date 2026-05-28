package tools

import (
	"context"
	"database/sql"
	"errors"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/media"
	"github.com/sausheong/wadb/internal/waclient"
)

func DownloadMediaTool() mcpgo.Tool {
	return mcpgo.NewTool("download_media",
		mcpgo.WithDescription("Fetch the binary blob for a media message; decrypt; cache locally."),
		mcpgo.WithString("chat_jid", mcpgo.Required()),
		mcpgo.WithString("message_id", mcpgo.Required()),
	)
}

func NewDownloadMediaHandler(q *db.Queries, c waclient.Client, cache *media.Cache) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		if err := validateRequired(args, "chat_jid", "message_id"); err != nil {
			return errResult(err.Error()), nil
		}
		chatJID := argStr(args, "chat_jid")
		msgID := argStr(args, "message_id")
		m, err := q.MediaForMessage(ctx, chatJID, msgID)
		if err == sql.ErrNoRows {
			return waErrResult(errors.New("no media for that message")), nil
		}
		if err != nil {
			return errResult("media lookup: " + err.Error()), nil
		}
		if m.LocalPath != "" {
			return jsonResult(map[string]any{
				"local_path": m.LocalPath, "mime_type": m.MimeType, "size": m.Size,
			}), nil
		}
		dl, err := c.Download(ctx, m.DownloadRef)
		if err != nil {
			return waErrResult(err), nil
		}
		path, sha, err := cache.Write(dl.Bytes, dl.MimeType)
		if err != nil {
			return errResult("write cache: " + err.Error()), nil
		}
		now := time.Now().Unix()
		if err := q.RecordMediaDownload(ctx, chatJID, msgID, path, sha, int64(len(dl.Bytes)), now); err != nil {
			return errResult("record download: " + err.Error()), nil
		}
		return jsonResult(map[string]any{
			"local_path": path, "mime_type": dl.MimeType, "size": int64(len(dl.Bytes)),
		}), nil
	}
}
