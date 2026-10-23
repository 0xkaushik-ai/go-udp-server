// Command server runs the UDP command server.
//
// Usage:
//
//	go run ./cmd/server [-addr :9000]
//
// Stop it with Ctrl+C. See README.md for the protocol.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"udpserver/internal/server"
)

func main() {
	addr := flag.String("addr", ":9000", "UDP address to listen on (host:port)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// ListenPacket opens a UDP socket. Unlike TCP's Listen there is no
	// Accept step: every client's packets arrive on this one socket.
	conn, err := net.ListenPacket("udp", *addr)
	if err != nil {
		log.Error("listen failed", "addr", *addr, "error", err)
		os.Exit(1)
	}

	// Ctrl+C or SIGTERM cancels ctx, which makes Serve close the socket and return.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.New(log).Serve(ctx, conn); err != nil {
		log.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
	log.Info("server stopped")
}
