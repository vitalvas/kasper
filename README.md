# Kasper

HTTP toolkit for Go. Drop-in gorilla/mux replacement with WebSocket, OpenAPI, and middleware support.

```bash
go get github.com/vitalvas/kasper
```

Requires Go 1.25+.

---

## mux

HTTP request multiplexer. API-compatible with gorilla/mux.

| Feature | Details |
|---------|---------|
| Path variables | Regex constraints and macros: `uuid`, `int`, `float`, `slug`, `alpha`, `alphanum`, `date`, `hex`, `domain` |
| Matchers | Host, method, header, query, scheme, custom `MatcherFunc` |
| Routing | Subrouters, middleware, named routes, URL building |
| Request/Response | `BindJSON`, `BindXML`, `BindQuery`, `BindForm`, `ResponseJSON`, `ResponseXML` |
| Values encoding | `EncodeQuery`, `EncodeForm` — struct to `url.Values` with dot notation |
| Standards | Strict slash (RFC 7538), path cleaning (RFC 3986), route walking |

---

## websocket

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

## openapi

Automatic OpenAPI v3.1.0 spec generation from mux routes via reflection and struct tags.

| Feature | Details |
|---------|---------|
| Schema | JSON Schema Draft 2020-12, struct tags (`openapi:"format=email,minLength=1"`), `Namer`/`Exampler` interfaces |
| Routes | Named routes (`Op`), direct attachment (`Route`), groups for shared metadata |
| Security | Basic, bearer, OAuth2, API key schemes |
| Content | Webhooks, callbacks, multiple content types, generic type support |
| Docs UI | Swagger UI, RapiDoc, Redoc |
| Export | `doc.JSON()`, `doc.YAML()`, JSON/YAML schema endpoints |
| Parsing | `DocumentFromJSON`, `DocumentFromYAML` |
| Merge | `MergeDocuments` combines multiple specs with conflict detection |
| Standalone | `SchemaGenerator.Document` produces a spec without a mux router |

---

## httpsig

HTTP Message Signatures (RFC 9421) with optional Content-Digest (RFC 9530).

| Feature | Standard | Details |
|---------|----------|---------|
| Signing | RFC 9421 | `SignRequest` adds Signature and Signature-Input headers |
| Verification | RFC 9421 | `VerifyRequest` with key resolver, max age, required components |
| Client Transport | RFC 9421 | `http.RoundTripper` for automatic request signing |
| Server Middleware | RFC 9421 | `mux.MiddlewareFunc` for automatic request verification |
| Content-Digest | RFC 9530 | SHA-256/SHA-512 body digest generation and verification |
| Algorithms | RFC 9421 | Ed25519, ECDSA P-256/P-384, RSA-PSS, RSA v1.5, HMAC-SHA256 |

---

## blindrsa

RSA Blind Signatures (RFC 9474 / RSABSSA) for privacy-preserving tokens.

| Feature | Standard | Details |
|---------|----------|---------|
| Blind | RFC 9474 | `Prepare` + `Blind` produce a blinded message and blinding state |
| Sign | RFC 9474 | `BlindSign` signs a blinded message without learning its content |
| Finalize | RFC 9474 | `Finalize` unblinds and verifies the signature |
| Verify | RFC 9474 | `Verify` checks a standard RSA-PSS signature |
| Issuer Handler | RFC 9474 | `http.Handler` for blind token issuance |
| Client | RFC 9474 | HTTP client for obtaining blind signatures |
| Verify Middleware | RFC 9474 | `mux.MiddlewareFunc` for token verification |
| Variants | RFC 9474 | SHA384-PSS, SHA384-PSSZERO, Randomized, Deterministic |

---

## muxhandlers

HTTP middleware for the mux router.

| Middleware | Standard | Details |
|------------|----------|---------|
| `CORSMiddleware` | Fetch Standard | Origin validation, preflight, wildcard/subdomain patterns |
| `BasicAuthMiddleware` | RFC 7617 | Static or dynamic credentials, constant-time comparison |
| `BearerAuthMiddleware` | RFC 6750 | Bearer token validation with request-aware callback |
| `ProxyHeadersMiddleware` | RFC 7239 | X-Forwarded-For/Proto/Host, Forwarded header, trusted proxy validation |
| `RecoveryMiddleware` | -- | Panic recovery, 500 response, optional log callback |
| `RequestIDMiddleware` | RFC 9562 | UUID v4/v7 generation/propagation via X-Request-ID header |
| `RequestSizeLimitMiddleware` | -- | Body size enforcement via `http.MaxBytesReader`, 413 on excess |
| `TimeoutMiddleware` | -- | Handler execution time limit via `http.TimeoutHandler`, 503 on timeout |
| `CompressionMiddleware` | -- | Gzip/deflate response compression with `sync.Pool`, quality-based encoding selection, min-length threshold |
| `SecurityHeadersMiddleware` | -- | X-Content-Type-Options, X-Frame-Options, Referrer-Policy, HSTS, CSP, Permissions-Policy |
| `MethodOverrideMiddleware` | -- | HTTP method override via configurable header, POST-only, allowed method validation |
| `ContentTypeCheckMiddleware` | -- | Content-Type validation with case-insensitive matching, parameter-ignoring, 415 on mismatch |
| `ServerMiddleware` | -- | Sets `X-Server-Hostname` response header, resolved once via `os.Hostname` or static value |
| `CacheControlMiddleware` | -- | Sets `Cache-Control` and `Expires` response headers based on Content-Type prefix rules, case-insensitive matching |
| `StaticFilesHandler` | -- | Serves static files from any `fs.FS` via `http.FileServerFS`, directory listing disabled by default, SPA fallback support |
| `RegisterProfiler` | -- | Registers `net/http/pprof` and `expvar` endpoints on any mount prefix |
| `IdempotencyMiddleware` | IETF Draft | Caches responses by `Idempotency-Key` header, replays on duplicate requests |
| `ContentNegotiationMiddleware` | RFC 9110 | Accept header parsing with quality values, 406 on mismatch |
| `WriteProblemDetails` | RFC 9457 | Writes `application/problem+json` error responses with extensions |
| `EarlyHintsMiddleware` | RFC 8297 | Sends 103 Early Hints with Link headers for resource preloading |
| `SunsetMiddleware` | RFC 8594 | Sets `Sunset`, `Deprecation`, and `Link` response headers for API deprecation |
| `IPAllowMiddleware` | -- | IP allow list with CIDR support, customizable denied handler |
| `PatchRoutingMiddleware` | RFC 7396, RFC 6902 | PATCH content type validation and routing, stores resolved type in context |
| `AcceptPatchMiddleware` | RFC 5789 | OPTIONS handler with Allow and Accept-Patch headers, auto-discovers methods from router |
| `RedirectMiddleware` | -- | Path-based redirects with exact and wildcard matching, HTML meta-refresh body |
| `CanonicalHostMiddleware` | -- | Redirects requests to canonical scheme and host, preserves path and query |

---

## securecookie

Authenticated and encrypted cookie values using AES-GCM.

| Feature | Details |
|---------|---------|
| Encryption | AES-128-GCM, AES-192-GCM, AES-256-GCM |
| AAD | Optional binding to user ID, cookie name, tenant, etc. |
| Timestamps | MaxAge, MinAge, future-timestamp rejection (5 min skew) |
| Key rotation | Multi-codec encode/decode via `CodecsFromKeys` |
| Serialization | JSON (default), pluggable `Serializer` interface |
| Key generation | `GenerateKey(size)` with cryptographic randomness |
