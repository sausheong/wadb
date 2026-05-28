package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sausheong/wadb/internal/config"
	"github.com/sausheong/wadb/internal/db"
	"github.com/sausheong/wadb/internal/macimport"
)

// runImportHistory copies messages, contacts, groups, and media metadata
// from the macOS WhatsApp Desktop app's local SQLite into wadb.db. The
// import is idempotent — re-running is safe and only adds new rows.
func runImportHistory(args []string) int {
	fs := flag.NewFlagSet("import-history", flag.ContinueOnError)
	srcDefault := defaultMacChatStoragePath()
	source := fs.String("source", srcDefault, "Path to ChatStorage.sqlite (macOS WhatsApp app)")
	dryRun := fs.Bool("dry-run", false, "Read and parse but don't write to wadb.db")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	if _, err := os.Stat(*source); err != nil {
		fmt.Fprintf(os.Stderr, "source not found: %s\n", *source)
		fmt.Fprintln(os.Stderr, "  pass --source to specify a different path.")
		return 1
	}

	srcInfo, _ := os.Stat(*source)
	fmt.Fprintf(os.Stderr, "Source: %s (%s)\n", *source, humanBytes(srcInfo.Size()))
	fmt.Fprintf(os.Stderr, "Target: %s\n", cfg.AppDB)

	if *dryRun {
		fmt.Fprintln(os.Stderr, "Dry run: no writes will be performed.")
		return 0
	}

	appDB, err := db.Open(cfg.AppDB)
	if err != nil {
		logger.Error("open app db", "err", err)
		return 2
	}
	defer appDB.Close()
	if err := db.Migrate(context.Background(), appDB); err != nil {
		logger.Error("migrate", "err", err)
		return 2
	}
	queries := db.NewQueries(appDB)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	imp, err := macimport.New(*source, queries)
	if err != nil {
		logger.Error("open source", "err", err)
		return 1
	}
	defer imp.Close()
	imp.SetLogger(logger)

	start := time.Now()
	stats, err := imp.Import(ctx)
	elapsed := time.Since(start)
	if err != nil {
		logger.Error("import", "err", err)
		fmt.Fprintf(os.Stderr, "\nImport failed after %s.\n", elapsed.Round(time.Millisecond))
		return 2
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Imported:")
	fmt.Fprintf(os.Stderr, "  contacts ........... %d\n", stats.Contacts)
	fmt.Fprintf(os.Stderr, "  chats .............. %d\n", stats.Chats)
	fmt.Fprintf(os.Stderr, "  groups ............. %d\n", stats.Groups)
	fmt.Fprintf(os.Stderr, "  participants ....... %d\n", stats.Participants)
	fmt.Fprintf(os.Stderr, "  messages ........... %d (skipped %d)\n", stats.Messages, stats.SkippedMessages)
	fmt.Fprintf(os.Stderr, "  media metadata ..... %d\n", stats.Media)
	if stats.Errors > 0 {
		fmt.Fprintf(os.Stderr, "  errors ............. %d (logged at warn)\n", stats.Errors)
	}
	fmt.Fprintf(os.Stderr, "\nFinished in %s.\n", elapsed.Round(time.Millisecond))
	return 0
}

func defaultMacChatStoragePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home,
		"Library", "Group Containers",
		"group.net.whatsapp.WhatsApp.shared", "ChatStorage.sqlite")
}

func humanBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.0f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(KB))
	}
	return fmt.Sprintf("%d B", n)
}
