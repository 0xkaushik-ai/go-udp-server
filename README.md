# Go UDP Server

A small UDP server and client written in Go using **only the standard library**.
The server answers simple text commands such as `PING`, `ECHO` and `TIME`, one packet in and one packet out.

The project is meant for learning. It shows how to:

- open a UDP socket with `net.ListenPacket` and read packets with `ReadFrom` / `WriteTo`
- write a UDP client with timeouts, since UDP never tells you a packet was lost
- shut down cleanly by closing the socket when a context is cancelled
- keep counters safe with `sync/atomic`
- test a network server on a random local port

---

## UDP vs. TCP in one minute

|                  | TCP (e.g. HTTP)                   | UDP (this project)                    |
|------------------|-----------------------------------|---------------------------------------|
| Connection       | Handshake first, then a stream    | None. Each packet stands alone        |
| Delivery         | Guaranteed, in order              | Packets may be lost, duplicated or reordered |
| Message borders  | No. It's a byte stream            | Yes. One `Write` is one packet        |
| Server API in Go | `Listen` → `Accept` → per-connection `Read` | One socket: `ReadFrom` / `WriteTo` |
| Typical uses     | Web, SSH, email                   | DNS, games, video/voice, metrics      |

Because UDP doesn't guarantee delivery, the client waits for a reply only up to a timeout. If none arrives, it reports that the packet was lost or the server is down.

Sometimes the client finds out sooner. When nothing is listening on the target port, the remote host often sends back an ICMP "port unreachable" message. Go reports that as `connection refused`, and the client prints a clear message saying no server is listening. Firewalls often drop those ICMP messages, though, and then you only get the timeout.

---

## Requirements

- Go **1.21 or newer**, for `context.AfterFunc` and `log/slog`

## Quick start

Terminal 1 starts the server:
```bash
cd UDP_server
go run ./cmd/server          # listens on :9000
```

Terminal 2 runs the client:
```bash
go run ./cmd/client PING              # one-shot → PONG
go run ./cmd/client ECHO hello        # → hello
go run ./cmd/client                   # interactive mode
```

You can also talk to it without the Go client, using standard Linux tools:
```bash
echo -n "PING" | nc -u -w1 127.0.0.1 9000
echo -n "TIME" | socat - UDP:127.0.0.1:9000
```

### Options

| Program | Flag       | Default          | Meaning |
|---------|------------|------------------|---------|
| server  | `-addr`    | `:9000`          | Address to listen on |
| client  | `-addr`    | `127.0.0.1:9000` | Server address |
| client  | `-timeout` | `2s`             | How long to wait for each reply |

### Make targets

| Command       | What it does |
|---------------|--------------|
| `make run`    | Start the server |
| `make client` | Start the interactive client |
| `make build`  | Build `bin/udpserver` and `bin/udpclient` |
| `make test`   | Run tests with the race detector |
| `make vet`    | Run `go vet` static analysis |
| `make fmt`    | Format the code |

---

## Protocol

Each request is one UDP packet of plain text: a command, optionally followed by a space and some text. Commands are case-insensitive, and leading or trailing whitespace is ignored. The server answers each request with exactly one packet.

| Request         | Reply                                   | Example |
|-----------------|-----------------------------------------|---------|
| `PING`          | `PONG`                                  | `PING` → `PONG` |
| `ECHO <text>`   | the text, unchanged                     | `ECHO hi` → `hi` |
| `UPPER <text>`  | the text in upper case                  | `UPPER go` → `GO` |
| `TIME`          | server time in RFC 3339 format, UTC     | `2026-10-23T12:00:00Z` |
| `STATS`         | counters since the server started       | `packets_in=5 packets_out=4 bytes_in=31 bytes_out=40 clients=2` |
| empty packet    | `ERR empty request`                     | |
| anything else   | `ERR unknown command: <cmd>`            | `FLY` → `ERR unknown command: FLY` |

The largest packet the server reads is **65,507 bytes**, the maximum UDP payload over IPv4. In practice, keep messages under about 1,400 bytes. Larger packets get split into IP fragments, and those are more likely to be lost.

---

## Project structure

```
UDP_server/
├── cmd/
│   ├── server/main.go          # server entry point: flags, socket, Ctrl+C handling
│   └── client/main.go          # client: one-shot and interactive modes
├── internal/
│   └── server/
│       ├── server.go           # read loop, command handling, stats
│       └── server_test.go      # unit tests + real UDP round-trip test
├── Makefile
├── go.mod
└── README.md
```

## How a packet flows

```
client ── packet "PING" ──►  net.PacketConn.ReadFrom(buf)
                                   │ (bytes + sender address)
                                   ▼
                             update stats, remember sender
                                   │
                                   ▼
                             Server.Handle("PING") → "PONG"
                                   │
                                   ▼
client ◄── packet "PONG" ──  net.PacketConn.WriteTo("PONG", sender)
```

## Design notes

- **One socket, one loop.** A UDP server has no `Accept`. Every client's packets arrive on the same socket, and the sender's address comes with each one. The reply goes back to that address.
- **The buffer is reused.** A single 64 KB buffer serves every read. This is safe because each packet is fully handled before the next read overwrites it. If you ever handle packets in goroutines, copy the bytes first.
- **Graceful shutdown.** `ReadFrom` blocks and doesn't watch a context. So `context.AfterFunc` closes the socket when Ctrl+C arrives, `ReadFrom` returns `net.ErrClosed`, and `Serve` returns `nil`.
- **`Handle` is a pure function.** The logic that turns a request into a reply doesn't touch the network, so it can be unit-tested directly.
- **Write errors don't stop the server.** If a reply to one client fails, the server logs it and keeps serving everyone else.
- **Atomic counters.** The stats use `sync/atomic` instead of a mutex. The client set does use a mutex, because a map isn't safe to use from several goroutines at once.

## Security notes

UDP sender addresses **can be forged**, because there is no handshake to prove who sent a packet. This leads to:

- **Amplification / reflection attacks.** An attacker sends a small request with a forged source address, and the server sends a bigger reply to the victim. Here, `STATS` and `TIME` replies are a little bigger than their requests. A public UDP service should keep replies no bigger than requests, or add per-address rate limiting.
- **No authentication or encryption.** Anyone who can reach the port can send commands and read the replies. For secure UDP, look at DTLS or QUIC.

Run this server on `localhost` or a trusted network only.

## Testing

```bash
go test -race ./...
```

- `TestHandle` checks each command's reply without using the network.
- `TestServeOverUDP` starts the server on a random port (`127.0.0.1:0`), exchanges real packets with it, and checks that it shuts down cleanly.

## Ideas to extend

- Handle each packet in a worker pool for higher throughput
- Add per-IP rate limiting
- Add request IDs and retries in the client, so it copes with lost packets
- Build a tiny DNS server on top of this, since DNS runs over UDP
- Add a multiplayer "position update" demo, the way game servers use UDP
