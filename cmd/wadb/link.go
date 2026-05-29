package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/sausheong/wadb/internal/config"
	"github.com/sausheong/wadb/internal/waclient"
)

// runLink pairs the local session DB with a WhatsApp account by displaying
// a QR code in the terminal. Exits 0 on successful pairing, non-zero on
// error or timeout.
//
// Pairing is a two-phase handshake:
//  1. The QR channel emits "success" once the QR is accepted on the phone.
//  2. WhatsApp then runs a key exchange + reconnect; the device only
//     appears in Linked Devices after *events.PairSuccess fires AND the
//     subsequent *events.Connected event arrives. Exiting between phases
//     causes the pairing to be silently dropped.
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

	if cli.Store.ID != nil {
		fmt.Fprintln(os.Stderr, "already paired:", cli.Store.ID.String())
		return 0
	}

	// Watch for the post-QR handshake events. PairSuccess fires once the
	// device row is written to the local store; Connected fires once the
	// reconnected socket is authenticated. Both must happen before the
	// device is registered on WhatsApp's side.
	pairedCh := make(chan struct{}, 1)
	connectedCh := make(chan struct{}, 1)
	handler := cli.AddEventHandler(func(ev any) {
		switch ev.(type) {
		case *events.PairSuccess:
			select {
			case pairedCh <- struct{}{}:
			default:
			}
		case *events.Connected:
			select {
			case connectedCh <- struct{}{}:
			default:
			}
		}
	})
	defer cli.RemoveEventHandler(handler)

	qrCh, err := cli.GetQRChannel(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get QR channel:", err)
		return 1
	}
	if err := cli.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		return 1
	}

QRLoop:
	for evt := range qrCh {
		switch evt.Event {
		case "code":
			fmt.Fprintln(os.Stderr, "Scan this QR code from WhatsApp -> Linked Devices:")
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stderr)
		case "success":
			// QR accepted. Wait for the handshake to finish below.
			break QRLoop
		case "timeout":
			fmt.Fprintln(os.Stderr, "QR timed out - re-run `wadb link`.")
			cli.Disconnect()
			return 1
		case "err-client-outdated":
			fmt.Fprintln(os.Stderr, "whatsmeow is outdated; update the dependency.")
			cli.Disconnect()
			return 1
		case "error":
			fmt.Fprintln(os.Stderr, "pairing error:", evt.Error)
			cli.Disconnect()
			return 1
		default:
			fmt.Fprintln(os.Stderr, "qr event:", evt.Event)
		}
	}

	// Wait for PairSuccess + Connected. Generous timeout because WhatsApp
	// occasionally takes several seconds to confirm the device.
	timeout := time.NewTimer(45 * time.Second)
	defer timeout.Stop()

	gotPaired, gotConnected := false, false
	for !(gotPaired && gotConnected) {
		select {
		case <-pairedCh:
			gotPaired = true
			id := ""
			if cli.Store.ID != nil {
				id = cli.Store.ID.String()
			}
			fmt.Fprintln(os.Stderr, "device registered:", id)
		case <-connectedCh:
			gotConnected = true
			fmt.Fprintln(os.Stderr, "authenticated.")
		case <-timeout.C:
			fmt.Fprintln(os.Stderr, "timed out waiting for pairing handshake. Try again.")
			cli.Disconnect()
			return 1
		case <-ctx.Done():
			cli.Disconnect()
			return 1
		}
	}

	// Hold the connection open briefly so any in-flight server messages
	// (initial app-state sync, contact list, etc.) land before we exit.
	// Without this, the device sometimes doesn't appear in Linked Devices.
	fmt.Fprintln(os.Stderr, "finalizing... (this may take a few seconds)")
	select {
	case <-time.After(5 * time.Second):
	case <-ctx.Done():
	}

	cli.Disconnect()
	fmt.Fprintln(os.Stderr, "paired. Check WhatsApp -> Linked Devices to confirm.")
	return 0
}
