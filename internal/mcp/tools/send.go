package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/waclient"
)

func SendTextTool() mcpgo.Tool {
	return mcpgo.NewTool("send_text",
		mcpgo.WithDescription("Send a text message, optionally as a reply."),
		mcpgo.WithString("chat_jid", mcpgo.Required()),
		mcpgo.WithString("text", mcpgo.Required()),
		mcpgo.WithString("reply_to_id", mcpgo.Description("Message ID to reply to (optional).")),
	)
}

func NewSendTextHandler(_ *db.Queries, c waclient.Client) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		if err := validateRequired(args, "chat_jid", "text"); err != nil {
			return errResult(err.Error()), nil
		}
		res, err := c.SendText(ctx, argStr(args, "chat_jid"), argStr(args, "text"), argStr(args, "reply_to_id"))
		if err != nil {
			return waErrResult(err), nil
		}
		return jsonResult(map[string]any{
			"message_id": res.MessageID,
			"timestamp":  res.Timestamp.Unix(),
		}), nil
	}
}

func SendMediaTool() mcpgo.Tool {
	return mcpgo.NewTool("send_media",
		mcpgo.WithDescription("Send a media file (image/video/document/audio). Kind inferred from MIME type."),
		mcpgo.WithString("chat_jid", mcpgo.Required()),
		mcpgo.WithString("local_path", mcpgo.Required(), mcpgo.Description("Absolute path to the file to send.")),
		mcpgo.WithString("caption", mcpgo.Description("Optional caption.")),
		mcpgo.WithString("reply_to_id", mcpgo.Description("Message ID to reply to (optional).")),
	)
}

func NewSendMediaHandler(_ *db.Queries, c waclient.Client) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		if err := validateRequired(args, "chat_jid", "local_path"); err != nil {
			return errResult(err.Error()), nil
		}
		path := argStr(args, "local_path")
		if !filepath.IsAbs(path) {
			return errResult("local_path must be absolute"), nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return errResult("read local_path: " + err.Error()), nil
		}
		mime := mimeFromPath(path)
		kind := mediaKindForMIME(mime)
		if kind == "" {
			return errResult("unsupported MIME type: " + mime), nil
		}
		res, err := c.SendMedia(ctx, argStr(args, "chat_jid"), kind, data, mime,
			argStr(args, "caption"), argStr(args, "reply_to_id"))
		if err != nil {
			return waErrResult(err), nil
		}
		return jsonResult(map[string]any{
			"message_id": res.MessageID,
			"timestamp":  res.Timestamp.Unix(),
		}), nil
	}
}

func ReactTool() mcpgo.Tool {
	return mcpgo.NewTool("react",
		mcpgo.WithDescription("React to a message with an emoji. Empty emoji removes your reaction."),
		mcpgo.WithString("chat_jid", mcpgo.Required()),
		mcpgo.WithString("message_id", mcpgo.Required()),
		mcpgo.WithString("emoji", mcpgo.Required(), mcpgo.Description("Emoji character, or empty string to remove.")),
	)
}

func NewReactHandler(_ *db.Queries, c waclient.Client) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		// emoji is required-key but the VALUE can legitimately be empty (means remove).
		if _, ok := args["chat_jid"]; !ok {
			return errResult("missing required argument: chat_jid"), nil
		}
		if _, ok := args["message_id"]; !ok {
			return errResult("missing required argument: message_id"), nil
		}
		if _, ok := args["emoji"]; !ok {
			return errResult("missing required argument: emoji"), nil
		}
		if err := c.React(ctx, argStr(args, "chat_jid"), argStr(args, "message_id"), argStr(args, "emoji")); err != nil {
			return waErrResult(err), nil
		}
		return jsonResult(map[string]any{"ok": true}), nil
	}
}

func MarkReadTool() mcpgo.Tool {
	return mcpgo.NewTool("mark_read",
		mcpgo.WithDescription("Mark a chat read up to a message (or latest if up_to_message_id omitted)."),
		mcpgo.WithString("chat_jid", mcpgo.Required()),
		mcpgo.WithString("up_to_message_id", mcpgo.Description("Optional message ID upper bound.")),
	)
}

func NewMarkReadHandler(_ *db.Queries, c waclient.Client) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		if err := validateRequired(args, "chat_jid"); err != nil {
			return errResult(err.Error()), nil
		}
		if err := c.MarkRead(ctx, argStr(args, "chat_jid"), argStr(args, "up_to_message_id")); err != nil {
			return waErrResult(err), nil
		}
		return jsonResult(map[string]any{"ok": true}), nil
	}
}

// --- error envelope ---

// waErrResult wraps a Client error as an MCP error result with a JSON body
// of {"error": "...", "retryable": bool} so callers can decide whether to retry.
func waErrResult(err error) *mcpgo.CallToolResult {
	b, _ := json.Marshal(map[string]any{"error": err.Error(), "retryable": isRetryable(err)})
	return &mcpgo.CallToolResult{
		IsError: true,
		Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: string(b)}},
	}
}

// isRetryable is a coarse classifier: network/rate-limit/timeout errors are
// considered retryable; validation/permission errors are not.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"rate", "timeout", "deadline", "connection", "temporar", "unavailable"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return errors.Is(err, waclient.ErrNotConnected)
}

// --- mime inference ---

func mimeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func mediaKindForMIME(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case strings.HasPrefix(mime, "application/"):
		return "document"
	}
	return ""
}
