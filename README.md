# Kasper

HTTP toolkit for Go.

## Packages

### mux

HTTP request multiplexer with URL pattern matching.

Features:

- URL path variables with optional regex constraints
- Host, method, header, query, and scheme matchers
- Subrouters with path prefix grouping
- Middleware support
- Named routes with URL building
- Strict slash and path cleaning options
- Walk function for route inspection
- CORS method middleware

### websocket

RFC 6455 and RFC 8441 compliant WebSocket implementation.

Features:

- Client and Server support
- Text/binary messaging
- Streaming API (NextReader/NextWriter)
- Control frames (ping, pong, close)
- Compression (permessage-deflate, RFC 7692, stateless)
- Proxy support (HTTP CONNECT)
- Subprotocol negotiation
- JSON helpers
- PreparedMessage for efficient broadcasting
- WriteBufferPool for buffer reuse

## License

MIT
