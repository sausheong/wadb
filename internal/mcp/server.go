package mcp

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/ingest"
	"github.com/sausheong/wadb/internal/mcp/tools"
	"github.com/sausheong/wadb/internal/waclient"
)

// Server bundles the MCP server with everything it needs.
type Server struct {
	srv *server.MCPServer
	log *slog.Logger
}

// New wires every tool. Add tools here as they're implemented.
func New(q *db.Queries, c waclient.Client, ing *ingest.Ingester, log *slog.Logger) *Server {
	s := server.NewMCPServer("wadb", "0.1.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)
	s.AddTool(tools.StatusTool(), server.ToolHandlerFunc(tools.NewStatusHandler(q, c, ing)))
	return &Server{srv: s, log: log}
}

// Serve blocks running the stdio MCP server until ctx is cancelled or stdin closes.
func (s *Server) Serve(ctx context.Context) error {
	return server.ServeStdio(s.srv)
}
