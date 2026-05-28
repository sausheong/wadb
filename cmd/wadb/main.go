package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "link":
		os.Exit(runLink(os.Args[2:]))
	case "serve":
		os.Exit(runServe(os.Args[2:]))
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `wadb — WhatsApp MCP server

Usage:
  wadb link            Pair with WhatsApp by scanning a QR code.
  wadb serve           Run the stdio MCP server.

Environment:
  WADB_HOME            Data directory (default: ~/.wadb)
  WADB_LOG_LEVEL       debug|info|warn|error (default: info)`)
}

func runServe(args []string) int { fmt.Fprintln(os.Stderr, "serve: not implemented"); return 1 }
