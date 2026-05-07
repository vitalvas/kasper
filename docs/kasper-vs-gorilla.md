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
| `gorilla/sessions` | -- | No Kasper equivalent |
| `gorilla/securecookie` | `kasper/securecookie` | New implementation, different API |
| `gorilla/csrf` | `kasper/csrf` | New implementation, different API |
| `gorilla/schema` | `kasper/mux` | `BindQuery`, `BindForm`, `EncodeQuery`, `EncodeForm` with dot notation |
| `gorilla/feeds` | -- | No Kasper equivalent |
| `gorilla/rpc` | -- | No Kasper equivalent |
| `gorilla/pat` | `kasper/mux` | Covered by mux pattern syntax |
| `gorilla/reverse` | `kasper/mux` | Covered by mux URL building |
| `gorilla/context` | -- | Deprecated; Go stdlib `context.Context` used instead |
| -- | `kasper/openapi` | No Gorilla equivalent |
| -- | `kasper/httpsig` | No Gorilla equivalent |
| -- | `kasper/blindrsa` | No Gorilla equivalent |

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
| Query parameter binding (`BindQuery`) | No | Yes |
| Form body binding (`BindForm`) | No | Yes |
| Values encoding (`EncodeQuery`, `EncodeForm`) | No | Yes |
| Nested struct binding (dot notation) | No | Yes |
| Slice-of-struct binding (indexed dot) | No | Yes |
| Map field binding | No | Yes |
| Embedded struct flattening | No | Yes |
| Max slice index protection | No | Yes |
| Tag options (`required`, `default`, `omitempty`) | No | Yes |
| JSON response writing (`ResponseJSON`) | No | Yes |
| XML response writing (`ResponseXML`) | No | Yes |
| HTML template responses (`SetTemplates`, `ResponseHTML`) | No | Yes |
| Inline HTML rendering (`ResponseHTMLTemplate`, `ResponseHTMLString`) | No | Yes |
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
| `Scheme(r)` | No | Yes |
| **URL Building** | | |
| Named routes | Yes | Yes |
| `URL`, `URLHost`, `URLPath` | Yes | Yes |
| `Reverse(r, name, pairs...)` from handlers | No | Yes |
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
url, err := mux.Reverse(r, "product-detail", "pk", "123")
scheme := mux.Scheme(r)
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
| Bearer token authentication (RFC 6750) | No | Yes |
| Security headers (X-Frame-Options, HSTS, CSP) | No | Yes |
| IP allow list (CIDR support) | No | Yes |
| Content-Type validation | No | Yes |
| **Request Processing** | | |
| Proxy headers (X-Forwarded-For/Proto/Host) | Yes | Yes |
| RFC 7239 Forwarded header | No | Yes |
| Request size limit | No | Yes |
| Method override (X-HTTP-Method-Override) | No | Yes |
| Request ID generation (UUID v4/v7) | No | Yes |
| Content negotiation (RFC 9110) | No | Yes |
| PATCH content type routing (RFC 7396, RFC 6902) | No | Yes |
| OPTIONS + Accept-Patch (RFC 5789) | No | Yes |
| **Response Processing** | | |
| Gzip/deflate compression | Yes | Yes |
| Cache-Control header generation | No | Yes |
| Server identification header | No | Yes |
| Early Hints (RFC 8297) | No | Yes |
| Problem Details (RFC 9457) | No | Yes |
| **Reliability** | | |
| Panic recovery | Yes | Yes |
| Handler timeout | No | Yes |
| Idempotency (draft-ietf-httpapi-idempotency-key) | No | Yes |
| **API Lifecycle** | | |
| Sunset/deprecation headers (RFC 8594) | No | Yes |
| **Static Files** | | |
| Static file serving (`fs.FS`) | No | Yes |
| SPA fallback | No | Yes |
| Static file ETag (If-None-Match, 304) | No | Yes |
| **Debugging** | | |
| pprof profiler endpoints | No | Yes |
| **Logging** | | |
| Apache/combined log format | Yes | No |
| Custom log formatting | Yes | No |
| **Other** | | |
| Canonical host redirect | Yes | Yes |
| Path-based redirects (exact + wildcard) | No | Yes |

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

---

## csrf

gorilla/csrf and kasper/csrf both protect against Cross-Site Request Forgery but use different validation models and cookie cryptography.

### CSRF Feature Comparison

| Feature | gorilla/csrf | kasper/csrf |
|---------|:------------:|:-----------:|
| **Validation Layers** | | |
| `Origin` header check (RFC 6454) | No | Yes (primary) |
| `Referer` fallback (HTTPS) | Yes | Yes |
| Signed token cookie | Yes | Yes |
| Constant-time comparison | Yes | Yes |
| **Token Cookie** | | |
| Cryptography | HMAC-SHA256 + AES-CTR (gorilla/securecookie) | AES-GCM (kasper/securecookie) |
| Default SameSite | `LaxMode` | `LaxMode` |
| Trusted origins with wildcards | Limited | Yes (`https://*.example.com`) |
| Trusted origin predicate (`TrustedOriginFunc`) | No | Yes |
| Strict scheme matching (CVE-2025-47909) | Bug (host-only) | Yes (scheme + host) |
| `Origin: null` handling | Bug (rejected as parse error) | Yes (treated as missing per RFC 6454) |
| **Token Transport** | | |
| Header submission (`X-CSRF-Token`) | Yes | Yes |
| Form field submission (`csrf_token`) | Yes | Yes (`csrf_token`, no dots) |
| URL-safe base64 token | No (standard b64) | Yes (`base64.RawURLEncoding`) |
| BREACH defense via masking | Yes | Yes |
| **Helpers** | | |
| Raw token (`Token`) | Yes | Yes |
| Hidden form field (`TemplateField`) | Yes | Yes |
| Token rotation (`Rotate`) | No | Yes |
| Manual validation (`Validate`) | No | Yes |
| `Token` available in error handler (SPA bootstrap) | Inconsistent | Yes |
| **Configuration** | | |
| Lazy cookie issuance | No | Yes (`Lazy: true`) |
| Custom error handler | Yes (`ErrorHandler`) | Yes |
| Custom safe methods | No | Yes |
| Reverse-proxy aware (via `muxhandlers.ProxyHeaders`) | Manual | Yes (documented) |

### CSRF Migration

```go
// gorilla/csrf
csrfMiddleware := csrf.Protect(
    key,
    csrf.Secure(true),
    csrf.TrustedOrigins([]string{"example.com"}),
)
r.Use(csrfMiddleware)
token := csrf.Token(r)
field := csrf.TemplateField(r)

// kasper/csrf
r.Use(csrf.Middleware(csrf.Config{
    Key:            key,
    TrustedOrigins: []string{"https://example.com", "https://*.example.com"},
}))
token := csrf.Token(r)
field := csrf.TemplateField(r)
csrf.Rotate(w, r) // not in gorilla/csrf
```

---

## securecookie

gorilla/securecookie and kasper/securecookie both provide authenticated, encrypted cookie values but use different cryptographic designs and APIs.

### Secure Cookie Feature Comparison

| Feature | gorilla/securecookie | kasper/securecookie |
|---------|:--------------------:|:-------------------:|
| **Cryptography** | | |
| Encryption algorithm | AES-CTR (stream cipher) | AES-GCM (AEAD) |
| Authentication | HMAC-SHA256 (separate step) | GCM auth tag (built-in) |
| Key count | 2 (hash key + block key) | 1 |
| Key sizes | Hash: 32-64 B, Block: 16/24/32 B | 16/24/32 B (AES-128/192/256) |
| Auth-only mode (no encryption) | Yes (nil block key) | No (always encrypted) |
| **Timestamps** | | |
| Embedded timestamp | Yes | Yes |
| MaxAge | Yes (default 30 days) | Yes (default 30 days) |
| MinAge | Yes | Yes |
| Future timestamp rejection | No | Yes (5 min skew) |
| **Key Rotation** | | |
| Multi-key encode | `EncodeMulti` | `EncodeMulti` |
| Multi-key decode | `DecodeMulti` | `DecodeMulti` |
| Codec factory | `CodecsFromPairs(hash, block, ...)` | `CodecsFromKeys(key1, key2, ...)` |
| Mixed key sizes during rotation | No | Yes |
| **Compression** | | |
| Automatic compression | No | Yes (deflate) |
| Entropy-adaptive (skip high-entropy) | No | Yes (Shannon entropy check) |
| Pooled compressor (low allocs) | No | Yes (`sync.Pool`) |
| **AAD (Additional Authenticated Data)** | | |
| Cookie name as AAD | Yes (always, via HMAC) | No (opt-in via `AdditionalData`) |
| Custom AAD | No | Yes |
| **Serialization** | | |
| Default serializer | Gob | JSON |
| Custom serializer | Yes (`Serializer` interface) | Yes (`Serializer` interface) |
| NopEncoder (pass-through) | Yes | No |
| **Safety** | | |
| Nil receiver guards | No | Yes |
| Typed-nil codec guards | No | Yes |
| Zero-value struct guards | No | Yes |
| Negative config value handling | Accepted as-is | Clamped to 0 |
| Decompression size limit | N/A | 512 KB |
| **Error Handling** | | |
| Error style | `Error` interface (IsUsage/IsDecode/IsInternal) | Sentinel errors (`errors.Is`) |
| Constructor returns error | No (deferred) | Yes (fail-fast) |
| MultiError on decode | Yes | No (returns last error) |
| **API** | | |
| Builder pattern | Yes | Yes |
| Custom hash function | Yes (`HashFunc`) | N/A (GCM) |
| Custom block cipher | Yes (`BlockFunc`) | N/A (AES only) |
| Key generation | `GenerateRandomKey(n) []byte` | `GenerateKey(n) ([]byte, error)` |
| **Status** | | |
| Maintenance | Archived | Active |

### API Comparison

| API | gorilla/securecookie | kasper/securecookie |
|-----|----------------------|---------------------|
| Constructor | `New(hashKey, blockKey []byte) *SecureCookie` | `New(key []byte) (*SecureCookie, error)` |
| Encode | `Encode(name string, value any) (string, error)` | `Encode(value any) (string, error)` |
| Decode | `Decode(name, value string, dst any) error` | `Decode(value string, dst any) error` |
| Multi-encode | `EncodeMulti(name string, value any, codecs ...Codec) (string, error)` | `EncodeMulti(value any, codecs ...Codec) (string, error)` |
| Multi-decode | `DecodeMulti(name, value string, dst any, codecs ...Codec) error` | `DecodeMulti(value string, dst any, codecs ...Codec) error` |
| Codec factory | `CodecsFromPairs(keyPairs ...[]byte) []Codec` | `CodecsFromKeys(keys ...[]byte) ([]Codec, error)` |
| Key generation | `GenerateRandomKey(length int) []byte` | `GenerateKey(size int) ([]byte, error)` |
| AAD | N/A (cookie name always used) | `AdditionalData(data []byte) *SecureCookie` |
| Serializer | `SetSerializer(sz Serializer) *SecureCookie` | `SetSerializer(sz Serializer) *SecureCookie` |
| MaxAge | `MaxAge(value int) *SecureCookie` | `MaxAge(seconds int) *SecureCookie` |
| MinAge | `MinAge(value int) *SecureCookie` | `MinAge(seconds int) *SecureCookie` |
| MaxLength | `MaxLength(value int) *SecureCookie` | `MaxLength(length int) *SecureCookie` |
| Hash function | `HashFunc(f func() hash.Hash) *SecureCookie` | N/A |
| Block cipher | `BlockFunc(f func([]byte) (cipher.Block, error)) *SecureCookie` | N/A |

### Secure Cookie Migration

```go
// gorilla/securecookie
s := securecookie.New(hashKey, blockKey)
s.MaxAge(3600)
encoded, err := s.Encode("session", value)
err = s.Decode("session", encoded, &dst)
codecs := securecookie.CodecsFromPairs(hashKey, blockKey, oldHash, oldBlock)
key := securecookie.GenerateRandomKey(32)

// kasper/securecookie
s, err := securecookie.New(key)          // single key, returns error
s.MaxAge(3600)
encoded, err := s.Encode(value)          // no name parameter
err = s.Decode(encoded, &dst)            // no name parameter
codecs, err := securecookie.CodecsFromKeys(key, oldKey) // returns error
key, err := securecookie.GenerateKey(32)                // returns error
```

### Architecture Difference

gorilla/securecookie uses two separate cryptographic primitives:
HMAC for authentication and AES-CTR for encryption.
This requires two keys and two passes over the data,
and allows an "auth-only" mode where data is signed but not encrypted.

kasper/securecookie uses AES-GCM, which provides authenticated encryption
in a single pass with a single key. There is no auth-only mode -- data is
always both encrypted and authenticated. This eliminates the risk of
accidentally deploying without encryption (a common gorilla/securecookie
misconfiguration when the block key is nil).
