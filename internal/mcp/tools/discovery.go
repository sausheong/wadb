package tools

import (
	"context"
	"database/sql"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/sausheong/wadb/internal/db"
)

func ListChatsTool() mcpgo.Tool {
	return mcpgo.NewTool("list_chats",
		mcpgo.WithDescription("List recent chats, newest activity first."),
		mcpgo.WithString("kind", mcpgo.Description("Filter by 'dm' or 'group'. Empty = both.")),
		mcpgo.WithNumber("limit", mcpgo.Description("Max results (default 50).")),
		mcpgo.WithString("cursor", mcpgo.Description("Opaque cursor from a previous response.")),
	)
}

func NewListChatsHandler(q *db.Queries) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		kind := argStr(args, "kind")
		if kind != "" && kind != "dm" && kind != "group" {
			return errResult("kind must be 'dm', 'group', or empty"), nil
		}
		limit := clampLimit(argInt(args, "limit", 50))
		var beforeTs int64
		var beforeID string
		if c := argStr(args, "cursor"); c != "" {
			cur, err := decodeCursorImpl(c)
			if err != nil {
				return errResult("invalid cursor: " + err.Error()), nil
			}
			beforeTs, beforeID = cur.Ts, cur.ID
		}
		chats, err := q.ListChats(ctx, kind, beforeTs, beforeID, limit)
		if err != nil {
			return errResult("list_chats: " + err.Error()), nil
		}
		out := make([]map[string]any, 0, len(chats))
		var nextCursor string
		for _, c := range chats {
			out = append(out, map[string]any{
				"jid":             c.JID,
				"kind":            c.Kind,
				"last_message_at": c.LastMessageAt,
				"unread_count":    c.UnreadCount,
				"archived":        c.Archived,
				"pinned":          c.Pinned,
			})
		}
		if len(chats) == limit {
			last := chats[len(chats)-1]
			nextCursor = encodeCursorImpl(last.LastMessageAt, last.JID)
		}
		return jsonResult(map[string]any{"chats": out, "next_cursor": nextCursor}), nil
	}
}

func ListContactsTool() mcpgo.Tool {
	return mcpgo.NewTool("list_contacts",
		mcpgo.WithDescription("Search contacts by name/business name/phone substring."),
		mcpgo.WithString("query", mcpgo.Description("Substring to match (empty = all).")),
		mcpgo.WithNumber("limit", mcpgo.Description("Max results (default 50).")),
	)
}

func NewListContactsHandler(q *db.Queries) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		contacts, err := q.SearchContacts(ctx, argStr(args, "query"), clampLimit(argInt(args, "limit", 50)))
		if err != nil {
			return errResult("list_contacts: " + err.Error()), nil
		}
		out := make([]map[string]any, 0, len(contacts))
		for _, c := range contacts {
			out = append(out, map[string]any{
				"jid":           c.JID,
				"push_name":     c.PushName,
				"business_name": c.BusinessName,
				"phone":         c.Phone,
				"is_blocked":    c.IsBlocked,
			})
		}
		return jsonResult(map[string]any{"contacts": out}), nil
	}
}

func ListGroupsTool() mcpgo.Tool {
	return mcpgo.NewTool("list_groups",
		mcpgo.WithDescription("List groups you're in, optionally filtered by name substring."),
		mcpgo.WithString("query", mcpgo.Description("Name substring (empty = all).")),
		mcpgo.WithNumber("limit", mcpgo.Description("Max results (default 50).")),
	)
}

func NewListGroupsHandler(q *db.Queries) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		groups, err := q.SearchGroups(ctx, argStr(args, "query"), clampLimit(argInt(args, "limit", 50)))
		if err != nil {
			return errResult("list_groups: " + err.Error()), nil
		}
		out := make([]map[string]any, 0, len(groups))
		for _, g := range groups {
			out = append(out, map[string]any{
				"jid":   g.JID,
				"name":  g.Name,
				"topic": g.Topic,
			})
		}
		return jsonResult(map[string]any{"groups": out}), nil
	}
}

func GetChatTool() mcpgo.Tool {
	return mcpgo.NewTool("get_chat",
		mcpgo.WithDescription("Full chat detail. For groups: includes participants and admin flags."),
		mcpgo.WithString("jid", mcpgo.Required(), mcpgo.Description("Chat JID.")),
	)
}

func NewGetChatHandler(q *db.Queries) ToolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		if err := validateRequired(args, "jid"); err != nil {
			return errResult(err.Error()), nil
		}
		jid := argStr(args, "jid")
		chat, err := q.GetChat(ctx, jid)
		if err == sql.ErrNoRows {
			return jsonResult(nil), nil
		}
		if err != nil {
			return errResult("get_chat: " + err.Error()), nil
		}
		out := map[string]any{
			"jid":             chat.JID,
			"kind":            chat.Kind,
			"last_message_at": chat.LastMessageAt,
			"unread_count":    chat.UnreadCount,
			"archived":        chat.Archived,
			"pinned":          chat.Pinned,
			"muted_until":     chat.MutedUntil,
		}
		if chat.Kind == "group" {
			parts, _ := q.GetGroupParticipants(ctx, jid)
			pOut := make([]map[string]any, 0, len(parts))
			for _, p := range parts {
				pOut = append(pOut, map[string]any{
					"jid":       p.ContactJID,
					"is_admin":  p.IsAdmin,
					"joined_at": p.JoinedAt,
				})
			}
			out["participants"] = pOut
		}
		return jsonResult(out), nil
	}
}
