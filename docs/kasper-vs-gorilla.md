# Kasper vs Gorilla Toolkit

Kasper is a drop-in replacement for the Gorilla web toolkit.
All Gorilla code works unchanged after swapping import paths.
Kasper adds features on top without breaking the existing API.

## Package Mapping

| Gorilla Package | Kasper Package | Compatibility |
|-----------------|----------------|---------------|
| `gorilla/mux` | `kasper/mux` | Full API compatible + extensions |
| `gorilla/websocket` | `kasper/websocket` | Full API compatible + extensions |
| `gorilla/handlers` | `kasper/muxhandlers` | New implementation, different API |
| -- | `kasper/openapi` | No Gorilla equivalent |
| -- | `kasper/httpsig` | No Gorilla equivalent |

## Migration

Replace import paths:

```go
// Before
import "github.com/gorilla/mux"
import "github.com/gorilla/websocket"
import "github.com/gorilla/handlers"

// After
import "github.com/vitalvas/kasper/mux"
import "github.com/vitalvas/kasper/websocket"
import "github.com/vitalvas/kasper/muxhandlers" // API differs, see below
```

`kasper/mux` and `kasper/websocket` are drop-in replacements with no code changes
required. `kasper/muxhandlers` has a different API from `gorilla/handlers` and
requires migration.

---

## mux

### Feature Comparison

| Feature | gorilla/mux | kasper/mux |
|---------|:-----------:|:----------:|
| **Routing** | | |
| Path variables (`{name}`, `{id:[0-9]+}`) | Yes | Yes |
| Pattern macros (`{id:uuid}`, `{n:int}`) | No | Yes |
| Host-based routing | Yes | Yes |
| Method matching | Yes | Yes |
| Header matching (exact and regex) | Yes | Yes |
| Query parameter matching | Yes | Yes |
| Scheme matching | Yes | Yes |
| Custom matcher functions | Yes | Yes |
| Strict slash redirects | Yes | Yes |
| Path cleaning (dot segments) | Yes | Yes |
| Encoded path matching | Yes | Yes |
| **Subrouters** | | |
| PathPrefix subrouters | Yes | Yes |
| Subrouter NotFoundHandler | Yes | Yes |
| Inline subrouters (`Route`, `Group`) | No | Yes |
| **Middleware** | | |
| Router-level middleware (`Use`) | Yes | Yes |
| Route-level middleware (`Route.Use`) | No | Yes |
| Inline middleware (`With`) | No | Yes |
| Chained inline middleware | No | Yes |
| **Route Metadata** | | |
| Static key-value metadata (`Metadata`) | Yes (unreleased) | Yes |
| `GetMetadata`, `MetadataContains` | Yes (unreleased) | Yes |
| `GetMetadataValue`, `GetMetadataValueOr` | Yes (unreleased) | Yes |
| `ErrMetadataKeyNotFound` | Yes (unreleased) | Yes |
| Bulk metadata (`MetadataMap`) | No | Yes |
| Dynamic metadata (`MetadataFunc`) | No | Yes |
| Request metadata accessor (`RequestMetadata`) | No | Yes |
| **Request/Response Helpers** | | |
| JSON request binding (`BindJSON`) | No | Yes |
| XML request binding (`BindXML`) | No | Yes |
| JSON response writing (`ResponseJSON`) | No | Yes |
| XML response writing (`ResponseXML`) | No | Yes |
| Typed JSON handlers (`HandleJSON`) | No | Yes |
| Response-only JSON handlers (`HandleJSONResponse`) | No | Yes |
| Content-Type constants | No | Yes |
| **Context Functions** | | |
| `Vars(r)` | Yes | Yes |
| `VarGet(r, name)` | No | Yes |
| `CurrentRoute(r)` | Yes | Yes |
| `CurrentRouter(r)` | No | Yes |
| `SetURLVars(r, vars)` | Yes | Yes |
| `RequestMetadata(r)` | No | Yes |
| **URL Building** | | |
| Named routes | Yes | Yes |
| `URL`, `URLHost`, `URLPath` | Yes | Yes |
| Build-only routes | Yes | Yes |
| **Route Inspection** | | |
| `GetPathTemplate`, `GetPathRegexp` | Yes | Yes |
| `GetHostTemplate`, `GetHostRegexp` | Yes | Yes |
| `GetMethods`, `GetSchemes` | Yes | Yes |
| `GetHeaders`, `GetHeadersRegexp` | Yes | Yes |
| `GetQueriesTemplates`, `GetQueriesRegexp` | Yes | Yes |
| `GetVarNames`, `GetName`, `GetError` | Yes | Yes |
| `GetHandler`, `GetHandlerWithMiddlewares` | No | Yes |
| **Walk** | Yes | Yes |
| **Custom regexp compiler** (`RegexpCompileFunc`) | No | Yes |
| **Error sentinels** | | |
| `ErrMethodMismatch` | Yes | Yes |
| `ErrNotFound` | Yes | Yes |
| `ErrMetadataKeyNotFound` | No | Yes |
| `SkipRouter` | Yes | Yes |

### Pattern Macros

gorilla/mux requires raw regular expressions. kasper/mux adds named macros:

```go
// gorilla/mux
r.HandleFunc("/users/{id:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}", handler)
r.HandleFunc("/articles/{page:[0-9]+}", handler)

// kasper/mux
r.HandleFunc("/users/{id:uuid}", handler)
r.HandleFunc("/articles/{page:int}", handler)
```

| Macro | Description | Example match |
|-------|-------------|---------------|
| `uuid` | RFC 4122 UUID | `550e8400-e29b-41d4-a716-446655440000` |
| `int` | Unsigned integer | `42` |
| `float` | Decimal number | `3.14`, `.5` |
| `slug` | URL-safe slug | `my-post-title` |
| `alpha` | Alphabetic characters | `hello` |
| `alphanum` | Alphanumeric characters | `abc123` |
| `date` | ISO 8601 date | `2024-01-15` |
| `hex` | Hexadecimal string | `deadBEEF` |
| `domain` | RFC 1123 domain name | `sub.example.co.uk` |

If the name after the colon does not match a known macro, it is treated as a raw regular expression for full backward compatibility.

### Inline Subrouters

```go
// gorilla/mux
api := r.PathPrefix("/api/v1").Subrouter()
api.Use(authMiddleware)
api.HandleFunc("/users", listUsers).Methods(http.MethodGet)
api.HandleFunc("/users/{id}", getUser).Methods(http.MethodGet)

// kasper/mux
r.Route("/api/v1", func(api *mux.Router) {
    api.Use(authMiddleware)
    api.HandleFunc("/users", listUsers).Methods(http.MethodGet)
    api.HandleFunc("/users/{id}", getUser).Methods(http.MethodGet)
})
```

`Group` works the same way but without a path prefix, for grouping routes that share middleware:

```go
r.Group(func(authed *mux.Router) {
    authed.Use(authMiddleware)
    authed.HandleFunc("/dashboard", dashboardHandler)
    authed.HandleFunc("/settings", settingsHandler)
})
```

Both return the parent router for chaining.

### Inline Middleware

gorilla/mux applies middleware to all routes on a router. kasper/mux adds `With` for per-route middleware:

```go
// gorilla/mux - middleware affects all routes on the router
r.Use(authMiddleware)
r.HandleFunc("/public", publicHandler)  // also gets auth middleware
r.HandleFunc("/secret", secretHandler)

// kasper/mux - middleware on specific routes only
r.With(authMiddleware).HandleFunc("/secret", secretHandler)
r.HandleFunc("/public", publicHandler)  // no auth middleware
```

`With` calls can be chained and compose with `Route`, `Group`, and `Use`:

```go
r.Use(loggingMiddleware)
r.With(authMiddleware).Route("/admin", func(admin *mux.Router) {
    admin.HandleFunc("/users", usersHandler)
})
// Order: logging -> auth -> usersHandler
```

### Route-Level Middleware

```go
r.HandleFunc("/admin", adminHandler).Use(auditMiddleware)
```

Execution order: router middleware -> `With` middleware -> route middleware -> handler.

### Route Metadata

gorilla/mux has basic metadata support on main (unreleased, not in v1.8.1):
`Metadata`, `GetMetadata`, `MetadataContains`, `GetMetadataValue`,
`GetMetadataValueOr`, `ErrMetadataKeyNotFound`. kasper/mux includes all of
these and adds `MetadataMap`, `MetadataFunc`, and `RequestMetadata`.

```go
// Works in both gorilla/mux (main) and kasper/mux
r.HandleFunc("/admin/users", handler).
    Metadata("role", "admin").
    Metadata("rateLimit", 100)

route := mux.CurrentRoute(r)
role, _ := route.GetMetadataValue("role")
limit := route.GetMetadataValueOr("rateLimit", 60)
```

kasper/mux additions:

```go
// Bulk metadata
r.HandleFunc("/admin/users", handler).
    MetadataMap(map[any]any{"role": "admin", "rateLimit": 100})

// Dynamic metadata based on request
r.HandleFunc("/users", handler).
    Metadata("static", "value").
    MetadataFunc(func(r *http.Request) map[any]any {
        return map[any]any{"lang": r.Header.Get("Accept-Language")}
    })

// Merged metadata in handler (static + dynamic)
md := mux.RequestMetadata(r)
```

### Request Binding and Response Helpers

```go
// Decode JSON body (rejects unknown fields by default)
var req CreateUserRequest
if err := mux.BindJSON(r, &req); err != nil {
    http.Error(w, err.Error(), http.StatusBadRequest)
    return
}

// Encode JSON response
mux.ResponseJSON(w, http.StatusOK, user)
```

### Typed JSON Handlers

```go
h := mux.HandleJSON(
    func(w http.ResponseWriter, r *http.Request, in CreateReq) (CreateResp, error) {
        return CreateResp{ID: uuid.New().String(), Name: in.Name}, nil
    },
    func(w http.ResponseWriter, r *http.Request, err error) {
        http.Error(w, err.Error(), http.StatusBadRequest)
    },
)
r.Handle("/users", h).Methods(http.MethodPost)
```

### Context Helpers

```go
// gorilla/mux
vars := mux.Vars(r)
id := vars["id"]

// kasper/mux (also available, more concise)
id, ok := mux.VarGet(r, "id")

// kasper/mux only
router := mux.CurrentRouter(r)
md := mux.RequestMetadata(r)
```

---

## websocket

### WebSocket Feature Comparison

| Feature | gorilla/websocket | kasper/websocket |
|---------|:-----------------:|:----------------:|
| **Connection** | | |
| Server-side upgrade (`Upgrader`) | Yes | Yes |
| Client-side dial (`Dialer`) | Yes | Yes |
| Custom headers on upgrade | Yes | Yes |
| Origin validation (`CheckOrigin`) | Yes | Yes |
| Subprotocol negotiation | Yes | Yes |
| **Messaging** | | |
| Text and binary messages | Yes | Yes |
| Streaming read (`NextReader`) | Yes | Yes |
| Streaming write (`NextWriter`) | Yes | Yes |
| JSON helpers (`ReadJSON`, `WriteJSON`) | Yes | Yes |
| Pre-built messages (`PreparedMessage`) | Yes | Yes |
| **Control Frames** | | |
| Ping, pong, close frames | Yes | Yes |
| Custom ping/pong/close handlers | Yes | Yes |
| Close codes (RFC 6455 Section 7.4) | Yes | Yes |
| `FormatCloseMessage` | Yes | Yes |
| `IsCloseError`, `IsUnexpectedCloseError` | Yes | Yes |
| **Compression** | | |
| permessage-deflate (RFC 7692) | Yes | Yes |
| **Performance** | | |
| Buffer pools (`BufferPool`) | Yes | Yes |
| Configurable read/write buffer sizes | Yes | Yes |
| **Network** | | |
| HTTP proxy support (CONNECT) | Yes | Yes |
| HTTP/2 upgrade (RFC 8441) | No | Yes |
| **Helpers** | | |
| `Subprotocols(r)` | Yes | Yes |
| `IsWebSocketUpgrade(r)` | Yes | Yes |

---

## handlers / muxhandlers

gorilla/handlers and kasper/muxhandlers both provide HTTP middleware but with different APIs. kasper/muxhandlers uses config structs instead of function parameters.

### Handlers Feature Comparison

| Feature | gorilla/handlers | kasper/muxhandlers |
|---------|:----------------:|:------------------:|
| **Security** | | |
| CORS | Yes | Yes |
| Basic authentication | No | Yes |
| Security headers (X-Frame-Options, HSTS, CSP) | No | Yes |
| IP allow list (CIDR support) | No | Yes |
| Content-Type validation | No | Yes |
| **Request Processing** | | |
| Proxy headers (X-Forwarded-For/Proto/Host) | Yes | Yes |
| RFC 7239 Forwarded header | No | Yes |
| Request size limit | No | Yes |
| Method override (X-HTTP-Method-Override) | No | Yes |
| Request ID generation (UUID v4/v7) | No | Yes |
| **Response Processing** | | |
| Gzip/deflate compression | Yes | Yes |
| Cache-Control header generation | No | Yes |
| Server identification header | No | Yes |
| **Reliability** | | |
| Panic recovery | Yes | Yes |
| Handler timeout | No | Yes |
| **API Lifecycle** | | |
| Sunset/deprecation headers (RFC 8594) | No | Yes |
| **Static Files** | | |
| Static file serving (`fs.FS`) | No | Yes |
| SPA fallback | No | Yes |
| **Debugging** | | |
| pprof profiler endpoints | No | Yes |
| **Logging** | | |
| Apache/combined log format | Yes | No |
| Custom log formatting | Yes | No |

### API Style Difference

gorilla/handlers uses function parameters:

```go
// gorilla/handlers
handler = handlers.CORS(
    handlers.AllowedOrigins([]string{"https://example.com"}),
    handlers.AllowedMethods([]string{"GET", "POST"}),
)(handler)

handler = handlers.CompressHandler(handler)
handler = handlers.RecoveryHandler()(handler)
```

kasper/muxhandlers uses config structs and returns `mux.MiddlewareFunc`:

```go
// kasper/muxhandlers
r.Use(muxhandlers.CORSMiddleware(muxhandlers.CORSConfig{
    AllowedOrigins: []string{"https://example.com"},
    AllowedMethods: []string{http.MethodGet, http.MethodPost},
}))

r.Use(muxhandlers.CompressionMiddleware(muxhandlers.CompressionConfig{}))
r.Use(muxhandlers.RecoveryMiddleware(muxhandlers.RecoveryConfig{}))
```

---

## openapi (kasper only)

Automatic OpenAPI v3.1.0 specification generation from mux routes via reflection and struct tags. No Gorilla equivalent.

| Feature | Details |
|---------|---------|
| Schema generation | JSON Schema Draft 2020-12 via reflection |
| Struct tags | `openapi:"description=...,format=email,minLength=1"` |
| Named routes | `spec.Op("routeName")` for fluent operation building |
| Direct attachment | `spec.Route(route)` for unnamed routes |
| Groups | Shared metadata (tags, security, responses, parameters) |
| Security schemes | Basic, bearer, OAuth2, API key |
| Content types | Multiple request/response content types |
| Webhooks | API-initiated callbacks |
| Docs UI | Swagger UI, RapiDoc, Redoc |
| Export | `doc.JSON()`, `doc.YAML()` |
| Parsing | `DocumentFromJSON`, `DocumentFromYAML` |
| Merging | `MergeDocuments` combines multiple specs |
| Standalone | `SchemaGenerator` produces specs without a router |
| Generic types | Proper instantiation handling |
| Custom naming | `Namer` and `Exampler` interfaces |

---

## httpsig (kasper only)

HTTP Message Signatures (RFC 9421) with optional Content-Digest (RFC 9530). No Gorilla equivalent.

| Feature | Details |
|---------|---------|
| Algorithms | Ed25519, ECDSA P-256/P-384, RSA-PSS, RSA v1.5, HMAC-SHA256 |
| Request signing | `SignRequest` adds Signature and Signature-Input headers |
| Request verification | `VerifyRequest` with key resolver, max age, required components |
| Content-Digest | SHA-256/SHA-512 body integrity (RFC 9530) |
| Client transport | `http.RoundTripper` for automatic request signing |
| Server middleware | `mux.MiddlewareFunc` for automatic request verification |
| Nonce generation | Replay attack prevention |
| Component coverage | Method, authority, path, query, scheme, target URI, headers |
