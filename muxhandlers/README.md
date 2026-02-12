# muxhandlers

HTTP middleware handlers for the `mux` router.

## Installation

```bash
go get github.com/vitalvas/kasper/muxhandlers
```

## CORS Middleware

`CORSMiddleware` implements the full CORS protocol. It validates the `Origin` header, handles preflight `OPTIONS` requests, and sets the appropriate response headers.

### CORSConfig

| Field | Type | Description |
|-------|------|-------------|
| `AllowedOrigins` | `[]string` | Exact origins, `"*"` for wildcard, or subdomain patterns like `"https://*.example.com"` |
| `AllowOriginFunc` | `func(string) bool` | Optional dynamic origin check (receives raw origin) |
| `AllowedMethods` | `[]string` | Override methods; empty = auto-discover from router |
| `AllowedHeaders` | `[]string` | Preflight allowed headers; empty = reflect request; `"*"` = reflect all requested headers |
| `ExposeHeaders` | `[]string` | Headers exposed to client code |
| `AllowCredentials` | `bool` | Send `Access-Control-Allow-Credentials: true` |
| `MaxAge` | `int` | Preflight cache seconds; 0 = omit, negative = `"0"` |
| `OptionsStatusCode` | `int` | Preflight HTTP status; 0 = 204 No Content |
| `OptionsPassthrough` | `bool` | Forward preflight to next handler after setting CORS headers |
| `AllowPrivateNetwork` | `bool` | Respond to `Access-Control-Request-Private-Network` with allow header |

### Usage

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

`BasicAuthMiddleware` implements HTTP Basic Authentication per [RFC 7617](https://www.rfc-editor.org/rfc/rfc7617). Credentials can be validated via a dynamic callback (`ValidateFunc`) or a static map (`Credentials`). When both are set, `ValidateFunc` takes priority. Static credential comparison uses `crypto/subtle.ConstantTimeCompare` to prevent timing attacks.

### BasicAuthConfig

| Field | Type | Description |
|-------|------|-------------|
| `Realm` | `string` | Authentication realm for `WWW-Authenticate` header; defaults to `"Restricted"` |
| `ValidateFunc` | `func(string, string) bool` | Dynamic credential validation callback (username, password); takes priority over `Credentials` |
| `Credentials` | `map[string]string` | Static username-to-password map; compared with constant-time comparison |

### Usage with ValidateFunc

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

### Usage with Credentials

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

`ProxyHeadersMiddleware` populates request fields from reverse proxy headers when the request originates from a trusted proxy. A trusted proxy list (IPs and CIDRs) restricts which peers are allowed to set these headers, preventing spoofing from untrusted clients. When `TrustedProxies` is empty, `DefaultTrustedProxies` (RFC 1918, RFC 4193, and loopback ranges) is used.

### Supported Headers

| Field | Headers (priority order) |
|-------|--------------------------|
| `r.RemoteAddr` | `X-Forwarded-For` > `X-Real-IP` > `Forwarded: for=`* |
| `r.URL.Scheme` | `X-Forwarded-Proto` > `X-Forwarded-Scheme` > `Forwarded: proto=`* |
| `r.Host` | `X-Forwarded-Host` > `Forwarded: host=`* |
| `X-Forwarded-By` header | `Forwarded: by=`* |

*Requires `EnableForwarded: true` ([RFC 7239](https://www.rfc-editor.org/rfc/rfc7239)).

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
| `TrustedProxies` | `[]string` | IP addresses and CIDR ranges of trusted proxies; empty = `DefaultTrustedProxies` |
| `EnableForwarded` | `bool` | Parse RFC 7239 `Forwarded` header as lowest-priority fallback |

### Usage

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
