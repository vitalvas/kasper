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
