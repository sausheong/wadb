package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mdp/qrterminal/v3"

	"github.com/sausheong/wadb/internal/config"
	"github.com/sausheong/wadb/internal/waclient"
)

// runLink pairs the local session DB with a WhatsApp account by displaying
// a QR code in the terminal. Exits 0 on successful pairing, non-zero on
// error or timeout.
func runLink(_ []string) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := waclient.NewWhatsmeow(ctx, cfg.SessionDB)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open session:", err)
		return 1
	}
	cli := client.Underlying()
	defer cli.Disconnect()

	if cli.Store.ID != nil {
		fmt.Fprintln(os.Stderr, "already paired:", cli.Store.ID.String())
		return 0
	}

	qrCh, err := cli.GetQRChannel(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get QR channel:", err)
		return 1
	}
	if err := cli.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		return 1
	}

	for evt := range qrCh {
		switch evt.Event {
		case "code":
			fmt.Fprintln(os.Stderr, "Scan this QR code from WhatsApp -> Linked Devices:")
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stderr)
		case "success":
			id := ""
			if cli.Store.ID != nil {
				id = cli.Store.ID.String()
			}
			fmt.Fprintln(os.Stderr, "paired:", id)
			return 0
		case "timeout":
			fmt.Fprintln(os.Stderr, "QR timed out - re-run `wadb link`.")
			return 1
		case "err-client-outdated":
			fmt.Fprintln(os.Stderr, "whatsmeow is outdated; update the dependency.")
			return 1
		case "error":
			fmt.Fprintln(os.Stderr, "pairing error:", evt.Error)
			return 1
		default:
			fmt.Fprintln(os.Stderr, "qr event:", evt.Event)
		}
	}
	return 1
}
