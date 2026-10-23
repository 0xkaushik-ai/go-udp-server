package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

// quietLogger discards log output so test results stay readable.
func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestHandle(t *testing.T) {
	s := New(quietLogger())
	s.now = func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) }

	tests := []struct{ in, want string }{
		{"PING", "PONG"},
		{"ping\n", "PONG"},
		{"ECHO hello world", "hello world"},
		{"upper go is fun", "GO IS FUN"},
		{"TIME", "2026-10-23T12:00:00Z"},
		{"", "ERR empty request"},
		{"FLY", "ERR unknown command: FLY"},
	}
	for _, tc := range tests {
		if got := s.Handle(tc.in); got != tc.want {
			t.Errorf("Handle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestServeOverUDP starts a real server on a random local port and talks to
// it with a real UDP socket.
func TestServeOverUDP(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0") // port 0 = pick a free port
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- New(quietLogger()).Serve(ctx, conn) }()

	client, err := net.Dial("udp", conn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	send := func(msg string) string {
		t.Helper()
		if _, err := client.Write([]byte(msg)); err != nil {
			t.Fatal(err)
		}
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, MaxPacketSize)
		n, err := client.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		return string(buf[:n])
	}

	if got := send("PING"); got != "PONG" {
		t.Errorf("PING -> %q, want PONG", got)
	}
	if got := send("ECHO udp"); got != "udp" {
		t.Errorf("ECHO -> %q, want udp", got)
	}
	if got := send("STATS"); !strings.Contains(got, "packets_in=3") || !strings.Contains(got, "clients=1") {
		t.Errorf("STATS -> %q, want packets_in=3 and clients=1", got)
	}

	// Cancelling the context must stop Serve cleanly.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not stop after cancel")
	}
}
