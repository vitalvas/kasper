package mux

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// matcher is the interface implemented by route matchers.
type matcher interface {
	Match(*http.Request, *RouteMatch) bool
}

// parentRoute is the interface implemented by types that can serve as
// a route's parent (Router or Route via subrouter).
type parentRoute interface {
	getNamedRoutes() map[string]*Route
	getRegexpGroup() *routeRegexpGroup
	buildVars(map[string]string) map[string]string
}

// Route stores information to match a request and build URLs.
type Route struct {
	parent      parentRoute
	handler     http.Handler
	matchers    []matcher
	regexp      routeRegexpGroup
	name        string
	err         error
	namedRoutes map[string]*Route
	buildOnly   bool

	strictSlash    bool
	skipClean      bool
	useEncodedPath bool
	buildVarsFunc  BuildVarsFunc
	buildScheme    string
}

// Match matches this route against the request.
func (r *Route) Match(req *http.Request, match *RouteMatch) bool {
	if r.err != nil {
		return false
	}

	var methodMismatch bool

	// Check all matchers.
	for _, m := range r.matchers {
		if !m.Match(req, match) {
			if _, ok := m.(methodMatcher); ok {
				methodMismatch = true
				continue
			}
			if match.MatchErr == ErrMethodMismatch {
				methodMismatch = true
				continue
			}
			return false
		}
	}

	// Check host regexp.
	if r.regexp.host != nil {
		if !r.regexp.host.Match(req, match) {
			return false
		}
	}

	// Check path regexp.
	if r.regexp.path != nil {
		if !r.regexp.path.Match(req, match) {
			return false
		}
	}

	// Check query regexps.
	for _, q := range r.regexp.queries {
		if !q.Match(req, match) {
			return false
		}
	}

	// If method didn't match but everything else did, record the mismatch.
	if methodMismatch {
		match.MatchErr = ErrMethodMismatch
		return false
	}

	// If the handler is a Router (subrouter), delegate to it.
	if r.handler != nil {
		if router, ok := r.handler.(*Router); ok {
			return router.Match(req, match)
		}
	}

	match.Route = r
	match.Handler = r.handler
	r.regexp.setMatch(req, match, r)

	// Apply buildVarsFunc if set.
	if r.buildVarsFunc != nil {
		match.Vars = r.buildVarsFunc(match.Vars)
	}
	if r.parent != nil {
		match.Vars = r.parent.buildVars(match.Vars)
	}

	return true
}

// --- Matchers ---

// addMatcher adds a matcher to the route.
func (r *Route) addMatcher(m matcher) *Route {
	if r.err == nil {
		r.matchers = append(r.matchers, m)
	}
	return r
}

// addRegexpMatcher adds a regexp-based matcher for path, host, or query.
func (r *Route) addRegexpMatcher(tpl string, typ regexpType) error {
	if r.err != nil {
		return r.err
	}

	// For path/prefix, prepend parent's path template if exists.
	if typ == regexpTypePath || typ == regexpTypePrefix {
		if r.parent != nil {
			if g := r.parent.getRegexpGroup(); g != nil && g.path != nil {
				tpl = strings.TrimRight(g.path.template, "/") + tpl
			}
		}
	}

	// For host, append parent's host template if exists.
	if typ == regexpTypeHost {
		if r.parent != nil {
			if g := r.parent.getRegexpGroup(); g != nil && g.host != nil {
				tpl = tpl + "." + g.host.template
			}
		}
	}

	rr, err := newRouteRegexp(tpl, typ, routeRegexpOptions{
		strictSlash:    r.strictSlash,
		useEncodedPath: r.useEncodedPath,
	})
	if err != nil {
		return err
	}

	switch typ {
	case regexpTypePath, regexpTypePrefix:
		// Check for unique vars between host and path.
		if r.regexp.host != nil {
			if err := uniqueVars(rr.varsN, r.regexp.host.varsN); err != nil {
				return err
			}
		}
		r.regexp.path = rr
	case regexpTypeHost:
		// Check for unique vars between host and path.
		if r.regexp.path != nil {
			if err := uniqueVars(rr.varsN, r.regexp.path.varsN); err != nil {
				return err
			}
		}
		r.regexp.host = rr
	case regexpTypeQuery:
		r.regexp.queries = append(r.regexp.queries, rr)
	}
	return nil
}

// Handler sets a handler for the route.
func (r *Route) Handler(handler http.Handler) *Route {
	if r.err == nil {
		r.handler = handler
	}
	return r
}

// HandlerFunc sets a handler function for the route.
func (r *Route) HandlerFunc(f func(http.ResponseWriter, *http.Request)) *Route {
	return r.Handler(http.HandlerFunc(f))
}

// GetHandler returns the handler for the route, if any.
func (r *Route) GetHandler() http.Handler {
	return r.handler
}

// Name sets the name for the route, used to build URLs.
// Returns an error if the name was already used.
func (r *Route) Name(name string) *Route {
	if r.name != "" {
		r.err = fmt.Errorf("mux: route already has name %q, can't set %q", r.name, name)
		return r
	}
	if r.err == nil {
		r.name = name
		if r.namedRoutes != nil {
			r.namedRoutes[name] = r
		}
	}
	return r
}

// GetName returns the name for the route, if any.
func (r *Route) GetName() string {
	return r.name
}

// Path adds a path matcher to the route per RFC 3986 Section 3.3.
func (r *Route) Path(tpl string) *Route {
	r.err = r.addRegexpMatcher(tpl, regexpTypePath)
	return r
}

// PathPrefix adds a path prefix matcher to the route per RFC 3986 Section 3.3.
func (r *Route) PathPrefix(tpl string) *Route {
	r.err = r.addRegexpMatcher(tpl, regexpTypePrefix)
	return r
}

// Host adds a host matcher to the route per RFC 7230 Section 5.4.
func (r *Route) Host(tpl string) *Route {
	r.err = r.addRegexpMatcher(tpl, regexpTypeHost)
	return r
}

// Methods adds a method matcher to the route. Methods are matched against
// the request method token defined in RFC 7231 Section 4.
// Calling Methods multiple times replaces the previous method matcher.
func (r *Route) Methods(methods ...string) *Route {
	for i, m := range methods {
		methods[i] = strings.ToUpper(m)
	}
	// Remove existing method matchers to allow replacing via chained calls.
	filtered := r.matchers[:0]
	for _, m := range r.matchers {
		if _, ok := m.(methodMatcher); !ok {
			filtered = append(filtered, m)
		}
	}
	r.matchers = filtered
	return r.addMatcher(methodMatcher(methods))
}

// Headers adds a matcher for request header values per RFC 7230 Section 3.2.
// It accepts pairs of header names and values. The value can be empty,
// in which case the matcher will only check for the header presence.
func (r *Route) Headers(pairs ...string) *Route {
	if r.err == nil {
		m, err := mapFromPairsToString(pairs...)
		if err != nil {
			r.err = err
			return r
		}
		return r.addMatcher(headerMatcher(m))
	}
	return r
}

// HeadersRegexp adds a matcher for request header values using regexps.
// Header names are case-insensitive per RFC 7230 Section 3.2.
func (r *Route) HeadersRegexp(pairs ...string) *Route {
	if r.err == nil {
		m, err := mapFromPairsToRegex(pairs...)
		if err != nil {
			r.err = err
			return r
		}
		return r.addMatcher(headerRegexMatcher(m))
	}
	return r
}

// Queries adds matchers for URL query values per RFC 3986 Section 3.4.
func (r *Route) Queries(pairs ...string) *Route {
	length, err := checkPairs(pairs...)
	if err != nil {
		r.err = err
		return r
	}
	for i := 0; i < length; i++ {
		if r.err = r.addRegexpMatcher(pairs[i*2]+"="+pairs[i*2+1], regexpTypeQuery); r.err != nil {
			return r
		}
	}
	return r
}

// Schemes adds a matcher for URL schemes per RFC 7230 Section 2.7.
func (r *Route) Schemes(schemes ...string) *Route {
	for i, s := range schemes {
		schemes[i] = strings.ToLower(s)
	}
	if len(schemes) > 0 {
		r.buildScheme = schemes[0]
	}
	return r.addMatcher(schemeMatcher(schemes))
}

// BuildOnly sets the route to be used only for URL building,
// not for request matching.
func (r *Route) BuildOnly() *Route {
	r.buildOnly = true
	return r
}

// Subrouter creates a new Router for the route.
func (r *Route) Subrouter() *Router {
	router := &Router{
		parent:         r,
		namedRoutes:    r.namedRoutes,
		strictSlash:    r.strictSlash,
		skipClean:      r.skipClean,
		useEncodedPath: r.useEncodedPath,
	}
	r.handler = router
	return router
}

// SkipClean reports whether the path cleaning is disabled for this route.
func (r *Route) SkipClean() bool {
	return r.skipClean
}

// MatcherFunc adds a custom matcher function to the route.
func (r *Route) MatcherFunc(f MatcherFunc) *Route {
	return r.addMatcher(f)
}

// BuildVarsFunc adds a custom variable builder function to the route.
func (r *Route) BuildVarsFunc(f BuildVarsFunc) *Route {
	if r.buildVarsFunc != nil {
		old := r.buildVarsFunc
		r.buildVarsFunc = func(m map[string]string) map[string]string {
			return f(old(m))
		}
	} else {
		r.buildVarsFunc = f
	}
	return r
}

// --- URL Building ---

// URL builds a URL for the route per RFC 3986 Section 5.3 (component
// recomposition). It accepts a sequence of key/value pairs for the route
// variables. Returns an error if the route has no path template or if a
// variable is missing/invalid.
func (r *Route) URL(pairs ...string) (*url.URL, error) {
	if r.err != nil {
		return nil, r.err
	}
	values, err := r.prepareVars(pairs...)
	if err != nil {
		return nil, err
	}
	var scheme, host, path string
	if r.regexp.host != nil {
		if host, err = r.regexp.host.url(values); err != nil {
			return nil, err
		}
		scheme = "http"
		if r.buildScheme != "" {
			scheme = r.buildScheme
		}
	}
	if r.regexp.path != nil {
		if path, err = r.regexp.path.url(values); err != nil {
			return nil, err
		}
	}
	return &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}, nil
}

// URLHost builds the host part of the URL per RFC 3986 Section 3.2.2.
func (r *Route) URLHost(pairs ...string) (*url.URL, error) {
	if r.err != nil {
		return nil, r.err
	}
	values, err := r.prepareVars(pairs...)
	if err != nil {
		return nil, err
	}
	if r.regexp.host == nil {
		return nil, errors.New("mux: route doesn't have a host")
	}
	host, err := r.regexp.host.url(values)
	if err != nil {
		return nil, err
	}
	scheme := "http"
	if r.buildScheme != "" {
		scheme = r.buildScheme
	}
	return &url.URL{
		Scheme: scheme,
		Host:   host,
	}, nil
}

// URLPath builds the path part of the URL per RFC 3986 Section 3.3.
func (r *Route) URLPath(pairs ...string) (*url.URL, error) {
	if r.err != nil {
		return nil, r.err
	}
	values, err := r.prepareVars(pairs...)
	if err != nil {
		return nil, err
	}
	if r.regexp.path == nil {
		return nil, errors.New("mux: route doesn't have a path")
	}
	path, err := r.regexp.path.url(values)
	if err != nil {
		return nil, err
	}
	return &url.URL{
		Path: path,
	}, nil
}

// prepareVars converts key/value pairs to a map and applies buildVarsFunc.
func (r *Route) prepareVars(pairs ...string) (map[string]string, error) {
	m, err := mapFromPairsToString(pairs...)
	if err != nil {
		return nil, err
	}
	return r.buildVarsFrom(m), nil
}

// buildVarsFrom applies the buildVarsFunc chain to the given vars.
func (r *Route) buildVarsFrom(m map[string]string) map[string]string {
	if r.buildVarsFunc != nil {
		m = r.buildVarsFunc(m)
	}
	if r.parent != nil {
		m = r.parent.buildVars(m)
	}
	return m
}

// --- Inspection ---

// GetPathTemplate returns the template for the route path, if defined.
func (r *Route) GetPathTemplate() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.regexp.path == nil {
		return "", errors.New("mux: route doesn't have a path")
	}
	return r.regexp.path.template, nil
}

// GetPathRegexp returns the compiled regexp for the route path, if defined.
func (r *Route) GetPathRegexp() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.regexp.path == nil {
		return "", errors.New("mux: route doesn't have a path")
	}
	return r.regexp.path.regexp.String(), nil
}

// GetHostTemplate returns the template for the route host, if defined.
func (r *Route) GetHostTemplate() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.regexp.host == nil {
		return "", errors.New("mux: route doesn't have a host")
	}
	return r.regexp.host.template, nil
}

// GetMethods returns the methods the route matches against.
func (r *Route) GetMethods() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	for _, m := range r.matchers {
		if methods, ok := m.(methodMatcher); ok {
			return []string(methods), nil
		}
	}
	return nil, errors.New("mux: route doesn't have methods")
}

// GetQueriesTemplates returns the query templates for the route.
func (r *Route) GetQueriesTemplates() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	if len(r.regexp.queries) == 0 {
		return nil, errors.New("mux: route doesn't have queries")
	}
	templates := make([]string, len(r.regexp.queries))
	for i, q := range r.regexp.queries {
		templates[i] = q.queryKey + "=" + q.template
	}
	return templates, nil
}

// GetQueriesRegexp returns the compiled query regexps for the route.
func (r *Route) GetQueriesRegexp() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	if len(r.regexp.queries) == 0 {
		return nil, errors.New("mux: route doesn't have queries")
	}
	regexps := make([]string, len(r.regexp.queries))
	for i, q := range r.regexp.queries {
		regexps[i] = q.regexp.String()
	}
	return regexps, nil
}

// GetVarNames returns the variable names for the route.
func (r *Route) GetVarNames() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	var varNames []string
	if r.regexp.host != nil {
		varNames = append(varNames, r.regexp.host.varsN...)
	}
	if r.regexp.path != nil {
		varNames = append(varNames, r.regexp.path.varsN...)
	}
	return varNames, nil
}

// GetError returns any error that was set on the route.
func (r *Route) GetError() error {
	return r.err
}

// --- parentRoute interface implementation ---

func (r *Route) getNamedRoutes() map[string]*Route {
	return r.namedRoutes
}

func (r *Route) getRegexpGroup() *routeRegexpGroup {
	return &r.regexp
}

func (r *Route) buildVars(m map[string]string) map[string]string {
	if r.buildVarsFunc != nil {
		m = r.buildVarsFunc(m)
	}
	if r.parent != nil {
		m = r.parent.buildVars(m)
	}
	return m
}

// --- Internal matchers ---

// methodMatcher matches the request method token (RFC 7231 Section 4)
// against a list of allowed methods.
type methodMatcher []string

func (m methodMatcher) Match(r *http.Request, _ *RouteMatch) bool {
	return matchInArray([]string(m), r.Method)
}

// headerMatcher matches request headers against expected values.
// Header names are case-insensitive per RFC 7230 Section 3.2.
type headerMatcher map[string]string

func (m headerMatcher) Match(r *http.Request, _ *RouteMatch) bool {
	return matchMapWithString(map[string]string(m), map[string][]string(r.Header), true)
}

// headerRegexMatcher matches request headers against regexp patterns.
// Header names are case-insensitive per RFC 7230 Section 3.2.
type headerRegexMatcher map[string]*regexp.Regexp

func (m headerRegexMatcher) Match(r *http.Request, _ *RouteMatch) bool {
	return matchMapWithRegex(map[string]*regexp.Regexp(m), map[string][]string(r.Header), true)
}

// schemeMatcher matches the request URL scheme per RFC 7230 Section 2.7.
// Schemes are case-insensitive per RFC 3986 Section 3.1.
type schemeMatcher []string

func (m schemeMatcher) Match(r *http.Request, _ *RouteMatch) bool {
	scheme := r.URL.Scheme
	// Infer scheme from TLS state when not explicitly set,
	// per RFC 7230 Section 2.7.1 (http) and 2.7.2 (https).
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return matchInArray([]string(m), scheme)
}
