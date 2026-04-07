# websocket

RFC 6455 compliant WebSocket implementation.

## Features

- Client and Server support
- HTTP/1.1 upgrade (RFC 6455) and HTTP/2 (RFC 8441)
- Text/binary messaging
- Streaming API (NextReader/NextWriter)
- Control frames (ping, pong, close)
- Keepalive with configurable ping payload and pong tolerance
- Message type policy enforcement (binary-only or text-only)
- Compression (permessage-deflate, RFC 7692, stateless)
- Proxy support (HTTP CONNECT)
- Subprotocol negotiation
- JSON helpers
- PreparedMessage for efficient broadcasting
- WriteBufferPool for buffer reuse

## Installation

```bash
go get github.com/vitalvas/kasper/websocket
```

## Server

```go
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

func handler(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    for {
        messageType, p, err := conn.ReadMessage()
        if err != nil {
            return
        }
        if err := conn.WriteMessage(messageType, p); err != nil {
            return
        }
    }
}
```

## Client

```go
conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

err = conn.WriteMessage(websocket.TextMessage, []byte("hello"))
if err != nil {
    log.Fatal(err)
}

_, message, err := conn.ReadMessage()
if err != nil {
    log.Fatal(err)
}
```

## Keepalive

StartKeepalive sends periodic ping frames to keep the connection alive and
optionally detect dead peers.

**With pong timeout** — the read deadline expires if no pong arrives within
`Interval + PongTimeout`. The next `ReadMessage` call returns a timeout error:

```go
conn.StartKeepalive(websocket.KeepaliveOptions{
    Interval:    30 * time.Second,
    PongTimeout: 10 * time.Second,
})
```

**Heartbeat-only (pong tolerance)** — pings are sent to prevent NAT/firewall
timeouts; missing pong responses are silently ignored:

```go
conn.StartKeepalive(websocket.KeepaliveOptions{
    Interval: 30 * time.Second,
    // PongTimeout: 0 — missing pongs do not affect the connection
})
```

**Ping payload / latency measurement** — arbitrary bytes can be embedded in
the ping frame. The server echoes them back in the pong, enabling the caller
to implement round-trip latency measurement:

```go
conn.StartKeepalive(websocket.KeepaliveOptions{
    Interval:    30 * time.Second,
    PongTimeout: 10 * time.Second,
    PingPayload: func() []byte {
        ts := make([]byte, 8)
        binary.BigEndian.PutUint64(ts, uint64(time.Now().UnixNano()))
        return ts
    },
    OnPong: func(payload []byte) {
        if len(payload) == 8 {
            sent := int64(binary.BigEndian.Uint64(payload))
            rtt := time.Since(time.Unix(0, sent))
            log.Printf("RTT: %s", rtt)
        }
    },
})
```

Both the client and server can initiate keepalive — call `StartKeepalive` on
whichever side needs to send pings.

## Message Type Policy

Restrict which data frame types a connection accepts. Receiving a forbidden
frame type closes the connection with a protocol error and returns
`ErrMessageTypeForbidden` from `ReadMessage`.

On a single connection:

```go
conn.SetMessageTypePolicy(websocket.MessageTypePolicyBinary) // reject text frames
conn.SetMessageTypePolicy(websocket.MessageTypePolicyText)   // reject binary frames
conn.SetMessageTypePolicy(websocket.MessageTypePolicyAny)    // no restriction (default)
```

On the server, apply the policy to every accepted connection automatically:

```go
var upgrader = websocket.Upgrader{
    MessageTypePolicy: websocket.MessageTypePolicyBinary,
}
```

To also disable the automatic pong reply to client ping frames:

```go
var upgrader = websocket.Upgrader{
    MessageTypePolicy: websocket.MessageTypePolicyBinary,
    DisablePongReply:  true,
}
```

## Server Ping/Pong Policy

The `Upgrader` provides several fields to control how the server handles incoming
ping frames:

**Require empty ping payload** — close the connection with a protocol error if a
client sends a ping with a non-empty payload:

```go
var upgrader = websocket.Upgrader{
    RequireEmptyPingPayload: true,
}
```

**Empty pong payload** — always send an empty pong, ignoring whatever payload the
client put in the ping:

```go
var upgrader = websocket.Upgrader{
    EmptyPongPayload: true,
}
```

**Custom ping handler** — full control over the pong response. The returned bytes
become the pong payload; returning `nil` sends an empty pong.
`RequireEmptyPingPayload` is still enforced before the handler is called:

```go
var upgrader = websocket.Upgrader{
    PingHandler: func(payload []byte) ([]byte, error) {
        // echo payload with a prefix
        return append([]byte("srv:"), payload...), nil
    },
}
```

## Frame Size Limit

Limit the maximum payload size of a single WebSocket frame. Frames exceeding the
limit are rejected with `ErrFrameSizeExceeded`. This matches protocol
constraints imposed by some services (e.g. AWS IoT Secure Tunneling enforces a
131076-byte frame limit).

On a single connection:

```go
conn.SetMaxFrameSize(131076)
```

On the server, apply automatically to every accepted connection:

```go
var upgrader = websocket.Upgrader{
    MaxFrameSize: 131076,
}
```

On the client:

```go
dialer := websocket.Dialer{
    MaxFrameSize: 131076,
}
conn, _, err := dialer.Dial("ws://localhost:8080/ws", nil)
```

## Custom Headers

Pass custom HTTP headers (User-Agent, authentication, etc.) to the handshake
request. Original key casing is preserved, which is required by some services
(e.g. AWS IoT Secure Tunneling expects `access-token` in lowercase):

```go
headers := http.Header{}
headers.Set("User-Agent", "MyApp/1.0")
headers.Set("Authorization", "Bearer token")
headers["access-token"] = []string{"tok123"}

conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", headers)
```

## Compression

Supports permessage-deflate extension (RFC 7692) with stateless compression.

Enable compression on the server:

```go
var upgrader = websocket.Upgrader{
    EnableCompression: true,
}
```

Enable compression on the client:

```go
dialer := websocket.Dialer{
    EnableCompression: true,
}
conn, _, err := dialer.Dial("ws://localhost:8080/ws", nil)
```

Supported parameters:

- `server_no_context_takeover` - always enabled
- `client_no_context_takeover` - acknowledged if requested
- `client_max_window_bits` - acknowledged with value 15

## Custom HTTP Client

Configure proxy, TLS, custom dial, and other settings via `http.Client`:

```go
dialer := websocket.Dialer{
    HTTPClient: &http.Client{
        Transport: &http.Transport{
            Proxy: http.ProxyFromEnvironment,
            TLSClientConfig: &tls.Config{
                InsecureSkipVerify: true,
            },
            DialContext: customDialFunc,
        },
    },
    HandshakeTimeout: 10 * time.Second,
}
conn, _, err := dialer.Dial("ws://localhost:8080/ws", nil)
```

Settings extracted from `http.Transport`:

- `Proxy` - HTTP CONNECT proxy tunneling
- `TLSClientConfig` - TLS configuration for wss://
- `DialContext` - custom TCP dial function
- `DialTLSContext` - custom TLS dial function

## PreparedMessage

Use PreparedMessage to efficiently send the same message to multiple connections:

```go
pm, err := websocket.NewPreparedMessage(websocket.TextMessage, []byte("hello"))
if err != nil {
    log.Fatal(err)
}

// Send to multiple connections
for _, conn := range connections {
    conn.WritePreparedMessage(pm)
}
```

## HTTP/2

WebSocket over HTTP/2 (RFC 8441) is supported. Server automatically detects
HTTP/2 CONNECT requests.

Client usage:

```go
dialer := websocket.Dialer{
    HTTPClient: &http.Client{
        Transport: &http2.Transport{},
    },
}
conn, _, err := dialer.Dial("wss://localhost:8080/ws", nil)
```
