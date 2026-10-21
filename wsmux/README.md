# wsmux

Stream multiplexing over a single WebSocket connection.

Run many independent logical streams over one `websocket.Conn`, each carrying
its own sequence of messages — like SSH channels or HTTP/2 streams, but over a
WebSocket.

## Features

- Many concurrent streams over one connection
- Client/server roles with collision-free stream IDs (odd client, even server)
- Blocking per-stream `Read`/`Write` API
- A single read loop delivers payloads; stream locks serialize delivery with teardown
- Non-blocking delivery: one slow stream never head-of-line blocks the others
- No per-frame size cap (the underlying connection frames each message)

## Installation

```bash
go get go.vitalvas.com/kasper/wsmux
```

## Protocol

Each message is a binary WebSocket frame with a 5-byte header:

```
[1-byte type][4-byte big-endian stream id][payload]
```

No length field is carried — the `websocket` connection already frames every
message and reports its length, so the payload is everything after the header.

## Server

```go
conn, err := upgrader.Upgrade(w, r, nil)
if err != nil {
    return
}

mux := wsmux.NewServer(conn, wsmux.Options{})
defer mux.Close()

for {
    stream, err := mux.Accept()
    if err != nil {
        return
    }
    go func(s *wsmux.Stream) {
        defer s.Close()
        for {
            data, err := s.Read()
            if err != nil {
                return
            }
            _ = s.Write(data) // echo
        }
    }(stream)
}
```

## Client

```go
mux := wsmux.NewClient(conn, wsmux.Options{})
defer mux.Close()

stream, err := mux.Open()
if err != nil {
    return
}
defer stream.Close()

if err := stream.Write([]byte("ping")); err != nil {
    return
}

resp, err := stream.Read()
if err != nil {
    return
}
fmt.Println(string(resp))
```

## Bidirectional

The mux is symmetric: after `NewClient`/`NewServer`, both roles use the same
API. Either side can `Open` a stream and either side can `Accept` one — the
client/server split only fixes stream-ID parity (client odd, server even) so
the two ends never collide. The Server/Client examples above show the common
pattern, not a restriction: a server may `Open` a stream to push to the client,
and a client may `Accept` server-initiated streams.

## Options

| Field          | Default | Description                          |
| -------------- | ------- | ------------------------------------ |
| `StreamBuffer` | 64      | Inbound payload buffer per stream    |

When a stream's buffer fills, that stream is reset with a CLOSE frame rather
than blocking the shared read loop, so a slow consumer cannot stall the others.
Resets are sent through a bounded queue. If that queue fills because the peer
is not reading, the tunnel closes and all streams are torn down. Streams rejected
because the accept backlog is full also receive a CLOSE frame.

## Concurrency

A single internal goroutine owns all reads and delivers stream payloads.
Stream locks serialize delivery with teardown. For each stream, `Read` and `Write`
may be called from one goroutine each, concurrently with the other — matching
the one-reader/one-writer model of the underlying connection.

## Limitations

This is single-connection multiplexing: all streams share one TCP pipe and its
ordering. It suits logically independent request/response streams, not cases
that need the isolation of separate connections.

Stream IDs are never reused within a tunnel. `Open` returns
`ErrStreamIDExhausted` after all IDs for that role have been allocated.

Closed peer IDs are remembered as ranges, so sequential stream churn uses
constant history space. At most 1024 disjoint ranges are retained. If closing
another stream would exceed that limit, the tunnel closes and its history is
released. This bounds memory while still rejecting late DATA for old streams
and allowing streams to send their first DATA out of ID order.
