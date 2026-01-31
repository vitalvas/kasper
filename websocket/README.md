# websocket

RFC 6455 compliant WebSocket implementation.

## Features

- Client and Server support
- Text/binary messaging
- Streaming API (NextReader/NextWriter)
- Control frames (ping, pong, close)
- Compression (permessage-deflate, RFC 7692)
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

## Compression

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

## Proxy

```go
dialer := websocket.Dialer{
    Proxy: http.ProxyFromEnvironment,
}
conn, _, err := dialer.Dial("ws://localhost:8080/ws", nil)
```
