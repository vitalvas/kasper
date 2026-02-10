# mux

HTTP request multiplexer with URL pattern matching.

## Features

- URL path variables with optional regex constraints (`{name}`, `{id:[0-9]+}`) or named macros (`{id:uuid}`)
- Host, method, header, query, and scheme matchers
- Subrouters with path prefix grouping
- Middleware support
- Named routes with URL building
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

## Strict Slash

```go
r.StrictSlash(true)
```

When enabled, `/users/` and `/users` are treated as the same route with a 301 redirect.

## Walk

```go
r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
    tpl, _ := route.GetPathTemplate()
    methods, _ := route.GetMethods()
    fmt.Printf("%s %v\n", tpl, methods)
    return nil
})
```
