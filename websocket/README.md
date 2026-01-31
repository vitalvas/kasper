# websocket

RFC 6455 compliant WebSocket implementation.

## Features

- Client and Server support
- HTTP/1.1 upgrade (RFC 6455) and HTTP/2 (RFC 8441)
- Text/binary messaging
- Streaming API (NextReader/NextWriter)
- Control frames (ping, pong, close)
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

## Custom Headers

Pass custom HTTP headers (User-Agent, authentication, etc.) to the handshake request:

```go
headers := http.Header{}
headers.Set("User-Agent", "MyApp/1.0")
headers.Set("Authorization", "Bearer token")
headers.Set("X-Custom-Key", "value")

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

WebSocket over HTTP/2 (RFC 8441) is supported. Server automatically detects HTTP/2 CONNECT requests.

Client usage:

```go
dialer := websocket.Dialer{
    HTTPClient: &http.Client{
        Transport: &http2.Transport{},
    },
}
conn, _, err := dialer.Dial("wss://localhost:8080/ws", nil)
```
