// Package websocket implements the WebSocket protocol defined in RFC 6455,
// with HTTP/2 support per RFC 8441.
//
// This package provides a complete WebSocket implementation including:
//   - Server-side connection upgrading via Upgrader
//   - Client-side connection dialing via Dialer
//   - HTTP/2 WebSocket bootstrapping (RFC 8441)
//   - Per-message compression (permessage-deflate, RFC 7692)
//   - Keepalive with configurable ping payload and pong tolerance
//   - Message type policy enforcement (binary-only or text-only)
//   - JSON encoding/decoding helpers
//   - Prepared messages for efficient broadcasting
//
// Server Example:
//
//	var upgrader = websocket.Upgrader{
//	    ReadBufferSize:    1024,
//	    WriteBufferSize:   1024,
//	    MessageTypePolicy: websocket.MessageTypePolicyBinary,
//	}
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    conn, err := upgrader.Upgrade(w, r, nil)
//	    if err != nil {
//	        return
//	    }
//	    defer conn.Close()
//
//	    for {
//	        messageType, p, err := conn.ReadMessage()
//	        if err != nil {
//	            return
//	        }
//	        if err := conn.WriteMessage(messageType, p); err != nil {
//	            return
//	        }
//	    }
//	}
//
// Client Example:
//
//	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer conn.Close()
//
//	err = conn.WriteMessage(websocket.TextMessage, []byte("hello"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Concurrency:
//
// Connections support one concurrent reader and one concurrent writer.
// Applications are responsible for ensuring that no more than one goroutine
// calls the write methods (NextWriter, WriteMessage, WriteJSON, WritePreparedMessage,
// WriteControl) concurrently, and that no more than one goroutine calls the
// read methods (NextReader, ReadMessage, ReadJSON) concurrently.
//
// The Close method can be called concurrently with other methods.
//
// Keepalive:
//
// StartKeepalive sends periodic ping frames and optionally enforces a pong
// deadline. Set PongTimeout to a non-zero value to expire the read deadline
// if no pong is received within Interval+PongTimeout; leave it zero for
// heartbeat-only mode where missing pong responses are silently tolerated.
// PingPayload and OnPong allow the caller to embed arbitrary bytes in the
// ping frame — useful for round-trip latency measurement.
//
// Message Type Policy:
//
// SetMessageTypePolicy restricts which data frame types are accepted on a
// connection. MessageTypePolicyBinary closes the connection with a protocol
// error when a text frame is received; MessageTypePolicyText does the same
// for binary frames. The Upgrader applies the policy automatically to every
// accepted connection via its MessageTypePolicy field.
//
// Server Ping/Pong Policy:
//
// The Upgrader provides fields to control how incoming ping frames are handled:
//
//   - DisablePongReply: suppress the automatic pong response entirely.
//   - RequireEmptyPingPayload: close the connection with CloseProtocolError when
//     a ping with a non-empty payload is received.
//   - EmptyPongPayload: always send an empty pong, ignoring the ping payload.
//   - PingHandler: full control; the returned bytes become the pong payload.
//     RequireEmptyPingPayload is still enforced before the handler is called.
//
// Frame Size Limit:
//
// SetMaxFrameSize sets the maximum allowed payload length for a single WebSocket
// frame. Frames that exceed the limit are rejected with ErrFrameSizeExceeded on
// reads, and writes of oversized frames are also rejected. The Upgrader and
// Dialer expose MaxFrameSize to apply the limit automatically to every accepted
// or dialed connection.
//
// Origin Checking:
//
// Web browsers allow any site to open a WebSocket connection to any other site.
// The server must validate the Origin header to prevent attacks. The Upgrader
// calls the CheckOrigin function to validate the request origin. If CheckOrigin
// is nil, the Upgrader uses a safe default that rejects cross-origin requests.
//
// Compression:
//
// Per-message compression is negotiated during the WebSocket handshake when
// EnableCompression is set to true on the Upgrader or Dialer. When compression
// is enabled, messages are compressed using the permessage-deflate extension
// (RFC 7692) with stateless compression (no context takeover).
//
// Extensions:
//
// This package supports the permessage-deflate extension (RFC 7692) for
// per-message compression. Custom WebSocket extensions are not supported.
// The permessage-deflate extension is the only widely deployed WebSocket
// extension and covers the primary use case for data compression.
package websocket
