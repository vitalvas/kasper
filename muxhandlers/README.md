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
