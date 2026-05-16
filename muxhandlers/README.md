# muxhandlers

HTTP middleware handlers for the `mux` router.

## Installation

```bash
go get github.com/vitalvas/kasper/muxhandlers
```

## CORS Middleware

`CORSMiddleware` implements the full CORS protocol. It validates the
`Origin` header, handles preflight `OPTIONS` requests, and sets the
appropriate response headers.

### CORSConfig

| Field | Type | Description |
|-------|------|-------------|
| `AllowedOrigins` | `[]string` | Exact origins, `"*"` for wildcard, or subdomain patterns |
| `AllowOriginFunc` | `func(string) bool` | Optional dynamic origin check (receives raw origin) |
| `AllowedMethods` | `[]string` | Override methods; empty = auto-discover from router |
| `AllowedHeaders` | `[]string` | Preflight allowed headers; empty = reflect request |
| `ExposeHeaders` | `[]string` | Headers exposed to client code |
| `AllowCredentials` | `bool` | Send `Access-Control-Allow-Credentials: true` |
| `MaxAge` | `int` | Preflight cache seconds; 0 = omit, negative = `"0"` |
| `OptionsStatusCode` | `int` | Preflight HTTP status; 0 = 204 No Content |
| `OptionsPassthrough` | `bool` | Forward preflight to next handler after setting headers |
| `AllowPrivateNetwork` | `bool` | Respond to private network access requests |

### CORS Usage

```go
r := mux.NewRouter()

r.HandleFunc("/users", listUsers).Methods(http.MethodGet)
r.HandleFunc("/users", createUser).Methods(http.MethodPost)

mw, err := muxhandlers.CORSMiddleware(r, muxhandlers.CORSConfig{
    AllowedOrigins:   []string{"https://example.com"},
    AllowCredentials: true,
    AllowedHeaders:   []string{"Content-Type", "Authorization"},
    ExposeHeaders:    []string{"X-Request-Id"},
    MaxAge:           3600,
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Basic Auth Middleware

`BasicAuthMiddleware` implements HTTP Basic Authentication per
[RFC 7617](https://www.rfc-editor.org/rfc/rfc7617). Credentials can be
validated via a dynamic callback (`ValidateFunc`) or a static map
(`Credentials`). When both are set, `ValidateFunc` takes priority.
Static credential comparison uses `crypto/subtle.ConstantTimeCompare`
to prevent timing attacks.

### BasicAuthConfig

| Field | Type | Description |
|-------|------|-------------|
| `Realm` | `string` | Authentication realm for `WWW-Authenticate` header |
| `ValidateFunc` | `func(string, string) bool` | Dynamic credential validation callback |
| `Credentials` | `map[string]string` | Static username-to-password map |

### BasicAuth Usage with ValidateFunc

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.BasicAuthMiddleware(muxhandlers.BasicAuthConfig{
    Realm: "My App",
    ValidateFunc: func(username, password string) bool {
        return username == "admin" && password == "secret"
    },
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

### BasicAuth Usage with Credentials

```go
mw, err := muxhandlers.BasicAuthMiddleware(muxhandlers.BasicAuthConfig{
    Credentials: map[string]string{
        "admin": "secret",
        "user":  "password",
    },
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Bearer Auth Middleware

`BearerAuthMiddleware` implements HTTP Bearer Token Authentication per
[RFC 6750](https://www.rfc-editor.org/rfc/rfc6750). It extracts the token
from the `Authorization` header and validates it using a user-provided
function. When the token is missing, malformed, or invalid, the middleware
responds with 401 Unauthorized and a `WWW-Authenticate: Bearer` header per
RFC 6750 Section 3.

### BearerAuthConfig

| Field | Type | Description |
|-------|------|-------------|
| `Realm` | `string` | Authentication realm for `WWW-Authenticate` header; defaults to `"Restricted"` |
| `ValidateFunc` | `func(*http.Request, string) bool` | Token validation callback; receives request and raw token |

### BearerAuth Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.BearerAuthMiddleware(muxhandlers.BearerAuthConfig{
    Realm: "My API",
    ValidateFunc: func(r *http.Request, token string) bool {
        return token == expectedToken
    },
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Proxy Headers Middleware

`ProxyHeadersMiddleware` populates request fields from reverse proxy
headers when the request originates from a trusted proxy. A trusted
proxy list (IPs and CIDRs) restricts which peers are allowed to set
these headers, preventing spoofing from untrusted clients. When
`TrustedProxies` is empty, `DefaultTrustedProxies` (RFC 1918,
RFC 4193, and loopback ranges) is used.

### Supported Headers

| Field | Headers (priority order) |
|-------|--------------------------|
| `r.RemoteAddr` | `X-Forwarded-For` > `X-Real-IP` > `Forwarded: for=`* |
| `r.URL.Scheme` | `X-Forwarded-Proto` > `X-Forwarded-Scheme`* |
| `r.Host` | `X-Forwarded-Host` > `Forwarded: host=`* |
| `X-Forwarded-By` header | `Forwarded: by=`* |

*Requires `EnableForwarded: true`
([RFC 7239](https://www.rfc-editor.org/rfc/rfc7239)).

### DefaultTrustedProxies

| Range | Description |
|-------|-------------|
| `127.0.0.0/8` | IPv4 loopback (RFC 1122) |
| `10.0.0.0/8` | Class A private (RFC 1918) |
| `172.16.0.0/12` | Class B private (RFC 1918) |
| `192.168.0.0/16` | Class C private (RFC 1918) |
| `100.64.0.0/10` | CGNAT shared address space (RFC 6598) |
| `::1/128` | IPv6 loopback (RFC 4291) |
| `fc00::/7` | IPv6 unique local (RFC 4193) |

### ProxyHeadersConfig

| Field | Type | Description |
|-------|------|-------------|
| `TrustedProxies` | `[]string` | IP/CIDR of trusted proxies; empty = defaults |
| `EnableForwarded` | `bool` | Parse RFC 7239 `Forwarded` header as fallback |

### ProxyHeaders Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.ProxyHeadersMiddleware(muxhandlers.ProxyHeadersConfig{
    TrustedProxies: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Recovery Middleware

`RecoveryMiddleware` recovers from panics in downstream handlers,
returns 500 Internal Server Error to the client, and optionally
invokes a custom log callback with the request and recovered value.

### RecoveryConfig

| Field | Type | Description |
|-------|------|-------------|
| `LogFunc` | `func(*http.Request, any)` | Optional callback; `nil` = no logging |

### Recovery Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

r.Use(muxhandlers.RecoveryMiddleware(muxhandlers.RecoveryConfig{
    LogFunc: func(r *http.Request, err any) {
        log.Printf("panic: %v %s", err, r.URL.Path)
    },
}))
```

## Request ID Middleware

`RequestIDMiddleware` generates or propagates a unique request
identifier. The ID is set on the request header, the response header,
and the request context. Downstream handlers can retrieve it with
`RequestIDFromContext`. By default it generates UUID v4 values using
`github.com/google/uuid`. Use `GenerateUUIDv7` for time-ordered IDs
([RFC 9562](https://www.rfc-editor.org/rfc/rfc9562#section-5.7)).
The `GenerateFunc` receives the current request, allowing ID
generation based on request context.

### RequestIDConfig

| Field | Type | Description |
|-------|------|-------------|
| `HeaderName` | `string` | Header name; defaults to `"X-Request-ID"` |
| `GenerateFunc` | `func(*http.Request) string` | Custom ID generator; defaults to UUID v4 |
| `TrustIncoming` | `bool` | Reuse existing header from the incoming request |

### Built-in Generators

| Function | Description |
|----------|-------------|
| `GenerateUUIDv4` | Random UUID v4 (default) |
| `GenerateUUIDv7` | Time-ordered UUID v7 (RFC 9562) |

### RequestID Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

r.Use(muxhandlers.RequestIDMiddleware(muxhandlers.RequestIDConfig{
    TrustIncoming: true,
}))
```

### RequestID Usage with UUID v7

```go
r.Use(muxhandlers.RequestIDMiddleware(muxhandlers.RequestIDConfig{
    GenerateFunc: muxhandlers.GenerateUUIDv7,
}))
```

### Reading the ID from context

```go
func handler(w http.ResponseWriter, r *http.Request) {
    id := muxhandlers.RequestIDFromContext(r.Context())
    log.Printf("request %s", id)
}
```

## Request Size Limit Middleware

`RequestSizeLimitMiddleware` limits the size of incoming request
bodies. It wraps `r.Body` with `http.MaxBytesReader`, which returns
413 Request Entity Too Large when the limit is exceeded.

### RequestSizeLimitConfig

| Field | Type | Description |
|-------|------|-------------|
| `MaxBytes` | `int64` | Maximum allowed body size in bytes; must be > 0 |

### RequestSizeLimit Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/upload", handleUpload).Methods(http.MethodPost)

mw, err := muxhandlers.RequestSizeLimitMiddleware(muxhandlers.RequestSizeLimitConfig{
    MaxBytes: 1 << 20, // 1 MiB
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Timeout Middleware

`TimeoutMiddleware` limits handler execution time by wrapping the
handler with `http.TimeoutHandler`. It returns 503 Service
Unavailable when the handler does not complete within the configured
duration.

### TimeoutConfig

| Field | Type | Description |
|-------|------|-------------|
| `Duration` | `time.Duration` | Maximum handler execution time; must be > 0 |
| `Message` | `string` | Custom timeout body; empty = stdlib default |

### Timeout Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.TimeoutMiddleware(muxhandlers.TimeoutConfig{
    Duration: 30 * time.Second,
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Compression Middleware

`CompressionMiddleware` compresses response bodies using gzip or
deflate when the client advertises support via the `Accept-Encoding`
header. Gzip is preferred over deflate when both are accepted.
Quality values (`q=`) are respected for encoding selection. It uses
`sync.Pool` instances to reuse writers for performance. Compression
is skipped for inherently compressed content types (images, video,
audio, archives) and when a `Content-Encoding` is already set.

### CompressionConfig

| Field | Type | Description |
|-------|------|-------------|
| `Level` | `int` | Compression level; 0 = default; `[HuffmanOnly, BestCompression]` |
| `MinLength` | `int` | Minimum body bytes before compressing; 0 = always |

### Compression Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.CompressionMiddleware(muxhandlers.CompressionConfig{
    Level:     gzip.BestSpeed,
    MinLength: 1024,
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Security Headers Middleware

`SecurityHeadersMiddleware` sets common security response headers
with sensible defaults. By default it sets
`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and
`Referrer-Policy: strict-origin-when-cross-origin`. HSTS, CSP,
Permissions-Policy, and Cross-Origin-Opener-Policy are opt-in.

### SecurityHeadersConfig

| Field | Type | Description |
|-------|------|-------------|
| `DisableContentTypeNosniff` | `bool` | Disable nosniff; enabled by default |
| `FrameOption` | `string` | `"DENY"` (default), `"SAMEORIGIN"`, or empty |
| `ReferrerPolicy` | `string` | Defaults to `"strict-origin-when-cross-origin"` |
| `HSTSMaxAge` | `int` | HSTS max-age in seconds; 0 = skip |
| `HSTSIncludeSubDomains` | `bool` | Append `includeSubDomains`; requires `HSTSMaxAge` |
| `HSTSPreload` | `bool` | Append `preload`; requires `HSTSMaxAge` |
| `CrossOriginOpenerPolicy` | `string` | COOP value; empty = skip |
| `ContentSecurityPolicy` | `string` | CSP value; empty = skip |
| `PermissionsPolicy` | `string` | Permissions-Policy value; empty = skip |

### SecurityHeaders Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.SecurityHeadersMiddleware(muxhandlers.SecurityHeadersConfig{
    HSTSMaxAge:            63072000,
    HSTSIncludeSubDomains: true,
    HSTSPreload:           true,
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Method Override Middleware

`MethodOverrideMiddleware` allows clients to override the HTTP method
via a configurable header. By default only `POST` requests are
eligible for override; use `OriginalMethods` to allow other methods.
The first non-empty header value from `HeaderNames` is uppercased and
checked against the allowed set. When allowed, `r.Method` is updated
and the header is removed from the request.

### MethodOverrideConfig

| Field | Type | Description |
|-------|------|-------------|
| `HeaderNames` | `[]string` | Header names checked in order; first non-empty value wins; `nil` = `X-HTTP-Method-Override`, `X-Method-Override`, `X-HTTP-Method` |
| `OriginalMethods` | `[]string` | Methods eligible for override; `nil` = POST |
| `AllowedMethods` | `[]string` | Allowed override methods; `nil` = PUT, PATCH, DELETE, HEAD, OPTIONS |

### MethodOverride Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", updateUser).Methods(http.MethodPut)

mw, err := muxhandlers.MethodOverrideMiddleware(muxhandlers.MethodOverrideConfig{})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Content-Type Check Middleware

`ContentTypeCheckMiddleware` validates that requests carry a matching
`Content-Type` header. Matching is case-insensitive and ignores
parameters such as charset (e.g. `"application/json"` matches
`"application/json; charset=utf-8"`). It returns `415 Unsupported
Media Type` when the `Content-Type` is missing or does not match
any of the allowed types. By default it checks `POST`, `PUT`, and
`PATCH` requests.

### ContentTypeCheckConfig

| Field | Type | Description |
|-------|------|-------------|
| `AllowedTypes` | `[]string` | Acceptable Content-Type values; case-insensitive, ignores params |
| `Methods` | `[]string` | HTTP methods that require validation; `nil` = POST, PUT, PATCH |

### ContentTypeCheck Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", createUser).Methods(http.MethodPost)

mw, err := muxhandlers.ContentTypeCheckMiddleware(muxhandlers.ContentTypeCheckConfig{
    AllowedTypes: []string{"application/json"},
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Server Middleware

`ServerMiddleware` sets server identification response headers. It
sets `X-Server-Hostname` with the machine hostname, resolved once at
factory time via `os.Hostname`. Use the `Hostname` field to provide a
static value instead.

### ServerConfig

| Field | Type | Description |
|-------|------|-------------|
| `Hostname` | `string` | Static hostname value; takes priority over `HostnameEnv` |
| `HostnameEnv` | `[]string` | Environment variable names checked in order (e.g. `["POD_NAME", "HOSTNAME"]`); first non-empty wins; fallback = `os.Hostname()` |

### Server Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.ServerMiddleware(muxhandlers.ServerConfig{})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Cache-Control Middleware

`CacheControlMiddleware` sets `Cache-Control` and `Expires` response
headers based on the response `Content-Type`. Rules are evaluated in
order; the first rule whose `ContentType` prefix matches wins. If no
rule matches and `DefaultValue`/`DefaultExpires` is non-empty, it is
used. When the handler already sets a `Cache-Control` or `Expires`
header, the middleware does not overwrite the respective header.
Matching is case-insensitive.

### CacheControlRule

| Field | Type | Description |
|-------|------|-------------|
| `ContentType` | `string` | Content type prefix to match (e.g. `"image/"`, `"application/json"`) |
| `Value` | `string` | Cache-Control header value to set when this rule matches |
| `Expires` | `time.Duration` | Offset from current time for the Expires header; `0` = epoch (already expired); negative = no header |

### CacheControlConfig

| Field | Type | Description |
|-------|------|-------------|
| `Rules` | `[]CacheControlRule` | Ordered list of rules; first match wins; at least one required |
| `DefaultValue` | `string` | Cache-Control value for unmatched types; empty = no header |
| `DefaultExpires` | `time.Duration` | Default Expires offset for unmatched types; negative = no header |

### CacheControl Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.CacheControlMiddleware(muxhandlers.CacheControlConfig{
    Rules: []muxhandlers.CacheControlRule{
        {ContentType: "image/", Value: "public, max-age=86400", Expires: 24 * time.Hour},
        {ContentType: "application/json", Value: "no-cache", Expires: 0},
    },
    DefaultValue:   "no-store",
    DefaultExpires: 0,
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Static Files Handler

`StaticFilesHandler` serves static files from any `fs.FS` implementation
(`os.DirFS`, `embed.FS`, `fstest.MapFS`, etc.) using `http.FileServerFS`.
It is not middleware — it returns an `http.Handler` that serves files
directly. Directory listing is disabled by default; when a directory has
no `index.html`, a 404 is returned instead of a file listing. When
`SPAFallback` is enabled, requests for non-existent paths serve the root
`index.html`, allowing client-side routers to handle routing.

### StaticFilesConfig

| Field | Type | Description |
|-------|------|-------------|
| `FS` | `fs.FS` | File system to serve files from; required |
| `EnableDirectoryListing` | `bool` | Show directory contents when no `index.html` is present; `false` by default |
| `SPAFallback` | `bool` | Serve root `index.html` for non-existent paths; requires `index.html` at FS root |
| `EnableETag` | `bool` | Precompute strong ETags at init; handles `If-None-Match` (304); designed for `embed.FS` |
| `PathPrefix` | `string` | URL path prefix to strip internally; replaces `http.StripPrefix` |
| `Aliases` | `map[string]string` | Maps URL paths (relative to PathPrefix) to file paths in the FS; targets validated at init; ETag support applies |

### StaticFiles Usage

```go
r := mux.NewRouter()

handler, err := muxhandlers.StaticFilesHandler(muxhandlers.StaticFilesConfig{
    FS: os.DirFS("./public"),
})
if err != nil {
    log.Fatal(err)
}

r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", handler))
```

### StaticFiles Usage with SPA

```go
r := mux.NewRouter()

// API routes first
r.PathPrefix("/api/").Handler(apiRouter)

// SPA handler last — catches all unmatched routes
handler, err := muxhandlers.StaticFilesHandler(muxhandlers.StaticFilesConfig{
    FS:          os.DirFS("./public"),
    SPAFallback: true,
})
if err != nil {
    log.Fatal(err)
}

r.PathPrefix("/").Handler(handler)
```

### StaticFiles Usage with embed.FS

```go
//go:embed static
var staticFS embed.FS

handler, err := muxhandlers.StaticFilesHandler(muxhandlers.StaticFilesConfig{
    FS: staticFS,
})
if err != nil {
    log.Fatal(err)
}

r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", handler))
```

### StaticFiles Usage with ETag

```go
//go:embed static
var staticFS embed.FS

handler, err := muxhandlers.StaticFilesHandler(muxhandlers.StaticFilesConfig{
    FS:         staticFS,
    EnableETag: true,
})
if err != nil {
    log.Fatal(err)
}

r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", handler))
```

### StaticFiles Usage with PathPrefix and Aliases

```go
handler, err := muxhandlers.StaticFilesHandler(muxhandlers.StaticFilesConfig{
    FS:         staticFS,
    PathPrefix: "/ui",
    EnableETag: true,
    Aliases: map[string]string{
        "/policy-builder/":    "policy-builder.html",
        "/policy-playground/": "policy-playground.html",
    },
})
if err != nil {
    log.Fatal(err)
}

r.PathPrefix("/ui/").Handler(handler)
// /ui/policy-builder/    -> policy-builder.html
// /ui/policy-playground/ -> policy-playground.html
// /ui/style.css          -> style.css
```

## Profiler Handler

`RegisterProfiler` registers the standard `net/http/pprof` and `expvar`
endpoints on the given router. It is not middleware — it registers
routes directly. Endpoints use the standard `/debug/pprof/` and
`/debug/vars` paths. Mount with any prefix using `Route`:

### Registered Endpoints

| Suffix | Description |
|--------|-------------|
| `/debug/pprof/` | Index page with links to all profiles |
| `/debug/pprof/cmdline` | Running program command line |
| `/debug/pprof/profile` | CPU profile (supports `?seconds=N`) |
| `/debug/pprof/symbol` | Symbol lookup |
| `/debug/pprof/trace` | Execution trace (supports `?seconds=N`) |
| `/debug/vars` | Exported variables via `expvar` package |

Named profiles (`allocs`, `block`, `goroutine`, `heap`, `mutex`,
`threadcreate`) are served by the index handler.

### Profiler Usage

```go
r := mux.NewRouter()

RegisterProfiler(r)
// serves /debug/pprof/, /debug/vars, etc.
```

### Profiler Usage with custom prefix

```go
r := mux.NewRouter()

r.Route("/_internal", muxhandlers.RegisterProfiler)
// serves /_internal/debug/pprof/, /_internal/debug/vars, etc.
```

## Sunset Middleware

`SunsetMiddleware` sets the `Sunset` response header per
[RFC 8594](https://www.rfc-editor.org/rfc/rfc8594) to indicate that a
resource is expected to become unresponsive at a specific point in time.
Optionally sets the `Deprecation` header and a `Link` header with
`rel="sunset"` pointing to migration documentation.

### SunsetConfig

| Field | Type | Description |
|-------|------|-------------|
| `Sunset` | `time.Time` | When the resource becomes unresponsive; required; serialized as HTTP-date |
| `Deprecation` | `time.Time` | When the resource was deprecated; zero = omit |
| `Link` | `string` | URI to deprecation/migration docs; empty = omit; added as `Link` header with `rel="sunset"` |

### Sunset Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.SunsetMiddleware(muxhandlers.SunsetConfig{
    Sunset:      time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
    Deprecation: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
    Link:        "https://example.com/docs/migration",
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Idempotency Middleware

`IdempotencyMiddleware` caches responses keyed by the `Idempotency-Key` header per
[draft-ietf-httpapi-idempotency-key-header](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/).
Duplicate requests with the same key replay the cached response without
invoking the handler. The middleware requires an `IdempotencyStore`
implementation for persistence.

### IdempotencyStore Interface

```go
type IdempotencyStore interface {
    Get(ctx context.Context, key string) ([]byte, bool)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration)
}
```

### IdempotencyLocker Interface

```go
type IdempotencyLocker interface {
    Lock(ctx context.Context, key string) bool
    Unlock(ctx context.Context, key string)
}
```

### IdempotencyConfig

| Field | Type | Description |
|-------|------|-------------|
| `Store` | `IdempotencyStore` | Backing store for cached responses; required |
| `HeaderName` | `string` | Header name; defaults to `"Idempotency-Key"` |
| `TTL` | `time.Duration` | Cache TTL; defaults to 24 hours; zero = no expiry |
| `Methods` | `[]string` | HTTP methods that require idempotency; `nil` = POST |
| `EnforceKey` | `bool` | Return 400 if header is missing on matched methods |
| `CacheableStatusCodes` | `[]int` | Status codes to cache; `nil` = cache all; e.g. `[]int{200, 201}` |
| `CacheKeyFunc` | `func(*http.Request, string) string` | Custom cache key builder; receives request and raw key; `nil` = default scoping by method + path + key |
| `ValidateKeyFunc` | `func(*http.Request, string) bool` | Key format validator; return false to reject with 400; `nil` = no validation |
| `KeyMaxLength` | `int` | Maximum key length; 0 = default (64); -1 = no limit |
| `CanCache` | `func(*http.Request) bool` | Pre-check before cache lookup/storage; return false to skip caching; `nil` = all matched requests are eligible |
| `OnCacheHit` | `func(*http.Request, string)` | Called on cache hit with request and raw key; use for observability; `nil` = no callback |
| `OnCacheMiss` | `func(*http.Request, string)` | Called on cache miss with request and raw key; use for observability; `nil` = no callback |
| `Locker` | `IdempotencyLocker` | Optional distributed lock for in-flight requests; returns 409 Conflict when lock cannot be acquired; `nil` = no locking |
| `FingerprintFunc` | `func(*http.Request) string` | Computes a request fingerprint; mismatched fingerprint on cache hit returns 422 Unprocessable Entity; `nil` = no fingerprint check |
| `OnConflict` | `func(*http.Request, string)` | Called on 409 Conflict (lock failure); use for observability; `nil` = no callback |
| `OnFingerprintMismatch` | `func(*http.Request, string)` | Called on 422 (fingerprint mismatch); use for observability; `nil` = no callback |
| `RetryAfter` | `time.Duration` | Duration for `Retry-After` header (as whole seconds) on 409 Conflict responses; 0 = no header |
| `ReplayedHeaderName` | `string` | Response header set to `"true"` on replayed responses; empty = no header; e.g. `"X-Idempotency-Replayed"` |
| `ErrorHandler` | `func(http.ResponseWriter, *http.Request, int)` | Custom error writer for 400/409/422 responses; `nil` = plain-text `http.Error` |
| `OnStore` | `func(*http.Request, string, int)` | Called when a response is stored in cache with request, key, and status code; `nil` = no callback |
| `ResponseHeadersFunc` | `func(http.Header, *http.Request, bool)` | Called before writing any response; `replayed` param is true for cached replays; `nil` = no callback |
| `MaxCacheBodySize` | `int64` | Maximum response body size in bytes to cache; larger responses are served but not stored; 0 = no limit |

### Route Metadata

Use the `IdempotencySkipMetadataKey` constant to skip idempotency processing
for specific routes via route metadata:

```go
r.HandleFunc("/health", handler).
    Methods(http.MethodPost).
    Metadata(muxhandlers.IdempotencySkipMetadataKey, true)
```

### Idempotency Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/payments", createPayment).Methods(http.MethodPost)

mw, err := muxhandlers.IdempotencyMiddleware(muxhandlers.IdempotencyConfig{
    Store: redisStore, // implements IdempotencyStore
    TTL:   1 * time.Hour,
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

### Idempotency Usage with EnforceKey

```go
mw, err := muxhandlers.IdempotencyMiddleware(muxhandlers.IdempotencyConfig{
    Store:      redisStore,
    EnforceKey: true, // 400 if Idempotency-Key header is missing
})
```

### Idempotency Usage with CacheKeyFunc (per-user scoping)

```go
mw, err := muxhandlers.IdempotencyMiddleware(muxhandlers.IdempotencyConfig{
    Store: redisStore,
    CacheKeyFunc: func(r *http.Request, key string) string {
        userID := r.Header.Get("X-User-ID")
        return userID + ":" + r.Method + ":" + r.URL.Path + ":" + key
    },
})
```

## Content Negotiation Middleware

`ContentNegotiationMiddleware` performs proactive content negotiation per
[RFC 9110 Section 12.5.1](https://www.rfc-editor.org/rfc/rfc9110#section-12.5.1).
It parses the `Accept` header with quality values, selects the best matching
type from the offered list, and stores the result in the request context.
Returns 406 Not Acceptable when no match is found.

### ContentNegotiationConfig

| Field | Type | Description |
|-------|------|-------------|
| `Offered` | `[]string` | Media types the server can produce, in preference order; empty = accept all |

### ContentNegotiation Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", handler).Methods(http.MethodGet)

r.Use(muxhandlers.ContentNegotiationMiddleware(muxhandlers.ContentNegotiationConfig{
    Offered: []string{"application/json", "application/xml"},
}))
```

### Reading the Negotiated Type

```go
func handler(w http.ResponseWriter, r *http.Request) {
    switch muxhandlers.NegotiatedType(r) {
    case "application/json":
        mux.ResponseJSON(w, http.StatusOK, data)
    case "application/xml":
        mux.ResponseXML(w, http.StatusOK, data)
    }
}
```

### Negotiation Rules

| Accept Header | Behavior |
|---------------|----------|
| absent or empty | First offered type is selected |
| `application/json` | Exact match |
| `text/*` | Matches any `text/` subtype |
| `*/*` | Matches any type; first offered wins |
| `application/json;q=0.5, text/html;q=0.9` | Higher quality wins |
| `application/json;q=0` | Explicitly excluded |
| `text/csv` (not offered) | 406 Not Acceptable |

## Problem Details

`WriteProblemDetails` writes an [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457)
Problem Details JSON response with Content-Type `application/problem+json`.

### ProblemDetails

| Field | Type | Description |
|-------|------|-------------|
| `Type` | `string` | URI identifying the problem type; defaults to `"about:blank"` |
| `Title` | `string` | Short human-readable summary |
| `Status` | `int` | HTTP status code |
| `Detail` | `string` | Human-readable explanation specific to this occurrence |
| `Instance` | `string` | URI identifying the specific occurrence |
| `Extensions` | `map[string]any` | Additional members merged into the top-level JSON object |

### ProblemDetails Usage

```go
func handler(w http.ResponseWriter, r *http.Request) {
    user, err := db.GetUser(id)
    if err != nil {
        muxhandlers.WriteProblemDetails(w, muxhandlers.ProblemDetails{
            Type:   "https://example.com/errors/not-found",
            Title:  "Resource not found",
            Status: http.StatusNotFound,
            Detail: fmt.Sprintf("User with ID %s was not found", id),
        })
        return
    }
}
```

### ProblemDetails with Extensions

```go
muxhandlers.WriteProblemDetails(w, muxhandlers.ProblemDetails{
    Type:   "https://example.com/errors/validation",
    Title:  "Validation Error",
    Status: http.StatusUnprocessableEntity,
    Detail: "One or more fields are invalid",
    Extensions: map[string]any{
        "errors": []map[string]string{
            {"field": "email", "message": "invalid format"},
        },
    },
})
```

### Quick Error Response

`NewProblemDetails` creates a `ProblemDetails` with the status code and
the standard status text as title:

```go
muxhandlers.WriteProblemDetails(w, muxhandlers.NewProblemDetails(http.StatusForbidden))
```

## Early Hints Middleware

`EarlyHintsMiddleware` sends a 103 Early Hints informational response per
[RFC 8297](https://www.rfc-editor.org/rfc/rfc8297) before the final response.
This allows clients to begin preloading resources (stylesheets, scripts, fonts)
while the server is still processing the request.

### EarlyHintsConfig

| Field | Type | Description |
|-------|------|-------------|
| `Links` | `[]string` | Static Link header values per RFC 8288 |
| `LinksFunc` | `func(*http.Request) []string` | Per-request dynamic link computation; results are sent alongside static Links |

Either `Links` or `LinksFunc` (or both) must be set.

### EarlyHints Usage

```go
r := mux.NewRouter()

r.HandleFunc("/", pageHandler).Methods(http.MethodGet)

mw, err := muxhandlers.EarlyHintsMiddleware(muxhandlers.EarlyHintsConfig{
    Links: []string{
        `</style.css>; rel=preload; as=style`,
        `</app.js>; rel=preload; as=script`,
        `</font.woff2>; rel=preload; as=font; crossorigin`,
    },
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

### EarlyHints Usage with embed.FS

```go
//go:embed static
var staticFS embed.FS

// buildLinks walks the embedded FS and returns Link headers for
// known asset types.
func buildLinks() []string {
    asType := map[string]string{
        ".css":   "style",
        ".js":    "script",
        ".woff2": "font",
    }

    var links []string
    fs.WalkDir(staticFS, ".", func(path string, d fs.DirEntry, err error) error {
        if err != nil || d.IsDir() {
            return err
        }
        ext := filepath.Ext(path)
        as, ok := asType[ext]
        if !ok {
            return nil
        }
        link := fmt.Sprintf("<%s>; rel=preload; as=%s", "/"+path, as)
        if as == "font" {
            link += "; crossorigin"
        }
        links = append(links, link)
        return nil
    })
    return links
}

mw, err := muxhandlers.EarlyHintsMiddleware(muxhandlers.EarlyHintsConfig{
    Links: buildLinks(),
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Accept-Patch Middleware

`AcceptPatchMiddleware` handles OPTIONS requests by responding with `Allow` and
`Accept-Patch` headers per [RFC 5789](https://www.rfc-editor.org/rfc/rfc5789#section-3.1).
The `Allow` header is auto-discovered from the router's registered methods for
the matched path. Non-OPTIONS requests pass through unchanged.

### AcceptPatchConfig

| Field | Type | Description |
|-------|------|-------------|
| `AcceptPatchTypes` | `[]string` | Content-Type values for Accept-Patch header; `nil` = `application/json`, `application/merge-patch+json`, `application/json-patch+json` |
| `StatusCode` | `int` | HTTP status for OPTIONS responses; 0 = 204 No Content |

### AcceptPatch Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users/{id}", getUser).Methods(http.MethodGet)
r.HandleFunc("/api/v1/users/{id}", updateUser).Methods(http.MethodPatch)
r.HandleFunc("/api/v1/users/{id}", deleteUser).Methods(http.MethodDelete)

mw := muxhandlers.AcceptPatchMiddleware(r, muxhandlers.AcceptPatchConfig{})

r.Use(mw)
```

## Patch Routing Middleware

`PatchRoutingMiddleware` validates the `Content-Type` of PATCH requests against
a set of allowed patch formats and stores the resolved type in the request
context. Non-PATCH requests pass through unchanged. Returns 415 Unsupported
Media Type when the `Content-Type` is missing or unsupported.

### Patch Content Type Constants

| Constant | Value | Spec |
|----------|-------|------|
| `PatchTypeJSON` | `application/json` | Implicit merge |
| `PatchTypeMergePatch` | `application/merge-patch+json` | [RFC 7396](https://www.rfc-editor.org/rfc/rfc7396) |
| `PatchTypeJSONPatch` | `application/json-patch+json` | [RFC 6902](https://www.rfc-editor.org/rfc/rfc6902) |

### PatchRoutingConfig

| Field | Type | Description |
|-------|------|-------------|
| `AllowedTypes` | `[]string` | Accepted Content-Type values; `nil` = all three defaults |

### PatchRouting Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users/{id}", updateUser).Methods(http.MethodPatch)

r.Use(muxhandlers.PatchRoutingMiddleware(muxhandlers.PatchRoutingConfig{}))
```

### Reading the Patch Type from context

```go
func updateUser(w http.ResponseWriter, r *http.Request) {
    switch muxhandlers.PatchContentType(r) {
    case muxhandlers.PatchTypeJSON:
        // implicit merge: sent fields overwrite, absent fields unchanged
    case muxhandlers.PatchTypeMergePatch:
        // RFC 7396: null values mean "remove this field"
    case muxhandlers.PatchTypeJSONPatch:
        // RFC 6902: array of add/remove/replace/move/copy/test operations
    }
}
```

## Redirect Middleware

`RedirectMiddleware` redirects requests based on path matching rules. It
supports exact path matching and prefix matching with a trailing wildcard
(`*`). The first matching rule wins. Non-matching requests pass through.
The redirect response includes a `Location` header and an HTML body with
a `<meta http-equiv="refresh">` tag for clients that do not follow the
`Location` header automatically.

### RedirectRule

| Field | Type | Description |
|-------|------|-------------|
| `From` | `string` | Path to match; must start with `/`; trailing `*` enables prefix matching |
| `To` | `string` | Redirect target; suffix appended for wildcard rules; can be an absolute URL |
| `StatusCode` | `int` | Per-rule HTTP redirect status code; 0 = use config default |

### RedirectConfig

| Field | Type | Description |
|-------|------|-------------|
| `Rules` | `[]RedirectRule` | Ordered list of rules; first match wins; required |
| `StatusCode` | `int` | Default HTTP redirect status code; 0 = 307 Temporary Redirect |

### Redirect Usage

```go
r := mux.NewRouter()

r.HandleFunc("/swagger/", swaggerHandler).Methods(http.MethodGet)
r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.RedirectMiddleware(muxhandlers.RedirectConfig{
    Rules: []muxhandlers.RedirectRule{
        {From: "/", To: "/swagger/"},
        {From: "/old-page", To: "/new-page"},
        {From: "/blog/2023/*", To: "/archive/2023/"},
        {From: "/github", To: "https://github.com/example", StatusCode: http.StatusMovedPermanently},
    },
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Canonical Host Middleware

`CanonicalHostMiddleware` redirects requests to a canonical host URL when
the incoming scheme or host does not match. The request path and query
string are preserved. Useful for enforcing a single canonical URL
(e.g. `example.com` to `www.example.com`, or HTTP to HTTPS).

### CanonicalHostConfig

| Field | Type | Description |
|-------|------|-------------|
| `URL` | `string` | Canonical base URL including scheme and host; required |
| `StatusCode` | `int` | HTTP redirect status code; 0 = 301 Moved Permanently |

### CanonicalHost Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.CanonicalHostMiddleware(muxhandlers.CanonicalHostConfig{
    URL: "https://www.example.com",
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## IP Allow Middleware

`IPAllowMiddleware` restricts access to requests originating from a
configured set of IP addresses and CIDR ranges. Requests from IPs not
in the allowed list are rejected with 403 Forbidden by default. The
client IP is extracted from `r.RemoteAddr`.

### IPAllowConfig

| Field | Type | Description |
|-------|------|-------------|
| `Allowed` | `[]string` | IP addresses and CIDR ranges that are permitted; required |
| `DeniedHandler` | `http.Handler` | Custom handler for denied requests; `nil` = 403 Forbidden with empty body |

### IPAllow Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

mw, err := muxhandlers.IPAllowMiddleware(muxhandlers.IPAllowConfig{
    Allowed: []string{"10.0.0.0/8", "192.168.0.0/16", "::1"},
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

### IPAllow Usage with custom denied handler

```go
mw, err := muxhandlers.IPAllowMiddleware(muxhandlers.IPAllowConfig{
    Allowed: []string{"10.0.0.0/8"},
    DeniedHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusForbidden)
        w.Write([]byte(`{"error":"access denied"}`))
    }),
})
if err != nil {
    log.Fatal(err)
}

r.Use(mw)
```

## Access Log Middleware

`AccessLogMiddleware` records a structured entry for every request,
capturing the response status code and byte count via a wrapped
`http.ResponseWriter`. By default entries are emitted through `log/slog`
(`slog.Default` when no `Logger` is provided). The `Logger` field accepts
a fully pre-configured parent `*slog.Logger` — the middleware inherits
its handler, output, format, level, and pre-bound attributes (from
`Logger.With` or `Logger.WithGroup`), and appends per-request fields to
every emitted record. Set `LogFunc` to bypass slog entirely and route
entries to a custom sink.

5xx responses are logged at `slog.LevelError`; otherwise-Info requests
are escalated to `slog.LevelWarn` when their duration exceeds
`SlowThreshold`. Use `Skip` to suppress logging for health checks or
metrics endpoints. Header capture is opt-in via `IncludeHeaders`, and
`Authorization`, `Cookie`, `Proxy-Authorization`, and `Set-Cookie` are
always redacted when captured.

### AccessLogConfig

| Field | Type | Description |
|-------|------|-------------|
| `Logger` | `*slog.Logger` | Pre-configured parent; inherits handler/output/format/level and `.With`/`.WithGroup` attrs; defaults to `slog.Default()` |
| `LogFunc` | `func(*AccessLogEntry)` | Custom sink; bypasses slog entirely when set |
| `Skip` | `func(*mux.Router, *http.Request) bool` | Return true to suppress logging; receives the router so it can inspect matched route metadata |
| `IncludeHeaders` | `[]string` | Request headers to capture; case-insensitive; `nil` = none |
| `RedactHeaders` | `[]string` | Additional headers to redact; baseline is `Authorization`, `Cookie`, `Proxy-Authorization`, `Set-Cookie` |
| `SlowThreshold` | `time.Duration` | Escalate Info → Warn when handler runs longer than this; `0` = disabled |
| `Now` | `func() time.Time` | Clock source; `nil` = `time.Now`; intended for tests |

### AccessLogEntry

| Field | Type | Description |
|-------|------|-------------|
| `Time` | `time.Time` | When the request started |
| `Method` | `string` | HTTP method |
| `Proto` | `string` | `r.Proto` (`HTTP/1.1`, `HTTP/2.0`) |
| `Scheme` | `string` | Resolved via `mux.Scheme(r)` (`http`/`https`) |
| `Host` | `string` | `r.Host` (post-proxy resolution when `ProxyHeadersMiddleware` is upstream) |
| `Path` | `string` | `r.URL.Path` at handler entry |
| `Query` | `string` | `r.URL.RawQuery` |
| `Status` | `int` | Status code; defaults to 200 when handler exits without writing; 0 when `Hijacked` is true |
| `Hijacked` | `bool` | True when the handler took over the connection via `http.Hijacker` (e.g. WebSocket upgrade); slog output emits `hijacked=true` instead of `status` |
| `Bytes` | `int64` | Response body bytes written |
| `Duration` | `time.Duration` | Handler execution time |
| `RemoteAddr` | `string` | `r.RemoteAddr` (use `ProxyHeadersMiddleware` to resolve) |
| `UserAgent` | `string` | `User-Agent` header |
| `Referer` | `string` | `Referer` header |
| `RouteName` | `string` | Name from `mux.Route.Name`, if any |
| `RequestID` | `string` | Result of `RequestIDFromContext`, if any |
| `Headers` | `map[string]string` | Captured headers with redaction applied |

### AccessLog Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

r.Use(muxhandlers.AccessLogMiddleware(r, muxhandlers.AccessLogConfig{
    SlowThreshold: 500 * time.Millisecond,
    Skip: func(_ *mux.Router, req *http.Request) bool {
        return req.URL.Path == "/healthz"
    },
}))
```

### AccessLog Usage with route metadata

```go
const skipLogKey = "access_log_skip"

r := mux.NewRouter()

r.Use(muxhandlers.AccessLogMiddleware(r, muxhandlers.AccessLogConfig{
    Skip: func(_ *mux.Router, req *http.Request) bool {
        route := mux.CurrentRoute(req)
        if route == nil {
            return false
        }
        skip, _ := route.GetMetadataValueOr(skipLogKey, false).(bool)
        return skip
    },
}))

// Tag noisy routes inline; the predicate above suppresses their logs.
r.HandleFunc("/metrics", metricsHandler).Metadata(skipLogKey, true)
r.HandleFunc("/healthz", healthHandler).Metadata(skipLogKey, true)
r.HandleFunc("/api/v1/users", listUsers)
```

### AccessLog Usage with a pre-configured parent logger

```go
// Build the application's parent logger once: handler, output, format,
// level, and any baseline attrs that should appear on every record.
parent := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})).With(
    slog.String("service", "auth-api"),
    slog.String("env", "prod"),
)

// AccessLogMiddleware inherits the parent's handler and pre-bound attrs;
// every emitted record carries `service` and `env` alongside the
// per-request access-log fields.
r.Use(muxhandlers.AccessLogMiddleware(r, muxhandlers.AccessLogConfig{
    Logger:         parent,
    IncludeHeaders: []string{"X-Tenant-ID", "X-Forwarded-For"},
}))
```

### AccessLog Usage with custom sink

```go
r.Use(muxhandlers.AccessLogMiddleware(r, muxhandlers.AccessLogConfig{
    LogFunc: func(e *muxhandlers.AccessLogEntry) {
        // ship to logging backend
        metrics.RecordRequest(e.RouteName, e.Status, e.Duration)
    },
}))
```

## Graceful Shutdown Middleware

`GracefulShutdownMiddleware` returns the middleware together with a
`*Drainer` control surface. Requests arriving before `Drain()` flow
through unchanged and are counted in `Drainer.InFlight`; requests
arriving after `Drain()` receive a `503 Service Unavailable` with
`Connection: close` so keep-alive clients reconnect to a healthy peer.
`Bypass` forwards selected requests (typically `/healthz`, `/readyz`,
`/metrics`) so the orchestrator can observe the drain. `Drainer.Wait`
blocks until in-flight requests have completed or the supplied context
fires, which is the natural pair for `http.Server.Shutdown`.

### GracefulShutdownConfig

| Field | Type | Description |
|-------|------|-------------|
| `Bypass` | `func(*mux.Router, *http.Request) bool` | Return true to forward a request to the next handler during drain |
| `Response` | `http.Handler` | Custom drain response; default headers are still applied before invocation |
| `StatusCode` | `int` | Status for the default response; defaults to 503 |
| `RetryAfter` | `time.Duration` | `Retry-After` header as delta-seconds; sub-second values round up to 1 |

### Drainer

| Method | Description |
|--------|-------------|
| `Drain()` | Begin rejecting new requests. Idempotent. |
| `IsDraining() bool` | Report whether Drain has been called |
| `InFlight() int64` | Number of requests currently inside the middleware |
| `Wait(ctx context.Context) error` | Block until `InFlight` reaches zero or ctx is cancelled; returns `ctx.Err()` on cancel |

### Graceful Shutdown Usage

```go
r := mux.NewRouter()

mw, drainer := muxhandlers.GracefulShutdownMiddleware(r, muxhandlers.GracefulShutdownConfig{
    RetryAfter: 15 * time.Second,
    Bypass: func(_ *mux.Router, req *http.Request) bool {
        return req.URL.Path == "/healthz" || req.URL.Path == "/readyz"
    },
})
r.Use(mw)

srv := &http.Server{Addr: ":8080", Handler: r}

go func() {
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatal(err)
    }
}()

stop := make(chan os.Signal, 1)
signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
<-stop

drainer.Drain()

shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := drainer.Wait(shutdownCtx); err != nil {
    log.Printf("drain timed out with %d requests still in flight", drainer.InFlight())
}
_ = srv.Shutdown(shutdownCtx)
```

### Graceful Shutdown Usage with custom response

```go
mw, drainer := muxhandlers.GracefulShutdownMiddleware(r, muxhandlers.GracefulShutdownConfig{
    Response: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusServiceUnavailable)
        _, _ = w.Write([]byte(`{"draining":true}`))
    }),
})

r.Use(mw)
```

## Maintenance Mode Middleware

`MaintenanceModeMiddleware` short-circuits matching requests with a
`503 Service Unavailable` response
([RFC 9110 Section 15.6.4](https://www.rfc-editor.org/rfc/rfc9110#section-15.6.4))
while a maintenance window is active. The `Enabled` predicate is the
single source of truth; back it with whatever you like (`atomic.Bool`,
file presence, env var, cron window). `Bypass` lets specific requests
through during maintenance (admin IPs, deploy tooling, health checks).
`Response`, when set, fully owns the response body so callers can render
an HTML maintenance page, return RFC 9457 problem JSON, or redirect to
a static page. `RetryAfter` / `RetryAt` populate the `Retry-After`
header in either delta-seconds or HTTP-date form
([RFC 9110 Section 10.2.3](https://www.rfc-editor.org/rfc/rfc9110#section-10.2.3)).

### MaintenanceConfig

| Field | Type | Description |
|-------|------|-------------|
| `Enabled` | `func(*http.Request) bool` | Required; returning true triggers maintenance for the request. `nil` = middleware is a no-op |
| `Bypass` | `func(*mux.Router, *http.Request) bool` | Return true to forward the request to the next handler even while Enabled is true |
| `Response` | `http.Handler` | Custom maintenance response; when set, `StatusCode` is ignored |
| `StatusCode` | `int` | Status for the default response; defaults to 503 |
| `RetryAfter` | `time.Duration` | `Retry-After` as delta-seconds; sub-second values round up to 1 |
| `RetryAt` | `time.Time` | `Retry-After` as HTTP-date in UTC; takes precedence over `RetryAfter` |

### Maintenance Usage with atomic.Bool

```go
var inMaintenance atomic.Bool

r := mux.NewRouter()

r.Use(muxhandlers.MaintenanceModeMiddleware(r, muxhandlers.MaintenanceConfig{
    Enabled:    func(_ *http.Request) bool { return inMaintenance.Load() },
    RetryAfter: 5 * time.Minute,
}))

// Flip from anywhere: signal handler, admin endpoint, deploy script, etc.
inMaintenance.Store(true)
```

### Maintenance Usage with custom HTML page

```go
tmpl := template.Must(template.New("maint").Parse(`<!doctype html>
<title>Maintenance</title>
<h1>We'll be right back</h1>
<p>Estimated end: {{.End}}</p>`))

end := time.Now().Add(15 * time.Minute)

r.Use(muxhandlers.MaintenanceModeMiddleware(r, muxhandlers.MaintenanceConfig{
    Enabled: func(_ *http.Request) bool { return inMaintenance.Load() },
    RetryAt: end,
    Response: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.WriteHeader(http.StatusServiceUnavailable)
        _ = tmpl.Execute(w, struct{ End string }{end.UTC().Format(time.RFC1123)})
    }),
}))
```

### Maintenance Usage with admin bypass and route metadata

```go
const exemptKey = "exempt_from_maintenance"

r.Use(muxhandlers.MaintenanceModeMiddleware(r, muxhandlers.MaintenanceConfig{
    Enabled: func(_ *http.Request) bool { return inMaintenance.Load() },
    Bypass: func(_ *mux.Router, req *http.Request) bool {
        if req.Header.Get("X-Admin-Token") == adminToken {
            return true
        }
        route := mux.CurrentRoute(req)
        if route == nil {
            return false
        }
        exempt, _ := route.GetMetadataValueOr(exemptKey, false).(bool)
        return exempt
    },
}))

// Health checks and metrics scrapes stay reachable during maintenance.
r.HandleFunc("/healthz", healthHandler).Metadata(exemptKey, true)
r.HandleFunc("/metrics", metricsHandler).Metadata(exemptKey, true)
r.HandleFunc("/api/v1/users", listUsers)
```

## No-Cache Middleware

`NoCacheMiddleware` forces responses to be uncacheable. It rewrites
caching headers on the response writer at the moment the handler
flushes its status line, overriding any `Cache-Control`, `Pragma`, or
`Expires` the handler may have set, and removes `ETag` and
`Last-Modified` so downstream caches cannot perform conditional
revalidation. The Modern preset emits `Cache-Control: no-store` per
RFC 9111 Section 5.2.2.5; Strict adds the legacy `Pragma` and `Expires`
combo for HTTP/1.0-era intermediaries.

### NoCachePreset

| Preset | Emitted Headers |
|--------|-----------------|
| `NoCachePresetModern` (default) | `Cache-Control: no-store` |
| `NoCachePresetStrict` | `Cache-Control: no-store, no-cache, must-revalidate, max-age=0, private`; `Pragma: no-cache`; `Expires: 0` |

Both presets strip `ETag` and `Last-Modified` from the response.

### NoCacheConfig

| Field | Type | Description |
|-------|------|-------------|
| `Preset` | `NoCachePreset` | Header set to emit; defaults to `NoCachePresetModern` |
| `Skip` | `func(*mux.Router, *http.Request) bool` | Return true to leave the handler's caching headers untouched |

### NoCache Usage

```go
r := mux.NewRouter()

r.HandleFunc("/api/v1/users", listUsers).Methods(http.MethodGet)

r.Use(muxhandlers.NoCacheMiddleware(r, muxhandlers.NoCacheConfig{}))
```

### NoCache Usage with strict preset

```go
r.Use(muxhandlers.NoCacheMiddleware(r, muxhandlers.NoCacheConfig{
    Preset: muxhandlers.NoCachePresetStrict,
}))
```

### NoCache Usage with route metadata opt-out

```go
const allowCacheKey = "allow_cache"

r := mux.NewRouter()

r.Use(muxhandlers.NoCacheMiddleware(r, muxhandlers.NoCacheConfig{
    Skip: func(_ *mux.Router, req *http.Request) bool {
        route := mux.CurrentRoute(req)
        if route == nil {
            return false
        }
        allow, _ := route.GetMetadataValueOr(allowCacheKey, false).(bool)
        return allow
    },
}))

// Most routes get no-store; tag specific ones to keep their own cache headers.
r.HandleFunc("/api/v1/users", dynamicHandler)
r.HandleFunc("/assets/logo.png", staticHandler).Metadata(allowCacheKey, true)
```

## HTCPCP Middleware

`HTCPCPMiddleware` implements the Hyper Text Coffee Pot Control Protocol
([RFC 2324](https://www.rfc-editor.org/rfc/rfc2324)) extended for tea
efflux appliances ([RFC 7168](https://www.rfc-editor.org/rfc/rfc7168)).
It intercepts `BREW` and `WHEN` requests and responds according to the
configured pot type. Non-HTCPCP methods pass through unchanged.

By default the middleware only activates on April 1 (the publication
date of both RFCs); on every other day it becomes a no-op. Override
`ActiveOn` to force-enable the protocol or restrict it further.

### Response Matrix

| Pot | Request | Response |
|-----|---------|----------|
| Teapot | `BREW` without `tea-*` addition | `418 I'm a Teapot` |
| Teapot | `BREW` with supported `tea-*` variety | `200` + `message/teapot` |
| Teapot | `BREW` with unsupported tea variety | `406 Not Acceptable` |
| Coffee pot | `BREW` with `tea-*` addition | `406 Not Acceptable` |
| Coffee pot | `BREW` without tea | `200` + `message/coffeepot` |
| Either | `BREW` with `Empty: true` | `503` + `Retry-After` |
| Either | `BREW` with addition not in `AvailableAdditions` | `406 Not Acceptable` |
| Either | `WHEN` | `200` + `message/coffeepot` |
| Either | Other methods (`GET`, `POST`, ...) | passthrough |

### HTCPCPConfig

| Field | Type | Description |
|-------|------|-------------|
| `PotType` | `PotType` | `PotCoffee` (default) or `PotTeapot` |
| `Teas` | `[]string` | Tea varieties this teapot can brew; `nil` for a teapot uses `DefaultTeaVarieties` |
| `AvailableAdditions` | `[]string` | Available additions per RFC 2324 Section 2.2.2.1; requests for other additions return 406 |
| `Empty` | `bool` | When true, BREW returns 503 with `Retry-After` |
| `RetryAfter` | `int` | `Retry-After` value in seconds; defaults to 60 |
| `ActiveOn` | `func(time.Time) bool` | Predicate gating the middleware; `nil` = `IsAprilFirst` |
| `Now` | `func() time.Time` | Clock source; `nil` = `time.Now`; intended for tests |

### DefaultTeaVarieties

`black`, `chai`, `earl-grey`, `english-breakfast`, `green`, `jasmine`,
`oolong`, `peppermint`, `rooibos` (RFC 7168 Section 2.1.1).

### HTCPCP Usage

```go
r := mux.NewRouter()

r.Route("/pot", func(pot *mux.Router) {
    pot.Use(muxhandlers.HTCPCPMiddleware(muxhandlers.HTCPCPConfig{
        PotType: muxhandlers.PotTeapot,
        Teas:    []string{"earl-grey", "rooibos"},
    }))
    pot.HandleFunc("/", potStatusHandler)
})
```

### HTCPCP Usage with always-on override

```go
mw := muxhandlers.HTCPCPMiddleware(muxhandlers.HTCPCPConfig{
    PotType:  muxhandlers.PotCoffee,
    ActiveOn: func(_ time.Time) bool { return true }, // year-round
})

r.Use(mw)
```
