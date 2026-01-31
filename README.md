# Kasper

HTTP toolkit for Go.

## Packages

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
