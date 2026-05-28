package waclient

import (
	"context"
	"fmt"
	"log/slog"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// slogAdapter routes whatsmeow log calls through slog. wadb's slog handler
// writes to stderr (see cmd/wadb/serve.go); critically, NOTHING in this
// process writes to stdout except the MCP JSON-RPC transport, so the
// whatsmeow library's logger must never use waLog.Stdout(...).
type slogAdapter struct {
	logger *slog.Logger
	module string
}

func newSlogAdapter(module string) waLog.Logger {
	return &slogAdapter{logger: slog.Default(), module: module}
}

func (a *slogAdapter) log(level slog.Level, msg string, args []any) {
	// Pre-format the args into the message because whatsmeow uses Printf-style.
	text := msg
	if len(args) > 0 {
		text = fmt.Sprintf(msg, args...)
	}
	a.logger.LogAttrs(context.Background(), level, text, slog.String("mod", a.module))
}

func (a *slogAdapter) Warnf(msg string, args ...interface{})  { a.log(slog.LevelWarn, msg, args) }
func (a *slogAdapter) Errorf(msg string, args ...interface{}) { a.log(slog.LevelError, msg, args) }
func (a *slogAdapter) Infof(msg string, args ...interface{})  { a.log(slog.LevelInfo, msg, args) }
func (a *slogAdapter) Debugf(msg string, args ...interface{}) { a.log(slog.LevelDebug, msg, args) }
func (a *slogAdapter) Sub(module string) waLog.Logger {
	return &slogAdapter{logger: a.logger, module: a.module + "/" + module}
}
