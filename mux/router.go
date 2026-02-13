package mux

import (
	"net/http"
	"strings"
	"sync"
)

// Router registers routes to be matched and dispatches a handler.
//
// It implements the http.Handler interface, so it can be registered to serve
// requests:
//
//	r := mux.NewRouter()
//	r.HandleFunc("/", handler)
//	http.ListenAndServe(":8080", r)
type Router struct {
	// NotFoundHandler is called when no route matches.
	// If nil, http.NotFoundHandler() is used.
	// Corresponds to 404 Not Found per RFC 7231 Section 6.5.4.
	NotFoundHandler http.Handler

	// MethodNotAllowedHandler is called when a route matches the path
	// but not the method. If nil, a default 405 handler is used.
	// Per RFC 7231 Section 6.5.5, the Allow header is always set before
	// this handler is invoked.
	MethodNotAllowedHandler http.Handler

	parent      parentRoute
	routes      []*Route
	namedRoutes map[string]*Route
	middlewares []MiddlewareFunc

	// handlerCache caches the middleware-wrapped handler per route
	// to avoid re-wrapping on every request.
	handlerCache sync.Map // map[*Route]http.Handler

	strictSlash    bool
	skipClean      bool
	useEncodedPath bool
}

// NewRouter returns a new router instance.
func NewRouter() *Router {
	return &Router{
		namedRoutes: make(map[string]*Route),
	}
}

// ServeHTTP dispatches the handler registered in the matched route.
// Implements http.Handler per RFC 7230 Section 3.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Normalize the request path per RFC 3986 Section 5.2.4
	// (removing dot segments) unless SkipClean is enabled.
	if !r.skipClean {
		path := req.URL.Path
		if r.useEncodedPath {
			path = requestURIPath(req.URL)
		}
		if cleaned := cleanPath(path); cleaned != path {
			u := *req.URL
			u.Path = cleaned
			u.RawPath = ""
			req = req.Clone(req.Context())
			req.URL = &u
		}
	}

	var match RouteMatch
	var handler http.Handler

	if r.Match(req, &match) {
		handler = match.Handler
		if handler == nil {
			handler = defaultNotFoundHandler
		}
		req = setRouteContext(req, match.Route, match.Vars)
	} else {
		if match.methodNotAllowed {
			// RFC 7231 Section 6.5.5: the origin server MUST generate an
			// Allow header field in a 405 response.
			allowed := allowedMethods(r, req)
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			handler = r.MethodNotAllowedHandler
			if handler == nil {
				handler = defaultMethodNotAllowedHandler
			}
		} else {
			handler = r.NotFoundHandler
			if handler == nil {
				handler = defaultNotFoundHandler
			}
		}
	}

	// Apply strict slash redirect if needed.
	if match.Route != nil && match.Route.strictSlash {
		p := strings.TrimSuffix(req.URL.Path, "/")
		tpl := match.Route.regexp.path
		if tpl != nil {
			tplPath := tpl.template
			// Check if template ends with slash but URL doesn't, or vice versa.
			tplHasSlash := strings.HasSuffix(tplPath, "/")
			urlHasSlash := strings.HasSuffix(req.URL.Path, "/")
			if tplHasSlash != urlHasSlash && p != "" {
				u := *req.URL
				if tplHasSlash {
					u.Path += "/"
				} else {
					u.Path = strings.TrimSuffix(u.Path, "/")
				}
				// RFC 7538 Section 3: 308 preserves the request method,
				// unlike 301 which allows clients to change POST to GET.
				http.Redirect(w, req, u.String(), http.StatusPermanentRedirect)
				return
			}
		}
	}

	handler.ServeHTTP(w, req)
}

// Match attempts to match the given request against the router's routes.
// Distinguishes between 404 Not Found (RFC 7231 Section 6.5.4) and
// 405 Method Not Allowed (RFC 7231 Section 6.5.5) by tracking method
// mismatches independently across route iteration.
func (r *Router) Match(req *http.Request, match *RouteMatch) bool {
	var methodNotAllowed bool
	for _, route := range r.routes {
		if route.buildOnly {
			continue
		}
		if route.Match(req, match) {
			if match.Handler != nil && len(r.middlewares) > 0 {
				if cached, ok := r.handlerCache.Load(match.Route); ok {
					match.Handler = cached.(http.Handler)
				} else {
					wrapped := r.applyMiddleware(match.Handler)
					r.handlerCache.Store(match.Route, wrapped)
					match.Handler = wrapped
				}
			}
			return true
		}
		if match.MatchErr == ErrMethodMismatch {
			methodNotAllowed = true
		}
	}

	if methodNotAllowed {
		match.MatchErr = ErrMethodMismatch
		match.methodNotAllowed = true
		return false
	}

	match.MatchErr = ErrNotFound
	return false
}

// StrictSlash defines the trailing slash behavior for new routes.
// When true, if the route path is "/path/", accessing "/path" will redirect
// to "/path/" and vice versa. Uses 308 Permanent Redirect (RFC 7538) to
// preserve the original request method.
func (r *Router) StrictSlash(value bool) *Router {
	r.strictSlash = value
	return r
}

// SkipClean defines the path cleaning behavior for new routes.
// When true, the path will not be cleaned (path.Clean will not be called).
func (r *Router) SkipClean(value bool) *Router {
	r.skipClean = value
	return r
}

// UseEncodedPath tells the router to match the percent-encoded original path
// (RFC 3986 Section 2.1) to the routes, instead of the decoded path.
func (r *Router) UseEncodedPath() *Router {
	r.useEncodedPath = true
	return r
}

// --- Route factory methods ---

// NewRoute creates an empty route for configuration.
func (r *Router) NewRoute() *Route {
	route := &Route{
		parent:         r,
		namedRoutes:    r.namedRoutes,
		strictSlash:    r.strictSlash,
		skipClean:      r.skipClean,
		useEncodedPath: r.useEncodedPath,
	}
	r.routes = append(r.routes, route)
	return route
}

// Handle registers a new route with a matcher for the URL path and handler.
func (r *Router) Handle(path string, handler http.Handler) *Route {
	return r.NewRoute().Path(path).Handler(handler)
}

// HandleFunc registers a new route with a matcher for the URL path and
// handler function.
func (r *Router) HandleFunc(path string, f func(http.ResponseWriter, *http.Request)) *Route {
	return r.NewRoute().Path(path).HandlerFunc(f)
}

// Path registers a new route with a matcher for the URL path.
func (r *Router) Path(tpl string) *Route {
	return r.NewRoute().Path(tpl)
}

// PathPrefix registers a new route with a matcher for the URL path prefix.
func (r *Router) PathPrefix(tpl string) *Route {
	return r.NewRoute().PathPrefix(tpl)
}

// Host registers a new route with a matcher for the URL host.
func (r *Router) Host(tpl string) *Route {
	return r.NewRoute().Host(tpl)
}

// Methods registers a new route with a matcher for HTTP methods.
func (r *Router) Methods(methods ...string) *Route {
	return r.NewRoute().Methods(methods...)
}

// Schemes registers a new route with a matcher for URL schemes.
func (r *Router) Schemes(schemes ...string) *Route {
	return r.NewRoute().Schemes(schemes...)
}

// Headers registers a new route with a matcher for request header values.
func (r *Router) Headers(pairs ...string) *Route {
	return r.NewRoute().Headers(pairs...)
}

// HeadersRegexp registers a new route with a matcher for request header
// values using regexps.
func (r *Router) HeadersRegexp(pairs ...string) *Route {
	return r.NewRoute().HeadersRegexp(pairs...)
}

// Queries registers a new route with a matcher for URL query values.
func (r *Router) Queries(pairs ...string) *Route {
	return r.NewRoute().Queries(pairs...)
}

// MatcherFunc registers a new route with a custom matcher function.
func (r *Router) MatcherFunc(f MatcherFunc) *Route {
	return r.NewRoute().MatcherFunc(f)
}

// Name registers a new route with the given name.
func (r *Router) Name(name string) *Route {
	return r.NewRoute().Name(name)
}

// BuildVarsFunc registers a new route with a custom build vars function.
func (r *Router) BuildVarsFunc(f BuildVarsFunc) *Route {
	return r.NewRoute().BuildVarsFunc(f)
}

// Get returns a route registered with the given name.
func (r *Router) Get(name string) *Route {
	return r.namedRoutes[name]
}

// GetRoute returns a route registered with the given name (alias for Get).
func (r *Router) GetRoute(name string) *Route {
	return r.Get(name)
}

// Walk walks the router and all its subrouters, calling walkFn for each route
// in the tree.
func (r *Router) Walk(walkFn WalkFunc) error {
	return r.walk(walkFn, nil)
}

func (r *Router) walk(walkFn WalkFunc, ancestors []*Route) error {
	for _, route := range r.routes {
		err := walkFn(route, r, ancestors)
		if err == SkipRouter {
			continue
		}
		if err != nil {
			return err
		}
		if route.handler != nil {
			if sr, ok := route.handler.(*Router); ok {
				err := sr.walk(walkFn, append(ancestors, route))
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// --- parentRoute interface implementation ---

func (r *Router) getNamedRoutes() map[string]*Route {
	return r.namedRoutes
}

func (r *Router) getRegexpGroup() *routeRegexpGroup {
	if r.parent != nil {
		return r.parent.getRegexpGroup()
	}
	return nil
}

func (r *Router) buildVars(m map[string]string) map[string]string {
	if r.parent != nil {
		m = r.parent.buildVars(m)
	}
	return m
}

// applyMiddleware wraps the handler with all registered middleware.
func (r *Router) applyMiddleware(handler http.Handler) http.Handler {
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		handler = r.middlewares[i].Middleware(handler)
	}
	return handler
}

// Use appends a MiddlewareFunc to the chain. Middleware is applied to
// matched handlers only.
func (r *Router) Use(mwf ...MiddlewareFunc) {
	r.middlewares = append(r.middlewares, mwf...)
}
