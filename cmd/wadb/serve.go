package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sausheong/wadb/internal/config"
	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/ingest"
	"github.com/sausheong/wadb/internal/mcp"
	"github.com/sausheong/wadb/internal/waclient"
)

func runServe(_ []string) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}

	// IMPORTANT: configure slog BEFORE constructing the whatsmeow client.
	// Task 11's waclient log adapter captures slog.Default() at construction
	// time, so the default must already write to stderr. stdout is reserved
	// for the MCP JSON-RPC transport — any stray write breaks the protocol.
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	appDB, err := db.Open(cfg.AppDB)
	if err != nil {
		logger.Error("open app db", "err", err)
		return 1
	}
	defer appDB.Close()
	if err := db.Migrate(context.Background(), appDB); err != nil {
		logger.Error("migrate", "err", err)
		return 1
	}
	queries := db.NewQueries(appDB)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := waclient.NewWhatsmeow(ctx, cfg.SessionDB)
	if err != nil {
		logger.Error("open session", "err", err)
		return 1
	}
	if err := client.Connect(ctx); err != nil {
		logger.Error("connect", "err", err)
		return 1
	}
	defer client.Disconnect()

	ing := ingest.New(queries, client)
	ing.SetLogger(logger)
	ingestCtx, ingestStop := context.WithCancel(ctx)
	defer ingestStop()
	go ing.Run(ingestCtx)

	srv := mcp.New(queries, client, ing, cfg.MediaDir, logger)
	logger.Info("wadb serve ready", "device", client.DeviceJID(), "home", cfg.Home)
	if err := srv.Serve(ctx); err != nil {
		logger.Error("mcp serve", "err", err)
		return 1
	}
	logger.Info("wadb serve exiting")
	return 0
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	// stdout is reserved for the MCP transport.
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
