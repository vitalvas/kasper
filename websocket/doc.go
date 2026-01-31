// Package websocket implements the WebSocket protocol defined in RFC 6455,
// with HTTP/2 support per RFC 8441.
//
// This package provides a complete WebSocket implementation including:
//   - Server-side connection upgrading via Upgrader
//   - Client-side connection dialing via Dialer
//   - HTTP/2 WebSocket bootstrapping (RFC 8441)
//   - Per-message compression (permessage-deflate, RFC 7692)
//   - JSON encoding/decoding helpers
//   - Prepared messages for efficient broadcasting
//
// Server Example:
//
//	var upgrader = websocket.Upgrader{
//	    ReadBufferSize:  1024,
//	    WriteBufferSize: 1024,
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
package websocket
