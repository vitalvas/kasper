// Package kasper is an HTTP toolkit for Go providing a drop-in gorilla/mux
// replacement with WebSocket, OpenAPI, and middleware support.
//
// The toolkit is organized into the following sub-packages:
//
//   - [github.com/vitalvas/kasper/mux] -- HTTP request multiplexer, API-compatible
//     with gorilla/mux. Supports path variables with regex constraints and macros,
//     host/method/header/query matchers, subrouters, middleware, named routes,
//     URL building, request binding, and response helpers.
//
//   - [github.com/vitalvas/kasper/websocket] -- RFC 6455 / RFC 8441 WebSocket
//     implementation with client and server support, streaming, JSON helpers,
//     permessage-deflate compression, and prepared messages for broadcasting.
//
//   - [github.com/vitalvas/kasper/openapi] -- Automatic OpenAPI v3.1.0 spec
//     generation from mux routes via reflection and struct tags. Supports
//     JSON Schema Draft 2020-12, security schemes, webhooks, callbacks,
//     and multiple documentation UIs.
//
//   - [github.com/vitalvas/kasper/httpsig] -- HTTP Message Signatures
//     (RFC 9421) with optional Content-Digest (RFC 9530). Provides signing,
//     verification, client transport, and server middleware.
//
//   - [github.com/vitalvas/kasper/muxhandlers] -- HTTP middleware collection
//     including CORS, authentication, compression, security headers, request
//     size limits, timeouts, recovery, and more.
//
//   - [github.com/vitalvas/kasper/blindrsa] -- RSA Blind Signatures per
//     RFC 9474 (RSABSSA). Provides blinding, signing, finalizing, and
//     verification along with HTTP handlers for token issuance.
//
// Install with:
//
//	go get github.com/vitalvas/kasper
package kasper
