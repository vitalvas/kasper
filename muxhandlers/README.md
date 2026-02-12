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
