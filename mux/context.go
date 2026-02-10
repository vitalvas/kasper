package mux

import (
	"context"
	"errors"
	"net/http"
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
// context using a single WithContext call.
func setRouteContext(r *http.Request, route *Route, vars map[string]string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxKey, &routeContext{route: route, vars: vars})
	return r.WithContext(ctx)
}

// RouteMatch stores information about a matched route.
type RouteMatch struct {
	Route   *Route
	Handler http.Handler
	Vars    map[string]string

	// MatchErr is set to ErrMethodMismatch when the request method
	// does not match but the path does.
	MatchErr error

	// methodNotAllowed is set to true when the request method does not
	// match but the path does.
	methodNotAllowed bool
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
// the method defined against the route.
var ErrMethodMismatch = errors.New("method is not allowed")

// ErrNotFound is returned when no route match is found.
var ErrNotFound = errors.New("no matching route was found")

// SkipRouter is used as a return value from WalkFunc to indicate that the
// router that walk is about to descend into should be skipped.
// Named without Err prefix for gorilla/mux API compatibility.
var SkipRouter = errors.New("skip this router") //nolint:revive,staticcheck // gorilla/mux API compatibility
