// Package mux implements a request router and dispatcher for matching
// incoming HTTP requests to their respective handler functions.
//
// The package implements routing semantics based on:
//   - RFC 9110 (HTTP Semantics, successor to RFC 7231)
//   - RFC 9112 (HTTP/1.1, successor to RFC 7230)
//   - RFC 3986 (URIs)
//   - RFC 7538 (308 Permanent Redirect)
//
// This package provides a drop-in replacement for gorilla/mux with full
// API compatibility, including:
//   - Path variables with optional regexp constraints
//   - Host-based routing
//   - Header and query matching
//   - Custom matcher functions
//   - Subrouters for route grouping
//   - Middleware support
//   - Reverse URL building
//   - Walking registered routes
//
// Router Example:
//
//	r := mux.NewRouter()
//	r.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler)
//	r.HandleFunc("/products/{key}", ProductHandler)
//	http.Handle("/", r)
//
// Path Variables:
//
// Routes can have variables enclosed in curly braces, optionally followed
// by a colon and a regular expression pattern:
//
//	r.HandleFunc("/articles/{category}/{id:[0-9]+}", handler)
//
// Variables are extracted and stored in the request context, accessible
// via the Vars function:
//
//	vars := mux.Vars(r)
//	category := vars["category"]
//
// Pattern Macros:
//
// Instead of writing full regex patterns, you can use named macros
// in variable definitions with the {name:macro} syntax:
//
//	r.HandleFunc("/users/{id:uuid}", handler)
//	r.HandleFunc("/articles/{page:int}", handler)
//	r.HandleFunc("/posts/{slug:slug}", handler)
//	r.HandleFunc("/events/{d:date}", handler)
//
// Available macros:
//
//	uuid     - RFC 4122 UUID (e.g. 550e8400-e29b-41d4-a716-446655440000)
//	int      - unsigned integer (e.g. 42)
//	float    - decimal number (e.g. 3.14, 42, .5)
//	slug     - URL-safe slug (e.g. my-post-title)
//	alpha    - alphabetic characters (e.g. hello)
//	alphanum - alphanumeric characters (e.g. abc123)
//	date     - ISO 8601 date (e.g. 2024-01-15)
//	hex      - hexadecimal string (e.g. deadBEEF)
//
// If the name after the colon does not match a known macro, it is
// treated as a raw regular expression for full backward compatibility.
//
// Subrouters:
//
// Subrouters can be used to group routes under a common path prefix,
// host constraint, or other matchers:
//
//	s := r.PathPrefix("/api").Subrouter()
//	s.HandleFunc("/users", UsersHandler)
//
// Middleware:
//
// Middleware can be added to a router or subrouter to wrap matched handlers:
//
//	r.Use(loggingMiddleware)
//
// URL Building:
//
// Named routes support reverse URL building:
//
//	r.HandleFunc("/articles/{category}/{id:[0-9]+}", handler).Name("article")
//	url, err := r.Get("article").URL("category", "tech", "id", "42")
package mux
