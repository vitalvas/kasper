# mux

HTTP request multiplexer with URL pattern matching.

## Features

- URL path variables with optional regex constraints (`{name}`, `{id:[0-9]+}`) or named macros (`{id:uuid}`)
- Host, method, header, query, and scheme matchers
- Subrouters with path prefix grouping
- Inline subrouters (`Route` and `Group`) for closure-based route definitions
- Inline middleware (`With`) for declaring middleware at route-registration time
- Middleware support
- Named routes with URL building
- Custom error handlers (404, 405)
- Strict slash and path cleaning options
- Typed JSON handler with generic request/response binding (`HandleJSON`)
- Route metadata for attaching arbitrary key-value data
- Walk function for route inspection

## Installation

```bash
go get github.com/vitalvas/kasper/mux
```

## Router

```go
r := mux.NewRouter()

r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("home"))
})

r.HandleFunc("/users/{id:[0-9]+}", func(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    fmt.Fprintf(w, "user %s", vars["id"])
})

srv := &http.Server{
    Addr:         ":8080",
    Handler:      r,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
}

srv.ListenAndServe()
```

## Server Usage

### Plain HTTP

```go
srv := &http.Server{
    Addr:         ":8080",
    Handler:      r,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
}

srv.ListenAndServe()
```

### TLS

```go
srv := &http.Server{
    Addr:         ":8443",
    Handler:      r,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
}

srv.ListenAndServeTLS("cert.pem", "key.pem")
```

### Unix Socket

```go
listener, err := net.Listen("unix", "/var/run/app.sock")
if err != nil {
    log.Fatal(err)
}
defer listener.Close()

srv := &http.Server{
    Handler:      r,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
}

srv.Serve(listener)
```

## Path Variables

Variables are defined with `{name}` or `{name:pattern}` syntax:

```go
r.HandleFunc("/articles/{category}/{id:[0-9]+}", handler)

// In handler:
vars := mux.Vars(r)
category := vars["category"]
id := vars["id"]
```

Path templates must start with a slash (`/`). Templates without a leading slash will return an error:

```go
r.HandleFunc("/users/{id}", handler) // OK
r.HandleFunc("users/{id}", handler)  // Error: path must start with a slash
```

Variable patterns must not contain capturing groups `(...)`. Capturing groups create extra submatches that misalign variable extraction. Use non-capturing groups `(?:...)` instead:

```go
r.HandleFunc("/{id:[0-9]+}", handler)        // OK: no groups
r.HandleFunc("/{id:(?:ab|cd)}", handler)     // OK: non-capturing group
r.HandleFunc("/{id:([0-9]+)}", handler)      // Error: capturing group
```

## Pattern Macros

Instead of writing full regex patterns, use named macros for common types:

```go
r.HandleFunc("/users/{id:uuid}", handler)
r.HandleFunc("/articles/{page:int}", handler)
r.HandleFunc("/posts/{slug:slug}", handler)
r.HandleFunc("/values/{val:float}", handler)
r.HandleFunc("/events/{d:date}", handler)
r.HandleFunc("/colors/{h:hex}", handler)
r.HandleFunc("/names/{name:alpha}", handler)
r.HandleFunc("/tokens/{token:alphanum}", handler)
r.HandleFunc("/sites/{d:domain}", handler)
```

| Macro | Description | Example match |
|-------|-------------|---------------|
| `uuid` | RFC 4122 UUID | `550e8400-e29b-41d4-a716-446655440000` |
| `int` | Unsigned integer | `42` |
| `float` | Decimal number | `3.14`, `42`, `.5` |
| `slug` | URL-safe slug | `my-post-title` |
| `alpha` | Alphabetic characters | `hello` |
| `alphanum` | Alphanumeric characters | `abc123` |
| `date` | ISO 8601 date | `2024-01-15` |
| `hex` | Hexadecimal string | `deadBEEF` |
| `domain` | Domain name (RFC 1123) | `example.com`, `sub.example.co.uk` |

If the name after the colon does not match a known macro, it is treated as a raw regular expression:

```go
// Raw regex still works
r.HandleFunc("/items/{id:[0-9]{4}}", handler)
```

## Matchers

```go
// Method
r.HandleFunc("/users", handler).Methods(http.MethodGet, http.MethodPost)

// Host
r.Host("{subdomain}.example.com").Path("/api").HandlerFunc(handler)

// Headers
r.HandleFunc("/api", handler).Headers("Content-Type", "application/json")

// Header regex
r.HandleFunc("/api", handler).HeadersRegexp("Content-Type", "application/.*")

// Query parameters
r.HandleFunc("/search", handler).Queries("q", "{query}", "page", "{page:[0-9]+}")

// Scheme
r.HandleFunc("/secure", handler).Schemes("https")

// Custom matcher
r.HandleFunc("/custom", handler).MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
    return r.Header.Get("X-Custom") != ""
})
```

### Host Matching and Ports

Host templates without a port pattern automatically strip the port from the request before matching. This means `{sub}.example.com` will match requests to `api.example.com:8080`:

```go
r.Host("{sub}.example.com").Path("/api").HandlerFunc(handler)
// Matches: api.example.com, api.example.com:8080, api.example.com:443
```

To capture the port as a variable, include it in the host template:

```go
r.Host("{host:[^:]+}:{port}").Path("/api").HandlerFunc(handler)

// In handler:
vars := mux.Vars(r)
host := vars["host"] // "example.com"
port := vars["port"] // "8080"
```

## Subrouters

```go
api := r.PathPrefix("/api/v1").Subrouter()
api.HandleFunc("/users", listUsers).Methods(http.MethodGet)
api.HandleFunc("/users", createUser).Methods(http.MethodPost)
api.HandleFunc("/users/{id}", getUser).Methods(http.MethodGet)
```

## Inline Subrouters

`Route` and `Group` provide a closure-based API for defining sub-routes inline, without saving intermediate variables.

### Route

`Route` creates a subrouter with a path prefix and calls the function with it. It is equivalent to `PathPrefix(path).Subrouter()`:

```go
r.Route("/api/v1", func(api *mux.Router) {
    api.HandleFunc("/users", listUsers).Methods(http.MethodGet)
    api.HandleFunc("/users", createUser).Methods(http.MethodPost)
    api.HandleFunc("/users/{id}", getUser).Methods(http.MethodGet)
})
```

Middleware can be scoped to a `Route` subrouter so it only applies to routes within the prefix:

```go
r.Route("/admin", func(admin *mux.Router) {
    admin.Use(authMiddleware)
    admin.Use(loggingMiddleware)
    admin.HandleFunc("/users", adminUsersHandler).Methods(http.MethodGet)
    admin.HandleFunc("/settings", adminSettingsHandler).Methods(http.MethodGet)
})
```

Path variables in the prefix are available in handlers:

```go
r.Route("/users/{id}", func(sub *mux.Router) {
    sub.HandleFunc("/profile", func(w http.ResponseWriter, r *http.Request) {
        id := mux.Vars(r)["id"]
        fmt.Fprintf(w, "profile for %s", id)
    })
})
```

`Route` calls can be nested:

```go
r.Route("/api", func(api *mux.Router) {
    api.Route("/v1", func(v1 *mux.Router) {
        v1.HandleFunc("/users", listUsers)
        v1.HandleFunc("/health", healthHandler)
    })
})
```

### Group

`Group` creates a subrouter with no path prefix, purely for grouping routes with shared middleware. It is equivalent to `NewRoute().Subrouter()`:

```go
r.HandleFunc("/public", publicHandler)

r.Group(func(authed *mux.Router) {
    authed.Use(authMiddleware)
    authed.HandleFunc("/dashboard", dashboardHandler)
    authed.HandleFunc("/settings", settingsHandler)
})
```

`Route` can be used inside `Group` to apply shared middleware to prefixed sub-routes:

```go
r.Group(func(authed *mux.Router) {
    authed.Use(authMiddleware)
    authed.Route("/api", func(api *mux.Router) {
        api.HandleFunc("/secrets", secretsHandler)
    })
})
```

`Group` does not add a path prefix. When nesting `Group` inside `Route`, child routes do not inherit the parent `Route` path prefix. Use `Route` inside `Group` (not `Group` inside `Route`) when combining middleware grouping with path prefixes.

### Chaining

Both methods return the parent router, allowing chained calls:

```go
r.Route("/api", func(api *mux.Router) {
    api.HandleFunc("/users", listUsers)
}).Route("/admin", func(admin *mux.Router) {
    admin.HandleFunc("/stats", statsHandler)
})
```

## Error Handling

### NotFoundHandler

Set a custom handler for 404 Not Found responses:

```go
r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusNotFound)
    fmt.Fprint(w, "page not found")
})
```

### MethodNotAllowedHandler

Set a custom handler for 405 Method Not Allowed responses. The `Allow` header is automatically set before the handler is invoked:

```go
r.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusMethodNotAllowed)
    fmt.Fprintf(w, "method %s not allowed", r.Method)
})
```

### Subrouter NotFoundHandler

Subrouters can have their own `NotFoundHandler`. When the subrouter's prefix matches but no sub-route matches, the subrouter's handler is used instead of the root router's:

```go
r := mux.NewRouter()
r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusNotFound)
    fmt.Fprint(w, "root 404")
})

api := r.PathPrefix("/api").Subrouter()
api.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusNotFound)
    json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
})
api.HandleFunc("/users", listUsers).Methods(http.MethodGet)
```

A request to `/api/unknown` uses the API subrouter's handler (JSON response), while `/other` uses the root handler (plain text). Method-not-allowed (405) errors still propagate correctly regardless of the subrouter's NotFoundHandler.

## Route Matching

Use `Router.Match` to test whether a request matches any registered route without dispatching it:

```go
var match mux.RouteMatch
if r.Match(req, &match) {
    // match.Route, match.Handler, match.Vars are populated
}
```

The `RouteMatch.MatchErr` field indicates the type of match failure:

| Error | Description |
|-------|-------------|
| `ErrMethodMismatch` | Path matched but method did not (405) |
| `ErrNotFound` | No route matched (404) |

## Context Functions

### Vars

Returns all route variables for the current request as a map:

```go
func handler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id := vars["id"]
}
```

### VarGet

Returns a single route variable by name and a boolean indicating whether it exists:

```go
func handler(w http.ResponseWriter, r *http.Request) {
    id, ok := mux.VarGet(r, "id")
    if !ok {
        http.Error(w, "missing id", http.StatusBadRequest)
        return
    }
    fmt.Fprintf(w, "id: %s", id)
}
```

### CurrentRoute

Returns the matched route for the current request. Only works inside the handler of the matched route:

```go
func handler(w http.ResponseWriter, r *http.Request) {
    route := mux.CurrentRoute(r)
    tpl, _ := route.GetPathTemplate()
    methods, _ := route.GetMethods()
}
```

### SetURLVars

Sets URL variables on a request, intended for testing route handlers:

```go
req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
req = mux.SetURLVars(req, map[string]string{"id": "42"})
handler(w, req)
```

## Middleware

```go
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Println(r.RequestURI)
        next.ServeHTTP(w, r)
    })
}

r.Use(loggingMiddleware)
```

Subrouter middleware is applied after parent router middleware:

```go
r.Use(parentMiddleware)
sub := r.PathPrefix("/api").Subrouter()
sub.Use(subMiddleware)
// Order: parentMiddleware -> subMiddleware -> handler
```

### Route-Level Middleware

Middleware can be applied to individual routes using `route.Use()`. Route middleware wraps the handler inside router middleware (innermost layer):

```go
route := r.HandleFunc("/admin", handler)
route.Use(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // route-specific logic
        next.ServeHTTP(w, r)
    })
})
// Order: router middleware -> route middleware -> handler
```

`GetHandlerWithMiddlewares` returns the handler wrapped with route-level middleware only (not router middleware). Returns `nil` if no handler is set:

```go
h := route.GetHandlerWithMiddlewares()
```

### Inline Middleware (With)

`With` creates a lightweight carrier that applies middleware to individual routes without affecting other routes on the same router:

```go
r.With(authMiddleware).HandleFunc("/secret", secretHandler)
r.HandleFunc("/public", publicHandler) // no auth middleware
```

Multiple middleware can be passed; they execute in order:

```go
r.With(loggingMiddleware, authMiddleware).HandleFunc("/admin", adminHandler)
```

`With` calls can be chained to incrementally layer middleware:

```go
r.With(loggingMiddleware).With(authMiddleware).HandleFunc("/admin", handler)
// Order: logging -> auth -> handler
```

Each chained `With` creates a new carrier. The parent carrier is not modified, so different branches can share a common base:

```go
logged := r.With(loggingMiddleware)
logged.HandleFunc("/public", publicHandler)                 // logging only
logged.With(authMiddleware).HandleFunc("/private", handler) // logging + auth
```

### With combined with Route and Group

`With` works with `Route` and `Group` to apply middleware to all routes in a subrouter:

```go
r.With(authMiddleware).Route("/admin", func(admin *mux.Router) {
    admin.HandleFunc("/users", usersHandler)
    admin.HandleFunc("/settings", settingsHandler)
})

r.With(authMiddleware).Group(func(authed *mux.Router) {
    authed.HandleFunc("/dashboard", dashboardHandler)
    authed.HandleFunc("/profile", profileHandler)
})
```

`With` can also be used inside `Route` and `Group` callbacks to apply middleware to a subset of the subrouter's routes:

```go
r.Route("/api", func(api *mux.Router) {
    api.HandleFunc("/health", healthHandler)
    api.With(authMiddleware).HandleFunc("/secret", secretHandler)
})
```

### With combined with Use

`With` middleware composes with router-level middleware (`Use`) and route-level middleware (`Route.Use`). Router middleware runs first (outermost), then `With` middleware, then route-level `Use` middleware (innermost before handler):

```go
r.Use(loggingMiddleware)
r.With(authMiddleware).HandleFunc("/admin", adminHandler).Use(auditMiddleware)
// Order: logging -> auth -> audit -> adminHandler
```

All layers can be combined with `Route` and `Group`:

```go
r.Use(loggingMiddleware)
r.With(authMiddleware).Route("/admin", func(admin *mux.Router) {
    admin.Use(adminAuditMiddleware)
    admin.HandleFunc("/users", usersHandler)
})
// Order: logging -> auth -> adminAudit -> usersHandler
```

## Named Routes and URL Building

```go
r.HandleFunc("/users/{id}", handler).Name("user")

route := r.Get("user")
url, err := route.URL("id", "42")
// url.Path == "/users/42"
```

Routes with host matchers can build full URLs:

```go
r.Host("{subdomain}.example.com").Path("/api/{resource}").Name("api")
url, _ := r.Get("api").URL("subdomain", "docs", "resource", "users")
// url == "http://docs.example.com/api/users"
```

Build host or path components individually:

```go
hostURL, _ := route.URLHost("subdomain", "docs")
pathURL, _ := route.URLPath("resource", "users")
```

## Route Inspection

Routes expose methods to inspect their configuration:

```go
tpl, _ := route.GetPathTemplate()         // e.g. "/articles/{category}/{id}"
re, _ := route.GetPathRegexp()            // compiled path regexp string
host, _ := route.GetHostTemplate()        // e.g. "{subdomain}.example.com"
hostRe, _ := route.GetHostRegexp()        // compiled host regexp string
methods, _ := route.GetMethods()          // e.g. ["GET", "POST"]
schemes, _ := route.GetSchemes()          // e.g. ["https"]
headers, _ := route.GetHeaders()          // e.g. {"Content-Type": "application/json"}
hre, _ := route.GetHeadersRegexp()        // compiled header regexp map
queries, _ := route.GetQueriesTemplates() // e.g. ["q={query}"]
qre, _ := route.GetQueriesRegexp()        // compiled query regexp strings
vars, _ := route.GetVarNames()            // e.g. ["category", "id"]
name := route.GetName()                   // e.g. "article"
buildOnly := route.IsBuildOnly()          // true if build-only
err := route.GetError()                   // any build error on the route
```

Router configuration can also be inspected:

```go
strictSlash := router.GetStrictSlash()       // trailing slash redirect
skipClean := router.GetSkipClean()           // path cleaning disabled
encodedPath := router.GetUseEncodedPath()    // percent-encoded matching
```

## Strict Slash

```go
r.StrictSlash(true)
```

When enabled, `/users/` and `/users` are treated as the same route with a 308 redirect that preserves the request method.

## Path Cleaning

By default, the router cleans request paths by removing dot segments per RFC 3986. Disable this with:

```go
r.SkipClean(true)
```

To match the percent-encoded original path instead of the decoded path:

```go
r.UseEncodedPath()
```

## Request Binding

`BindJSON` and `BindXML` decode a request body into a Go value. `BindJSON` rejects unknown fields by default; pass `true` to allow them. Both functions reject trailing data after the first value.

```go
r.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
    var req CreateUserRequest
    if err := mux.BindJSON(r, &req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    // use req
}).Methods(http.MethodPost)
```

To allow unknown fields:

```go
err := mux.BindJSON(r, &req, true)
```

```go
r.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
    var req CreateUserRequest
    if err := mux.BindXML(r, &req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    // use req
}).Methods(http.MethodPost)
```

| Function | Rejects unknown fields | Rejects trailing data |
|----------|------------------------|-----------------------|
| `BindJSON` | Yes (default), pass `true` to allow | Yes |
| `BindXML` | No (not supported by `encoding/xml`) | Yes |

## Response Helpers

`ResponseJSON` and `ResponseXML` encode a value and write it to the response with the appropriate `Content-Type` header. If encoding fails, an HTTP 500 Internal Server Error is written instead.

```go
r.HandleFunc("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
    user := User{Name: "Alice", Age: 30}
    mux.ResponseJSON(w, http.StatusOK, user)
})
```

```go
r.HandleFunc("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
    user := User{XMLName: xml.Name{Local: "user"}, Name: "Alice", Age: 30}
    mux.ResponseXML(w, http.StatusOK, user)
})
```

| Function | Content-Type |
|----------|--------------|
| `ResponseJSON` | `application/json` |
| `ResponseXML` | `application/xml` |

## Typed JSON Handlers

`HandleJSON` combines `BindJSON` and `ResponseJSON` into a single generic handler that decodes the request body, calls a typed function, and encodes the result as JSON with status 200. The caller provides an error callback to control how errors are mapped to HTTP responses.

```go
type CreateReq struct {
    Name string `json:"name"`
}

type CreateResp struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

h := mux.HandleJSON(
    func(w http.ResponseWriter, r *http.Request, in CreateReq) (CreateResp, error) {
        id := uuid.New().String()
        return CreateResp{ID: id, Name: in.Name}, nil
    },
    func(w http.ResponseWriter, r *http.Request, err error) {
        http.Error(w, err.Error(), http.StatusBadRequest)
    },
)
r.Handle("/users", h).Methods(http.MethodPost)
```

The handler function receives `http.ResponseWriter` and `*http.Request` so it can access route variables, headers, and set response headers. The error callback is invoked when JSON decoding fails or the handler returns a non-nil error.

`HandleJSONResponse` is the same but without request body decoding, suitable for GET or DELETE endpoints that only return JSON:

```go
h := mux.HandleJSONResponse(
    func(w http.ResponseWriter, r *http.Request) (UserResp, error) {
        id, _ := mux.VarGet(r, "id")
        user, err := db.GetUser(id)
        if err != nil {
            return UserResp{}, err
        }
        return UserResp{ID: user.ID, Name: user.Name}, nil
    },
    func(w http.ResponseWriter, r *http.Request, err error) {
        http.Error(w, err.Error(), http.StatusNotFound)
    },
)
r.Handle("/users/{id:uuid}", h).Methods(http.MethodGet)
```

## Build-Only Routes

Routes can be marked as build-only, used only for URL building and not for request matching:

```go
r.HandleFunc("/old/{id}", handler).Name("old").BuildOnly()
url, _ := r.Get("old").URL("id", "42")
```

## Route Metadata

Routes support arbitrary key-value metadata for attaching custom information (e.g. permissions, rate limits, feature flags) that can be read at runtime:

```go
r.HandleFunc("/admin/users", handler).
    Methods(http.MethodGet).
    Metadata("role", "admin").
    Metadata("rateLimit", 100)
```

Set multiple keys at once with `MetadataMap`:

```go
r.HandleFunc("/admin/users", handler).
    MetadataMap(map[any]any{"role": "admin", "rateLimit": 100})
```

Read metadata inside a handler via `CurrentRoute`:

```go
func handler(w http.ResponseWriter, r *http.Request) {
    route := mux.CurrentRoute(r)
    role, err := route.GetMetadataValue("role")
    if err != nil {
        // key not found: err == mux.ErrMetadataKeyNotFound
    }
}
```

Use `GetMetadataValueOr` for a fallback when the key may not exist:

```go
limit := route.GetMetadataValueOr("rateLimit", 60)
```

### Dynamic Metadata (MetadataFunc)

`MetadataFunc` adds request-time dynamic metadata. The function receives the current `*http.Request` and returns a map that is merged on top of the route's static metadata:

```go
r.HandleFunc("/users", handler).
    Metadata("static", "value").
    MetadataFunc(func(r *http.Request) map[any]any {
        return map[any]any{"lang": r.Header.Get("Accept-Language")}
    })
```

Use `RequestMetadata` inside a handler to retrieve the merged metadata (static + dynamic). When no `MetadataFunc` is set, it falls back to the route's static metadata without extra context allocation:

```go
func handler(w http.ResponseWriter, r *http.Request) {
    md := mux.RequestMetadata(r)
    lang := md["lang"]
    role := md["static"]
}
```

| Method | Description |
|--------|-------------|
| `Metadata(key, value any)` | Set a key-value pair (fluent, chainable) |
| `MetadataMap(m map[any]any)` | Merge a map into metadata (fluent, chainable) |
| `MetadataFunc(fn func(*http.Request) map[any]any)` | Set dynamic metadata function (fluent, chainable) |
| `GetMetadata()` | Return the full metadata map (`nil` if unset) |
| `MetadataContains(key any)` | Check whether a key exists |
| `GetMetadataValue(key any)` | Get value or `ErrMetadataKeyNotFound` |
| `GetMetadataValueOr(key, fallback any)` | Get value with fallback default |
| `RequestMetadata(r *http.Request)` | Get merged metadata from request context (package-level function) |

## Custom Regexp Compiler

By default, route patterns are compiled with `regexp.Compile`. Override `RegexpCompileFunc` to use a different compiler:

```go
mux.RegexpCompileFunc = regexp.CompilePOSIX
```

## Walk

```go
r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
    tpl, _ := route.GetPathTemplate()
    methods, _ := route.GetMethods()
    fmt.Printf("%s %v\n", tpl, methods)
    return nil
})
```

Return `mux.SkipRouter` from the walk function to skip descending into a subrouter.
