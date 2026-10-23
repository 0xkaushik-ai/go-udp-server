// Package server implements a small UDP server that answers text commands.
//
// UDP has no connections: every datagram (packet) stands alone and carries
// the sender's address. The server reads a packet, works out a reply and
// sends it back to that address. Nothing guarantees a packet arrives, arrives
// once, or arrives in order, so the protocol is kept stateless: one request
// packet in, one reply packet out.
//
// Protocol (case-insensitive command, then optional text):
//
//	PING          -> PONG
//	ECHO <text>   -> <text>
//	UPPER <text>  -> <TEXT>
//	TIME          -> current server time (RFC 3339, UTC)
//	STATS         -> packet and client counters
//	anything else -> ERR unknown command: <cmd>
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MaxPacketSize is the largest datagram the server will read. 65507 bytes is
// the maximum payload of a UDP packet over IPv4. Anything larger is truncated
// by the operating system.
const MaxPacketSize = 65507

// Stats counts what the server has seen. Fields are atomics so the read loop
// and the STATS command can touch them without a mutex.
type Stats struct {
	PacketsIn  atomic.Uint64
	PacketsOut atomic.Uint64
	BytesIn    atomic.Uint64
	BytesOut   atomic.Uint64
}

// Server answers UDP requests. Create one with New and start it with Serve.
type Server struct {
	log   *slog.Logger
	now   func() time.Time // replaceable in tests
	stats Stats

	mu      sync.Mutex
	clients map[string]struct{} // unique sender addresses seen
}

// New returns a Server that logs to log.
func New(log *slog.Logger) *Server {
	return &Server{
		log:     log,
		now:     time.Now,
		clients: make(map[string]struct{}),
	}
}

// Serve reads packets from conn until ctx is cancelled or conn fails.
// It closes conn on return. A cancelled ctx is a normal shutdown and
// returns nil.
func (s *Server) Serve(ctx context.Context, conn net.PacketConn) error {
	// ReadFrom blocks, so the only way to stop it is to close the socket.
	// When ctx is cancelled, close conn; ReadFrom then returns net.ErrClosed.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	defer conn.Close()

	s.log.Info("udp server listening", "addr", conn.LocalAddr())

	// One buffer is reused for every packet. That is safe because each packet
	// is handled fully before the next ReadFrom call overwrites the buffer.
	buf := make([]byte, MaxPacketSize)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) && ctx.Err() != nil {
				return nil // shutdown requested
			}
			return fmt.Errorf("read: %w", err)
		}

		s.stats.PacketsIn.Add(1)
		s.stats.BytesIn.Add(uint64(n))
		s.rememberClient(addr)

		reply := s.Handle(string(buf[:n]))

		written, err := conn.WriteTo([]byte(reply), addr)
		if err != nil {
			// A failed reply affects one client only, so log it and keep serving.
			s.log.Warn("write failed", "to", addr, "error", err)
			continue
		}
		s.stats.PacketsOut.Add(1)
		s.stats.BytesOut.Add(uint64(written))

		s.log.Info("packet", "from", addr, "bytes", n, "reply_bytes", written)
	}
}

// Handle turns one request into one reply. It has no side effects on the
// network, which makes it easy to unit test.
func (s *Server) Handle(msg string) string {
	msg = strings.TrimSpace(msg)
	cmd, arg, _ := strings.Cut(msg, " ")
	arg = strings.TrimSpace(arg)

	switch strings.ToUpper(cmd) {
	case "PING":
		return "PONG"
	case "ECHO":
		return arg
	case "UPPER":
		return strings.ToUpper(arg)
	case "TIME":
		return s.now().UTC().Format(time.RFC3339)
	case "STATS":
		return fmt.Sprintf("packets_in=%d packets_out=%d bytes_in=%d bytes_out=%d clients=%d",
			s.stats.PacketsIn.Load(), s.stats.PacketsOut.Load(),
			s.stats.BytesIn.Load(), s.stats.BytesOut.Load(), s.clientCount())
	case "":
		return "ERR empty request"
	default:
		return "ERR unknown command: " + cmd
	}
}

// rememberClient records a sender address for the STATS client count.
func (s *Server) rememberClient(addr net.Addr) {
	s.mu.Lock()
	s.clients[addr.String()] = struct{}{}
	s.mu.Unlock()
}

// clientCount returns how many distinct sender addresses have been seen.
func (s *Server) clientCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}
