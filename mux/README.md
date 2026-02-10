# mux

HTTP request multiplexer with URL pattern matching.

## Features

- URL path variables with optional regex constraints (`{name}`, `{id:[0-9]+}`) or named macros (`{id:uuid}`)
- Host, method, header, query, and scheme matchers
- Subrouters with path prefix grouping
- Middleware support
- Named routes with URL building
- Custom error handlers (404, 405)
- Strict slash and path cleaning options
- Walk function for route inspection
- CORS method middleware

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

## Subrouters

```go
api := r.PathPrefix("/api/v1").Subrouter()
api.HandleFunc("/users", listUsers).Methods(http.MethodGet)
api.HandleFunc("/users", createUser).Methods(http.MethodPost)
api.HandleFunc("/users/{id}", getUser).Methods(http.MethodGet)
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

r.Use(mux.MiddlewareFunc(loggingMiddleware))
```

Subrouter middleware is applied after parent router middleware:

```go
r.Use(mux.MiddlewareFunc(parentMiddleware))
sub := r.PathPrefix("/api").Subrouter()
sub.Use(mux.MiddlewareFunc(subMiddleware))
// Order: parentMiddleware -> subMiddleware -> handler
```

## CORS

```go
r.Use(mux.CORSMethodMiddleware(r))
```

Sets `Access-Control-Allow-Methods` header based on registered route methods.

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
methods, _ := route.GetMethods()          // e.g. ["GET", "POST"]
queries, _ := route.GetQueriesTemplates() // e.g. ["q={query}"]
qre, _ := route.GetQueriesRegexp()        // compiled query regexp strings
vars, _ := route.GetVarNames()            // e.g. ["category", "id"]
name := route.GetName()                   // e.g. "article"
err := route.GetError()                   // any build error on the route
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

## Build-Only Routes

Routes can be marked as build-only, used only for URL building and not for request matching:

```go
r.HandleFunc("/old/{id}", handler).Name("old").BuildOnly()
url, _ := r.Get("old").URL("id", "42")
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
