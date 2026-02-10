// Package mux implements a request router and dispatcher for matching
// incoming HTTP requests to their respective handler functions.
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
