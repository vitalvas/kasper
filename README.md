# Kasper

HTTP toolkit for Go. Drop-in gorilla/mux replacement with WebSocket, OpenAPI, and middleware support.
Inspired by [FastAPI](https://fastapi.tiangolo.com/).

```bash
go get github.com/vitalvas/kasper
```

Requires Go 1.25+.

---

## mux `github.com/vitalvas/kasper/mux`

HTTP request multiplexer. API-compatible with gorilla/mux.

| Feature | Details |
|---------|---------|
| Path variables | Regex constraints and macros: `uuid`, `int`, `float`, `slug`, `alpha`, `alphanum`, `date`, `hex`, `domain` |
| Matchers | Host, method, header, query, scheme, custom `MatcherFunc` |
| Routing | Subrouters, middleware, named routes, URL building |
| Request/Response | `BindJSON`, `BindXML`, `ResponseJSON`, `ResponseXML` |
| Standards | Strict slash (RFC 7538), path cleaning (RFC 3986), route walking |

---

## websocket `github.com/vitalvas/kasper/websocket`

RFC 6455 / RFC 8441 WebSocket implementation.

| Feature | Details |
|---------|---------|
| Connections | Client (`Dialer`) and server (`Upgrader`) |
| Messaging | Text/binary, streaming (`NextReader`/`NextWriter`), JSON helpers |
| Control | Ping, pong, close frames, subprotocol negotiation |
| Compression | permessage-deflate (RFC 7692), stateless |
| Performance | `PreparedMessage` for broadcasting, `BufferPool` for buffer reuse |
| Network | Proxy support (HTTP CONNECT) |

---

## openapi `github.com/vitalvas/kasper/openapi`

Automatic OpenAPI v3.1.0 spec generation from mux routes via reflection and struct tags.

| Feature | Details |
|---------|---------|
| Schema | JSON Schema Draft 2020-12, struct tags (`openapi:"format=email,minLength=1"`) |
| Routes | Named routes (`Op`), direct attachment (`Route`), groups for shared metadata |
| Security | Basic, bearer, OAuth2, API key schemes |
| Content | Webhooks, callbacks, multiple content types, generic type support |
| Docs UI | Swagger UI, RapiDoc, Redoc |
| Export | JSON and YAML schema endpoints |

---

## muxhandlers `github.com/vitalvas/kasper/muxhandlers`

HTTP middleware for the mux router.

| Middleware | Standard | Details |
|------------|----------|---------|
| `CORSMiddleware` | Fetch Standard | Origin validation, preflight, wildcard/subdomain patterns |
| `BasicAuthMiddleware` | RFC 7617 | Static or dynamic credentials, constant-time comparison |
| `ProxyHeadersMiddleware` | RFC 7239 | X-Forwarded-For/Proto/Host, Forwarded header, trusted proxy validation |
| `RecoveryMiddleware` | -- | Panic recovery, 500 response, optional log callback |
| `RequestIDMiddleware` | RFC 9562 | UUID v4/v7 generation/propagation via X-Request-ID header |
| `RequestSizeLimitMiddleware` | -- | Body size enforcement via `http.MaxBytesReader`, 413 on excess |
| `TimeoutMiddleware` | -- | Handler execution time limit via `http.TimeoutHandler`, 503 on timeout |
| `CompressionMiddleware` | -- | Gzip/deflate response compression with `sync.Pool`, quality-based encoding selection, min-length threshold |
| `SecurityHeadersMiddleware` | -- | X-Content-Type-Options, X-Frame-Options, Referrer-Policy, HSTS, CSP, Permissions-Policy |
| `MethodOverrideMiddleware` | -- | HTTP method override via configurable header, POST-only, allowed method validation |
| `ContentTypeCheckMiddleware` | -- | Content-Type validation with case-insensitive matching, parameter-ignoring, 415 on mismatch |
