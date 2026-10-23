// Command client sends requests to the UDP server and prints the replies.
//
// Usage:
//
//	go run ./cmd/client PING            # send one message
//	go run ./cmd/client                 # interactive: one message per line
//	go run ./cmd/client -addr host:9000 -timeout 3s ECHO hi
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9000", "server address (host:port)")
	timeout := flag.Duration("timeout", 2*time.Second, "how long to wait for each reply")
	flag.Parse()

	// Dial on UDP does not contact the server. It only fixes the remote
	// address, so Write and Read can be used instead of WriteTo and ReadFrom.
	conn, err := net.Dial("udp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// One-shot mode: the message comes from the command-line arguments.
	if flag.NArg() > 0 {
		reply, err := request(conn, strings.Join(flag.Args(), " "), *timeout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(reply)
		return
	}

	// Interactive mode: read lines from stdin until EOF (Ctrl+D).
	fmt.Printf("connected to %s (UDP). Type PING, ECHO <text>, UPPER <text>, TIME, STATS. Ctrl+D to quit.\n", *addr)
	in := bufio.NewScanner(os.Stdin)
	for fmt.Print("> "); in.Scan(); fmt.Print("> ") {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		reply, err := request(conn, line, *timeout)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Println(reply)
	}
	fmt.Println()
}

// request sends msg and waits up to timeout for one reply packet.
//
// UDP gives no delivery guarantee, so a lost request or reply shows up
// only as a timeout. A real client might retry here.
func request(conn net.Conn, msg string, timeout time.Duration) (string, error) {
	if _, err := conn.Write([]byte(msg)); err != nil {
		return "", fmt.Errorf("send: %w", err)
	}
	conn.SetReadDeadline(time.Now().Add(timeout))

	buf := make([]byte, 65507)
	n, err := conn.Read(buf)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return "", fmt.Errorf("no reply within %s (server down or packet lost)", timeout)
		}
		// On a connected UDP socket, the OS reports an ICMP "port unreachable"
		// from the remote host as ECONNREFUSED: nothing is listening there.
		if errors.Is(err, syscall.ECONNREFUSED) {
			return "", errors.New("connection refused: no server is listening on that port")
		}
		return "", fmt.Errorf("receive: %w", err)
	}
	return string(buf[:n]), nil
}
