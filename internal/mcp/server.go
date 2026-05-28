package mcp

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/ingest"
	"github.com/sausheong/wadb/internal/mcp/tools"
	"github.com/sausheong/wadb/internal/media"
	"github.com/sausheong/wadb/internal/waclient"
)

// Server bundles the MCP server with everything it needs.
type Server struct {
	srv *server.MCPServer
	log *slog.Logger
}

// New wires every tool. Add tools here as they're implemented.
func New(q *db.Queries, c waclient.Client, ing *ingest.Ingester, mediaDir string, log *slog.Logger) *Server {
	s := server.NewMCPServer("wadb", "0.1.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)
	cache := media.NewCache(mediaDir)

	s.AddTool(tools.StatusTool(), server.ToolHandlerFunc(tools.NewStatusHandler(q, c, ing)))
	s.AddTool(tools.ListChatsTool(), server.ToolHandlerFunc(tools.NewListChatsHandler(q)))
	s.AddTool(tools.ListContactsTool(), server.ToolHandlerFunc(tools.NewListContactsHandler(q)))
	s.AddTool(tools.ListGroupsTool(), server.ToolHandlerFunc(tools.NewListGroupsHandler(q)))
	s.AddTool(tools.GetChatTool(), server.ToolHandlerFunc(tools.NewGetChatHandler(q)))
	s.AddTool(tools.GetMessagesTool(), server.ToolHandlerFunc(tools.NewGetMessagesHandler(q)))
	s.AddTool(tools.SearchMessagesTool(), server.ToolHandlerFunc(tools.NewSearchMessagesHandler(q)))
	s.AddTool(tools.GetMessageTool(), server.ToolHandlerFunc(tools.NewGetMessageHandler(q)))
	s.AddTool(tools.SendTextTool(), server.ToolHandlerFunc(tools.NewSendTextHandler(q, c)))
	s.AddTool(tools.SendMediaTool(), server.ToolHandlerFunc(tools.NewSendMediaHandler(q, c)))
	s.AddTool(tools.ReactTool(), server.ToolHandlerFunc(tools.NewReactHandler(q, c)))
	s.AddTool(tools.MarkReadTool(), server.ToolHandlerFunc(tools.NewMarkReadHandler(q, c)))
	s.AddTool(tools.DownloadMediaTool(), server.ToolHandlerFunc(tools.NewDownloadMediaHandler(q, c, cache)))

	return &Server{srv: s, log: log}
}

// Serve blocks running the stdio MCP server until ctx is cancelled or stdin closes.
func (s *Server) Serve(ctx context.Context) error {
	return server.ServeStdio(s.srv)
}

// Underlying returns the wrapped *server.MCPServer for in-process tests.
func (s *Server) Underlying() *server.MCPServer { return s.srv }
