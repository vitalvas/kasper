package mux

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// routeContextKey is an unexported type for the single context key.
type routeContextKey struct{}

// ctxKey is the single context key used to store both route and vars.
var ctxKey = routeContextKey{}

// routeContext holds the matched route and extracted variables.
type routeContext struct {
	route *Route
	vars  map[string]string
}

// Vars returns the route variables for the current request, if any.
func Vars(r *http.Request) map[string]string {
	if rc, ok := r.Context().Value(ctxKey).(*routeContext); ok {
		return rc.vars
	}
	return nil
}

// VarGet returns the value of a single route variable by name and a boolean
// indicating whether the variable exists.
func VarGet(r *http.Request, name string) (string, bool) {
	if rc, ok := r.Context().Value(ctxKey).(*routeContext); ok && rc.vars != nil {
		val, exists := rc.vars[name]
		return val, exists
	}
	return "", false
}

// CurrentRoute returns the matched route for the current request, if any.
// This only works when called inside the handler of the matched route
// because the matched route is stored in the request context.
func CurrentRoute(r *http.Request) *Route {
	if rc, ok := r.Context().Value(ctxKey).(*routeContext); ok {
		return rc.route
	}
	return nil
}

// SetURLVars sets the URL variables for the given request, returning the
// modified request. This is intended for testing route handlers.
func SetURLVars(r *http.Request, val map[string]string) *http.Request {
	var route *Route
	if rc, ok := r.Context().Value(ctxKey).(*routeContext); ok {
		route = rc.route
	}
	return setRouteContext(r, route, val)
}

// setRouteContext stores both the matched route and vars in the request
// context using a single WithContext call. For static routes (no variables),
// the routeContext is cached on the Route to avoid a heap allocation per
// request after the first dispatch.
func setRouteContext(r *http.Request, route *Route, vars map[string]string) *http.Request {
	var rc *routeContext
	if route != nil && vars == nil && route.buildVarsFunc == nil {
		route.staticCtxOnce.Do(func() {
			route.staticCtx = &routeContext{route: route}
		})
		rc = route.staticCtx
	} else {
		rc = &routeContext{route: route, vars: vars}
	}
	ctx := context.WithValue(r.Context(), ctxKey, rc)
	return r.WithContext(ctx)
}

// RouteMatch stores information about a matched route.
type RouteMatch struct {
	// Route is the matched route, if any.
	Route *Route

	// Handler is the handler to use for the matched route.
	Handler http.Handler

	// Vars contains the extracted path variables from the matched route.
	Vars map[string]string

	// MatchErr is set to ErrMethodMismatch when the request method
	// does not match but the path does. This triggers a 405 response
	// per RFC 7231 Section 6.5.5.
	MatchErr error

	// methodNotAllowed signals that the router should respond with
	// 405 Method Not Allowed (RFC 7231 Section 6.5.5) instead of
	// 404 Not Found (RFC 7231 Section 6.5.4).
	methodNotAllowed bool

	// parsedQuery caches the parsed query string to avoid repeated
	// url.Query() calls during matching and variable extraction.
	parsedQuery url.Values
}

// getQuery returns the parsed query string, caching it for reuse.
func (m *RouteMatch) getQuery(req *http.Request) url.Values {
	if m.parsedQuery == nil {
		m.parsedQuery = req.URL.Query()
	}
	return m.parsedQuery
}

// MatcherFunc is the function signature used by custom matchers.
type MatcherFunc func(*http.Request, *RouteMatch) bool

// Match implements the matcher interface.
func (m MatcherFunc) Match(r *http.Request, match *RouteMatch) bool {
	return m(r, match)
}

// MiddlewareFunc is a function which receives an http.Handler and returns
// another http.Handler. It can be used to wrap handlers with additional
// behavior such as logging, authentication, etc.
type MiddlewareFunc func(http.Handler) http.Handler

// Middleware allows MiddlewareFunc to implement the Middleware interface.
func (mw MiddlewareFunc) Middleware(handler http.Handler) http.Handler {
	return mw(handler)
}

// BuildVarsFunc is the function signature used by custom build vars functions.
type BuildVarsFunc func(map[string]string) map[string]string

// WalkFunc is the type of the function called for each route visited by Walk.
// At every invocation, it is given the current route and router, as well as
// a list of ancestor routes that led to the current route.
type WalkFunc func(route *Route, router *Router, ancestors []*Route) error

// ErrMethodMismatch is returned when the method in the request does not match
// the method defined against the route. Triggers 405 Method Not Allowed
// per RFC 7231 Section 6.5.5.
var ErrMethodMismatch = errors.New("method is not allowed")

// ErrNotFound is returned when no route match is found. Triggers 404 Not Found
// per RFC 7231 Section 6.5.4.
var ErrNotFound = errors.New("no matching route was found")

// SkipRouter is used as a return value from WalkFunc to indicate that the
// router that walk is about to descend into should be skipped.
// Named without Err prefix for gorilla/mux API compatibility.
var SkipRouter = errors.New("skip this router") //nolint:revive,staticcheck // gorilla/mux API compatibility
