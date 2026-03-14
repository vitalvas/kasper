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
//   - Inline subrouters (Route and Group) for closure-based route definitions
//   - Inline middleware (With) for declaring middleware at route-registration time
//   - Middleware support
//   - Reverse URL building
//   - Walking registered routes
//
// # Router
//
// Create a new router and register handlers:
//
//	r := mux.NewRouter()
//	r.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler)
//	r.HandleFunc("/products/{key}", ProductHandler)
//	http.Handle("/", r)
//
// # Path Variables
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
// # Pattern Macros
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
//	domain   - domain name per RFC 1123 (e.g. example.com, sub.example.co.uk)
//
// If the name after the colon does not match a known macro, it is
// treated as a raw regular expression for full backward compatibility.
//
// # Matchers
//
// Routes support multiple matchers that can be combined:
//
//	// Method matching
//	r.HandleFunc("/users", handler).Methods(http.MethodGet, http.MethodPost)
//
//	// Host matching
//	r.Host("{subdomain}.example.com").Path("/api").HandlerFunc(handler)
//
//	// Header matching
//	r.HandleFunc("/api", handler).Headers("Content-Type", "application/json")
//
//	// Header regex matching
//	r.HandleFunc("/api", handler).HeadersRegexp("Content-Type", "application/.*")
//
//	// Query parameter matching
//	r.HandleFunc("/search", handler).Queries("q", "{query}")
//
//	// Scheme matching
//	r.HandleFunc("/secure", handler).Schemes("https")
//
//	// Custom matcher function
//	r.HandleFunc("/custom", handler).MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
//	    return r.Header.Get("X-Custom") != ""
//	})
//
// # Subrouters
//
// Subrouters can be used to group routes under a common path prefix,
// host constraint, or other matchers:
//
//	s := r.PathPrefix("/api").Subrouter()
//	s.HandleFunc("/users", UsersHandler)
//
// Subrouters support their own NotFoundHandler. When a subrouter's prefix
// matches but no sub-route matches, the subrouter's NotFoundHandler is used
// instead of propagating the 404 to the parent router:
//
//	s := r.PathPrefix("/api").Subrouter()
//	s.NotFoundHandler = http.HandlerFunc(apiNotFoundHandler)
//	s.HandleFunc("/users", UsersHandler)
//
// # Inline Subrouters
//
// Route and Group provide a closure-based API for defining sub-routes
// inline, without saving intermediate variables.
//
// Route creates a subrouter with a path prefix and calls a function with it.
// It is equivalent to PathPrefix(path).Subrouter():
//
//	r.Route("/api/v1", func(api *mux.Router) {
//	    api.HandleFunc("/users", listUsers).Methods(http.MethodGet)
//	    api.HandleFunc("/users/{id}", getUser).Methods(http.MethodGet)
//	})
//
// Middleware can be scoped to a Route subrouter so it only applies
// to routes within the prefix:
//
//	r.Route("/admin", func(admin *mux.Router) {
//	    admin.Use(authMiddleware)
//	    admin.Use(loggingMiddleware)
//	    admin.HandleFunc("/users", adminUsersHandler).Methods(http.MethodGet)
//	    admin.HandleFunc("/settings", adminSettingsHandler).Methods(http.MethodGet)
//	})
//
// Route calls can be nested:
//
//	r.Route("/api", func(api *mux.Router) {
//	    api.Route("/v1", func(v1 *mux.Router) {
//	        v1.HandleFunc("/users", listUsers)
//	    })
//	})
//
// Group creates a subrouter with no path prefix, purely for grouping
// routes with shared middleware. It is equivalent to NewRoute().Subrouter():
//
//	r.Group(func(authed *mux.Router) {
//	    authed.Use(authMiddleware)
//	    authed.HandleFunc("/dashboard", dashboardHandler)
//	    authed.HandleFunc("/settings", settingsHandler)
//	})
//
// Both methods return the parent router for chaining:
//
//	r.Route("/api", func(api *mux.Router) {
//	    api.HandleFunc("/users", listUsers)
//	}).Route("/admin", func(admin *mux.Router) {
//	    admin.HandleFunc("/stats", statsHandler)
//	})
//
// Use Route inside Group (not Group inside Route) when combining
// middleware grouping with path prefixes:
//
//	r.Group(func(authed *mux.Router) {
//	    authed.Use(authMiddleware)
//	    authed.Route("/api", func(api *mux.Router) {
//	        api.HandleFunc("/secrets", secretsHandler)
//	    })
//	})
//
// # Inline Middleware
//
// With creates a lightweight carrier that applies middleware to individual
// routes without affecting other routes on the same router. This is useful
// when you want to attach middleware inline at route-registration time:
//
//	r.With(authMiddleware).HandleFunc("/secret", secretHandler)
//	r.HandleFunc("/public", publicHandler) // no auth middleware
//
// Multiple middleware can be passed to With; they execute in order:
//
//	r.With(loggingMiddleware, authMiddleware).HandleFunc("/admin", adminHandler)
//
// The returned MiddlewareRoute supports the same route-registration methods
// as Router (Handle, HandleFunc, Path, PathPrefix, Host, Methods, Headers,
// HeadersRegexp, Queries, Schemes, MatcherFunc, Name, NewRoute). Routes
// created through it are registered on the parent router but carry the
// additional middleware via Route.Use.
//
// With calls can be chained to incrementally layer middleware:
//
//	r.With(loggingMiddleware).With(authMiddleware).HandleFunc("/admin", handler)
//	// request flow: logging -> auth -> handler
//
// Each chained With creates a new carrier; the parent carrier is not
// modified, so different branches can share a common base:
//
//	logged := r.With(loggingMiddleware)
//	logged.HandleFunc("/public", publicHandler)                 // logging only
//	logged.With(authMiddleware).HandleFunc("/private", handler) // logging + auth
//
// With works with Route and Group to apply middleware to all routes in a
// subrouter:
//
//	r.With(authMiddleware).Route("/admin", func(admin *mux.Router) {
//	    admin.HandleFunc("/users", usersHandler)
//	    admin.HandleFunc("/settings", settingsHandler)
//	})
//
//	r.With(authMiddleware).Group(func(authed *mux.Router) {
//	    authed.HandleFunc("/dashboard", dashboardHandler)
//	    authed.HandleFunc("/profile", profileHandler)
//	})
//
// With can also be used inside Route and Group callbacks to apply middleware
// to a subset of the subrouter's routes:
//
//	r.Route("/api", func(api *mux.Router) {
//	    api.HandleFunc("/health", healthHandler)
//	    api.With(authMiddleware).HandleFunc("/secret", secretHandler)
//	})
//
// With middleware composes with router-level middleware (Use) and
// route-level middleware (Route.Use). Router middleware runs first
// (outermost), then With middleware, then route-level Use middleware
// (innermost before handler):
//
//	r.Use(loggingMiddleware)
//	r.With(authMiddleware).HandleFunc("/admin", adminHandler).Use(auditMiddleware)
//	// request flow: logging -> auth -> audit -> adminHandler
//
// # Error Handling
//
// The Router provides two handler fields for error responses:
//
// NotFoundHandler is called when no route matches a request. If nil,
// http.NotFoundHandler() is used. Corresponds to 404 Not Found per
// RFC 9110 Section 15.5.5.
//
// MethodNotAllowedHandler is called when a route matches the path but not
// the method. If nil, a default 405 handler is used. The Allow header is
// always set before this handler is invoked, per RFC 9110 Section 15.5.6.
//
//	r.NotFoundHandler = http.HandlerFunc(custom404Handler)
//	r.MethodNotAllowedHandler = http.HandlerFunc(custom405Handler)
//
// # Route Matching
//
// Use Router.Match to test whether a request matches any registered route
// without dispatching it:
//
//	var match mux.RouteMatch
//	if r.Match(req, &match) {
//	    // match.Route, match.Handler, match.Vars are populated
//	}
//
// The RouteMatch.MatchErr field indicates the type of match failure:
// ErrMethodMismatch for 405 errors and ErrNotFound for 404 errors.
//
// # Context Functions
//
// Vars returns all route variables for the current request as a map:
//
//	vars := mux.Vars(r)
//
// VarGet returns a single route variable by name and a boolean indicating
// whether it exists:
//
//	id, ok := mux.VarGet(r, "id")
//
// CurrentRoute returns the matched route for the current request. This only
// works when called inside the handler of the matched route:
//
//	route := mux.CurrentRoute(r)
//	tpl, _ := route.GetPathTemplate()
//
// SetURLVars sets the URL variables for the given request, intended for
// testing route handlers:
//
//	req = mux.SetURLVars(req, map[string]string{"id": "42"})
//
// # Middleware
//
// Middleware can be added to a router or subrouter to wrap matched handlers:
//
//	r.Use(loggingMiddleware)
//
// Subrouter middleware is applied after parent router middleware.
//
// # URL Building
//
// Named routes support reverse URL building:
//
//	r.HandleFunc("/articles/{category}/{id:[0-9]+}", handler).Name("article")
//	url, err := r.Get("article").URL("category", "tech", "id", "42")
//
// Routes also provide URLHost and URLPath for building individual URL
// components:
//
//	hostURL, _ := route.URLHost("subdomain", "api")
//	pathURL, _ := route.URLPath("category", "tech", "id", "42")
//
// # Route Inspection
//
// Routes expose methods to inspect their configuration:
//
//	tpl, _ := route.GetPathTemplate()         // e.g. "/articles/{category}/{id}"
//	re, _ := route.GetPathRegexp()            // compiled path regexp string
//	host, _ := route.GetHostTemplate()        // e.g. "{subdomain}.example.com"
//	hostRe, _ := route.GetHostRegexp()        // compiled host regexp string
//	methods, _ := route.GetMethods()          // e.g. ["GET", "POST"]
//	schemes, _ := route.GetSchemes()          // e.g. ["https"]
//	headers, _ := route.GetHeaders()          // e.g. {"Content-Type": "application/json"}
//	hre, _ := route.GetHeadersRegexp()        // compiled header regexp map
//	queries, _ := route.GetQueriesTemplates() // e.g. ["q={query}"]
//	qre, _ := route.GetQueriesRegexp()        // compiled query regexp strings
//	vars, _ := route.GetVarNames()            // e.g. ["category", "id"]
//	buildOnly := route.IsBuildOnly()              // true if build-only
//
// Router configuration can also be inspected:
//
//	strictSlash := router.GetStrictSlash()       // trailing slash redirect
//	skipClean := router.GetSkipClean()           // path cleaning disabled
//	encodedPath := router.GetUseEncodedPath()    // percent-encoded matching
//
// # Strict Slash
//
// StrictSlash defines the trailing slash behavior for new routes. When true,
// if the route path is "/path/", accessing "/path" will redirect to "/path/"
// and vice versa. Uses 308 Permanent Redirect (RFC 7538) to preserve the
// original request method:
//
//	r.StrictSlash(true)
//
// # Path Cleaning
//
// By default, the router cleans request paths by removing dot segments per
// RFC 3986 Section 5.2.4. SkipClean disables this behavior:
//
//	r.SkipClean(true)
//
// UseEncodedPath tells the router to match the percent-encoded original path
// (RFC 3986 Section 2.1) instead of the decoded path:
//
//	r.UseEncodedPath()
//
// # Request Binding
//
// BindJSON and BindXML decode a request body into a Go value. BindJSON
// rejects unknown fields by default; pass true to allow them. Both
// functions reject trailing data after the first value.
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    var req CreateUserRequest
//	    if err := mux.BindJSON(r, &req); err != nil {
//	        http.Error(w, err.Error(), http.StatusBadRequest)
//	        return
//	    }
//	    // use req
//	}
//
// To allow unknown fields:
//
//	err := mux.BindJSON(r, &req, true)
//
// # Response Helpers
//
// ResponseJSON and ResponseXML encode a value and write it to the response
// with the appropriate Content-Type header. If encoding fails, an HTTP 500
// Internal Server Error is written instead.
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    data := map[string]string{"message": "hello"}
//	    mux.ResponseJSON(w, http.StatusOK, data)
//	}
//
//	func xmlHandler(w http.ResponseWriter, r *http.Request) {
//	    data := MyStruct{Name: "example"}
//	    mux.ResponseXML(w, http.StatusOK, data)
//	}
//
// # Typed JSON Handlers
//
// HandleJSON combines [BindJSON] and [ResponseJSON] into a single generic
// handler that decodes the request body, calls a typed function, and encodes
// the result as JSON:
//
//	type CreateReq struct {
//	    Name string `json:"name"`
//	}
//
//	type CreateResp struct {
//	    ID   string `json:"id"`
//	    Name string `json:"name"`
//	}
//
//	h := mux.HandleJSON(
//	    func(w http.ResponseWriter, r *http.Request, in CreateReq) (CreateResp, error) {
//	        id := uuid.New().String()
//	        return CreateResp{ID: id, Name: in.Name}, nil
//	    },
//	    func(w http.ResponseWriter, r *http.Request, err error) {
//	        http.Error(w, err.Error(), http.StatusBadRequest)
//	    },
//	)
//	r.Handle("/users", h).Methods(http.MethodPost)
//
// The first argument is the handler function that receives the decoded
// request body and returns a response value. The second argument is an
// error callback invoked when JSON decoding fails or the handler returns
// an error. On success the response is written as JSON with status 200.
//
// HandleJSONResponse is the same but without request body decoding,
// suitable for GET or DELETE endpoints that only return JSON:
//
//	h := mux.HandleJSONResponse(
//	    func(w http.ResponseWriter, r *http.Request) (UserResp, error) {
//	        id, _ := mux.VarGet(r, "id")
//	        user, err := db.GetUser(id)
//	        if err != nil {
//	            return UserResp{}, err
//	        }
//	        return UserResp{ID: user.ID, Name: user.Name}, nil
//	    },
//	    func(w http.ResponseWriter, r *http.Request, err error) {
//	        http.Error(w, err.Error(), http.StatusNotFound)
//	    },
//	)
//	r.Handle("/users/{id:uuid}", h).Methods(http.MethodGet)
//
// # Build-Only Routes
//
// Routes can be marked as build-only, meaning they are used only for URL
// building and not for request matching:
//
//	r.HandleFunc("/old/{id}", handler).Name("old").BuildOnly()
//	url, _ := r.Get("old").URL("id", "42")
//
// # Route Metadata
//
// Routes support arbitrary key-value metadata for attaching custom
// information that can be read at runtime:
//
//	r.HandleFunc("/admin/users", handler).
//	    Methods(http.MethodGet).
//	    Metadata("role", "admin").
//	    Metadata("rateLimit", 100)
//
// Read metadata inside a handler via CurrentRoute:
//
//	route := mux.CurrentRoute(r)
//	role, err := route.GetMetadataValue("role")
//	limit := route.GetMetadataValueOr("rateLimit", 60)
//
// MetadataFunc adds request-time dynamic metadata. The function receives
// the current request and returns a map that is merged on top of the
// route's static metadata and stored in the request context:
//
//	r.HandleFunc("/users", handler).
//	    Metadata("static", "value").
//	    MetadataFunc(func(r *http.Request) map[any]any {
//	        return map[any]any{"lang": r.Header.Get("Accept-Language")}
//	    })
//
// Use RequestMetadata inside a handler to retrieve the merged metadata
// (static + dynamic). When no MetadataFunc is set, it falls back to the
// route's static metadata without extra context allocation:
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    md := mux.RequestMetadata(r)
//	    lang := md["lang"]
//	}
//
// # Walking Routes
//
// Walk traverses the router and all its subrouters, calling a function for
// each registered route:
//
//	r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
//	    tpl, _ := route.GetPathTemplate()
//	    fmt.Println(tpl)
//	    return nil
//	})
//
// Return SkipRouter from the walk function to skip descending into a
// subrouter.
package mux
