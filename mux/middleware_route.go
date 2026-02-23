package mux

import "net/http"

// MiddlewareRoute is a lightweight carrier that holds middleware and delegates
// route creation to a parent Router. Each route-registration method creates
// a route on the parent router and applies the stored middleware via
// Route.Use, so the middleware only affects routes created through this
// carrier.
//
// Use Router.With to obtain a MiddlewareRoute:
//
//	r.With(authMiddleware).HandleFunc("/secret", handler)
type MiddlewareRoute struct {
	router      *Router
	middlewares []MiddlewareFunc
}

// newRoute creates a new route on the parent router and applies stored middleware.
func (mr *MiddlewareRoute) newRoute() *Route {
	route := mr.router.NewRoute()
	route.Use(mr.middlewares...)
	return route
}

// NewRoute creates an empty route with the stored middleware applied.
func (mr *MiddlewareRoute) NewRoute() *Route {
	return mr.newRoute()
}

// Handle registers a new route with a matcher for the URL path and handler.
func (mr *MiddlewareRoute) Handle(path string, handler http.Handler) *Route {
	return mr.newRoute().Path(path).Handler(handler)
}

// HandleFunc registers a new route with a matcher for the URL path and
// handler function.
func (mr *MiddlewareRoute) HandleFunc(path string, f func(http.ResponseWriter, *http.Request)) *Route {
	return mr.newRoute().Path(path).HandlerFunc(f)
}

// Path registers a new route with a matcher for the URL path.
func (mr *MiddlewareRoute) Path(tpl string) *Route {
	return mr.newRoute().Path(tpl)
}

// PathPrefix registers a new route with a matcher for the URL path prefix.
func (mr *MiddlewareRoute) PathPrefix(tpl string) *Route {
	return mr.newRoute().PathPrefix(tpl)
}

// Host registers a new route with a matcher for the URL host.
func (mr *MiddlewareRoute) Host(tpl string) *Route {
	return mr.newRoute().Host(tpl)
}

// Methods registers a new route with a matcher for HTTP methods.
func (mr *MiddlewareRoute) Methods(methods ...string) *Route {
	return mr.newRoute().Methods(methods...)
}

// Headers registers a new route with a matcher for request header values.
func (mr *MiddlewareRoute) Headers(pairs ...string) *Route {
	return mr.newRoute().Headers(pairs...)
}

// HeadersRegexp registers a new route with a matcher for request header
// values using regexps.
func (mr *MiddlewareRoute) HeadersRegexp(pairs ...string) *Route {
	return mr.newRoute().HeadersRegexp(pairs...)
}

// Queries registers a new route with a matcher for URL query values.
func (mr *MiddlewareRoute) Queries(pairs ...string) *Route {
	return mr.newRoute().Queries(pairs...)
}

// Schemes registers a new route with a matcher for URL schemes.
func (mr *MiddlewareRoute) Schemes(schemes ...string) *Route {
	return mr.newRoute().Schemes(schemes...)
}

// MatcherFunc registers a new route with a custom matcher function.
func (mr *MiddlewareRoute) MatcherFunc(f MatcherFunc) *Route {
	return mr.newRoute().MatcherFunc(f)
}

// Name registers a new route with the given name.
func (mr *MiddlewareRoute) Name(name string) *Route {
	return mr.newRoute().Name(name)
}

// Route creates a subrouter with the given path prefix, applies the stored
// middleware to the subrouter, and invokes fn to register routes on it.
// Returns the MiddlewareRoute for chaining.
//
//	r.With(authMiddleware).Route("/admin", func(sub *mux.Router) {
//	    sub.HandleFunc("/users", handler)
//	})
func (mr *MiddlewareRoute) Route(path string, fn func(sub *Router)) *MiddlewareRoute {
	sub := mr.router.PathPrefix(path).Subrouter()
	sub.Use(mr.middlewares...)
	fn(sub)
	return mr
}

// Group creates a subrouter with no path prefix, applies the stored
// middleware to the subrouter, and invokes fn to register routes on it.
// Returns the MiddlewareRoute for chaining.
//
//	r.With(authMiddleware).Group(func(sub *mux.Router) {
//	    sub.HandleFunc("/dashboard", handler)
//	    sub.HandleFunc("/settings", handler)
//	})
func (mr *MiddlewareRoute) Group(fn func(sub *Router)) *MiddlewareRoute {
	sub := mr.router.NewRoute().Subrouter()
	sub.Use(mr.middlewares...)
	fn(sub)
	return mr
}

// With returns a new MiddlewareRoute that combines the existing middleware
// with additional middleware. This allows chaining:
//
//	r.With(logging).With(auth).HandleFunc("/admin", handler)
func (mr *MiddlewareRoute) With(mwf ...MiddlewareFunc) *MiddlewareRoute {
	combined := make([]MiddlewareFunc, len(mr.middlewares)+len(mwf))
	copy(combined, mr.middlewares)
	copy(combined[len(mr.middlewares):], mwf)
	return &MiddlewareRoute{
		router:      mr.router,
		middlewares: combined,
	}
}
