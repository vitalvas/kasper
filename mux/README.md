# mux

HTTP request multiplexer with URL pattern matching.

## Features

- URL path variables with optional regex constraints (`{name}`, `{id:[0-9]+}`)
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

http.ListenAndServe(":8080", r)
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
