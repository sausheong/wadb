package tools

import (
	"context"
	"database/sql"
	"encoding/json"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/sausheong/wadb/internal/db"
)

func GetMessagesTool() mcpgo.Tool {
	return mcpgo.NewTool("get_messages",
		mcpgo.WithDescription("Page through messages in a chat, newest first."),
		mcpgo.WithString("chat_jid", mcpgo.Required()),
		mcpgo.WithString("cursor", mcpgo.Description("Opaque cursor from previous response.")),
		mcpgo.WithNumber("limit", mcpgo.Description("Default 50, max 500.")),
	)
}

func NewGetMessagesHandler(q *db.Queries) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		if err := validateRequired(args, "chat_jid"); err != nil {
			return errResult(err.Error()), nil
		}
		limit := argInt(args, "limit", 50)
		if limit <= 0 || limit > 500 {
			limit = 50
		}
		var beforeTs int64
		var beforeID string
		if c := argStr(args, "cursor"); c != "" {
			cur, err := decodeCursorImpl(c)
			if err != nil {
				return errResult("invalid cursor: " + err.Error()), nil
			}
			beforeTs, beforeID = cur.Ts, cur.ID
		}
		msgs, err := q.GetMessages(ctx, argStr(args, "chat_jid"), beforeTs, beforeID, limit)
		if err != nil {
			return errResult("get_messages: " + err.Error()), nil
		}
		out := make([]map[string]any, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, messageToMap(m))
		}
		var next string
		if len(msgs) == limit {
			last := msgs[len(msgs)-1]
			next = encodeCursorImpl(last.Timestamp, last.ID)
		}
		return jsonResult(map[string]any{"messages": out, "next_cursor": next}), nil
	}
}

func SearchMessagesTool() mcpgo.Tool {
	return mcpgo.NewTool("search_messages",
		mcpgo.WithDescription("Full-text search across messages.text/captions."),
		mcpgo.WithString("query", mcpgo.Required()),
		mcpgo.WithString("chat_jid", mcpgo.Description("Limit to one chat.")),
		mcpgo.WithString("sender_jid", mcpgo.Description("Limit to messages from one sender.")),
		mcpgo.WithNumber("since", mcpgo.Description("Unix seconds lower bound.")),
		mcpgo.WithNumber("until", mcpgo.Description("Unix seconds upper bound.")),
		mcpgo.WithNumber("limit", mcpgo.Description("Default 50, max 500.")),
	)
}

func NewSearchMessagesHandler(q *db.Queries) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		if err := validateRequired(args, "query"); err != nil {
			return errResult(err.Error()), nil
		}
		limit := argInt(args, "limit", 50)
		if limit <= 0 || limit > 500 {
			limit = 50
		}
		msgs, err := q.SearchMessages(ctx,
			argStr(args, "query"),
			argStr(args, "chat_jid"),
			argStr(args, "sender_jid"),
			argInt64(args, "since"),
			argInt64(args, "until"),
			limit)
		if err != nil {
			return errResult("search_messages: " + err.Error()), nil
		}
		out := make([]map[string]any, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, messageToMap(m))
		}
		return jsonResult(map[string]any{"messages": out}), nil
	}
}

func GetMessageTool() mcpgo.Tool {
	return mcpgo.NewTool("get_message",
		mcpgo.WithDescription("Fetch one message with reactions, quoted message (1 level), and media metadata."),
		mcpgo.WithString("chat_jid", mcpgo.Required()),
		mcpgo.WithString("id", mcpgo.Required()),
	)
}

func NewGetMessageHandler(q *db.Queries) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		if err := validateRequired(args, "chat_jid", "id"); err != nil {
			return errResult(err.Error()), nil
		}
		chatJID := argStr(args, "chat_jid")
		id := argStr(args, "id")
		m, err := q.GetMessage(ctx, chatJID, id)
		if err == sql.ErrNoRows {
			return jsonResult(nil), nil
		}
		if err != nil {
			return errResult("get_message: " + err.Error()), nil
		}
		out := messageToMap(m)
		if m.QuotedID != "" {
			if qm, err := q.GetMessage(ctx, chatJID, m.QuotedID); err == nil {
				out["quoted"] = messageToMap(qm)
			}
		}
		if media, err := q.MediaForMessage(ctx, chatJID, id); err == nil {
			out["media"] = map[string]any{
				"mime_type":     media.MimeType,
				"size":          media.Size,
				"width":         media.Width,
				"height":        media.Height,
				"duration_sec":  media.DurationSec,
				"local_path":    media.LocalPath,
				"downloaded_at": media.DownloadedAt,
			}
		}
		return jsonResult(out), nil
	}
}

func messageToMap(m db.Message) map[string]any {
	var text any
	if m.Text != nil {
		text = *m.Text
	}
	var reactions any
	if m.Reactions != "" {
		var arr any
		_ = json.Unmarshal([]byte(m.Reactions), &arr)
		reactions = arr
	}
	return map[string]any{
		"id":         m.ID,
		"chat_jid":   m.ChatJID,
		"sender_jid": m.SenderJID,
		"from_me":    m.FromMe,
		"timestamp":  m.Timestamp,
		"kind":       m.Kind,
		"text":       text,
		"quoted_id":  m.QuotedID,
		"reactions":  reactions,
		"edited_at":  m.EditedAt,
		"deleted_at": m.DeletedAt,
	}
}
