package muxhandlers

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// ErrWildcardCredentials is returned when AllowedOrigins contains "*" and
// AllowCredentials is true. Use AllowOriginFunc for dynamic origin checks
// with credentials.
var ErrWildcardCredentials = errors.New("wildcard origin \"*\" cannot be used with AllowCredentials; use AllowOriginFunc instead")

// CORSConfig configures the CORS middleware behaviour.
//
// Spec references:
//   - CORS protocol: https://fetch.spec.whatwg.org/#http-cors-protocol
//   - Web Origin:    https://www.rfc-editor.org/rfc/rfc6454
//   - HTTP Vary:     https://www.rfc-editor.org/rfc/rfc9110#field.vary
type CORSConfig struct {
	// AllowedOrigins is a list of exact origin strings, "*" for wildcard,
	// or subdomain wildcard patterns like "https://*.example.com".
	AllowedOrigins []string

	// AllowOriginFunc is an optional dynamic callback invoked when the
	// origin does not match any entry in AllowedOrigins. Return true to allow.
	AllowOriginFunc func(origin string) bool

	// AllowedMethods overrides the set of methods advertised in preflight
	// and actual responses. When empty the middleware auto-discovers methods
	// from the router for the matched route.
	AllowedMethods []string

	// AllowedHeaders lists the headers the client may send in the actual
	// request. When empty the middleware reflects the Access-Control-Request-Headers
	// value from the preflight request. Use "*" to reflect all requested headers.
	AllowedHeaders []string

	// ExposeHeaders lists the headers the browser may expose to client code.
	ExposeHeaders []string

	// AllowCredentials sets Access-Control-Allow-Credentials: true.
	// Per the Fetch Standard, "*" cannot be used as Allow-Origin when
	// credentials are enabled; the middleware returns ErrWildcardCredentials.
	AllowCredentials bool

	// MaxAge is the duration in seconds a preflight result may be cached.
	// Positive values are sent as-is, negative values emit "0", zero omits the header.
	MaxAge int

	// OptionsStatusCode overrides the HTTP status code for preflight responses.
	// When zero (default) the middleware uses 204 No Content.
	OptionsStatusCode int

	// OptionsPassthrough, when true, sets CORS headers on preflight but
	// forwards the request to the next handler instead of terminating the chain.
	OptionsPassthrough bool

	// AllowPrivateNetwork, when true, responds to Access-Control-Request-Private-Network
	// preflight headers with Access-Control-Allow-Private-Network: true.
	// See https://wicg.github.io/private-network-access/
	AllowPrivateNetwork bool
}

// wildcardPattern represents a subdomain wildcard pattern split at the "*".
type wildcardPattern struct {
	prefix string
	suffix string
}

// hasWildcardOrigin reports whether AllowedOrigins contains "*".
func (c *CORSConfig) hasWildcardOrigin() bool {
	return slices.Contains(c.AllowedOrigins, "*")
}

// setCORSOriginHeaders sets Access-Control-Allow-Origin, Vary, and
// Access-Control-Allow-Credentials on the response.
func setCORSOriginHeaders(w http.ResponseWriter, cfg *CORSConfig, origin string) {
	if cfg.hasWildcardOrigin() && !cfg.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
	}

	if cfg.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}

// parseOrigins normalizes AllowedOrigins to lowercase and splits them into
// exact matches and wildcard patterns. Returns an error if a pattern contains
// multiple wildcards.
func parseOrigins(origins []string) ([]string, []wildcardPattern, error) {
	var exact []string
	var patterns []wildcardPattern

	for _, o := range origins {
		if o == "*" {
			exact = append(exact, o)
			continue
		}

		lower := strings.ToLower(o)

		if strings.Contains(lower, "*") {
			parts := strings.SplitN(lower, "*", 2)
			if strings.Contains(parts[1], "*") {
				return nil, nil, errors.New("origin pattern contains multiple wildcards: " + o)
			}

			patterns = append(patterns, wildcardPattern{
				prefix: parts[0],
				suffix: parts[1],
			})
		} else {
			exact = append(exact, lower)
		}
	}

	return exact, patterns, nil
}

// matchOrigin reports whether originLower matches any exact origin or wildcard pattern.
func matchOrigin(originLower string, exactOrigins []string, patterns []wildcardPattern) bool {
	for _, o := range exactOrigins {
		if o == "*" || o == originLower {
			return true
		}
	}

	for _, wp := range patterns {
		if len(originLower) >= len(wp.prefix)+len(wp.suffix) &&
			strings.HasPrefix(originLower, wp.prefix) &&
			strings.HasSuffix(originLower, wp.suffix) {
			return true
		}
	}

	return false
}

// CORSMiddleware returns a middleware that implements the full CORS protocol
// per the Fetch Standard (https://fetch.spec.whatwg.org/#http-cors-protocol).
// It validates the Origin header (RFC 6454), handles preflight OPTIONS
// requests, and sets the appropriate response headers.
//
// It returns an error if the configuration is invalid (e.g. wildcard origin
// combined with AllowCredentials).
//
// Because the router middleware only runs for matched routes, this function
// also sets the router's MethodNotAllowedHandler to intercept CORS preflight
// OPTIONS requests that would otherwise receive a 405 response.
func CORSMiddleware(r *mux.Router, cfg CORSConfig) (mux.MiddlewareFunc, error) {
	if cfg.hasWildcardOrigin() && cfg.AllowCredentials {
		return nil, ErrWildcardCredentials
	}

	exactOrigins, wildcardPatterns, err := parseOrigins(cfg.AllowedOrigins)
	if err != nil {
		return nil, err
	}

	isAllowed := func(originLower, rawOrigin string) bool {
		if matchOrigin(originLower, exactOrigins, wildcardPatterns) {
			return true
		}

		if cfg.AllowOriginFunc != nil {
			return cfg.AllowOriginFunc(rawOrigin)
		}

		return false
	}

	hasSpecificOrigins := !cfg.hasWildcardOrigin() &&
		(len(exactOrigins) > 0 || len(wildcardPatterns) > 0 || cfg.AllowOriginFunc != nil)

	// Feature 6: Check for AllowedHeaders wildcard.
	headersWildcard := slices.Contains(cfg.AllowedHeaders, "*")

	// Feature 4: Resolve preflight status code.
	preflightStatus := cfg.OptionsStatusCode
	if preflightStatus == 0 {
		preflightStatus = http.StatusNoContent
	}

	prevHandler := r.MethodNotAllowedHandler

	r.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rawOrigin := req.Header.Get("Origin")
		originLower := strings.ToLower(rawOrigin)

		if rawOrigin != "" && isAllowed(originLower, rawOrigin) &&
			req.Method == http.MethodOptions &&
			req.Header.Get("Access-Control-Request-Method") != "" {
			setCORSOriginHeaders(w, &cfg, rawOrigin)
			handlePreflight(w, req, r, &cfg, preflightStatus, headersWildcard, true)
			return
		}

		if prevHandler != nil {
			prevHandler.ServeHTTP(w, req)
			return
		}

		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			rawOrigin := req.Header.Get("Origin")

			if rawOrigin == "" {
				// Feature 7: Vary on non-CORS requests with specific origins.
				if hasSpecificOrigins {
					w.Header().Add("Vary", "Origin")
				}

				next.ServeHTTP(w, req)
				return
			}

			originLower := strings.ToLower(rawOrigin)

			if !isAllowed(originLower, rawOrigin) {
				next.ServeHTTP(w, req)
				return
			}

			setCORSOriginHeaders(w, &cfg, rawOrigin)

			if req.Method == http.MethodOptions && req.Header.Get("Access-Control-Request-Method") != "" {
				handlePreflight(w, req, r, &cfg, preflightStatus, headersWildcard, !cfg.OptionsPassthrough)

				// Feature 5: OptionsPassthrough forwards to next handler.
				if cfg.OptionsPassthrough {
					next.ServeHTTP(w, req)
				}

				return
			}

			methods := cfg.AllowedMethods
			if len(methods) == 0 {
				if discovered, err := getAllMethodsForRoute(r, req); err == nil {
					methods = discovered
				}
			}

			if len(methods) > 0 {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ","))
			}

			if len(cfg.ExposeHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ","))
			}

			next.ServeHTTP(w, req)
		})
	}, nil
}

func handlePreflight(w http.ResponseWriter, req *http.Request, r *mux.Router, cfg *CORSConfig, statusCode int, headersWildcard bool, writeStatus bool) {
	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		if discovered, err := getAllMethodsForRoute(r, req); err == nil {
			methods = discovered
		}
	}

	if len(methods) > 0 {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ","))
	}

	if headersWildcard {
		if reqHeaders := req.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		}
	} else if len(cfg.AllowedHeaders) > 0 {
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ","))
	} else if reqHeaders := req.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
		w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
	}

	if cfg.MaxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
	} else if cfg.MaxAge < 0 {
		w.Header().Set("Access-Control-Max-Age", "0")
	}

	if cfg.AllowPrivateNetwork && req.Header.Get("Access-Control-Request-Private-Network") == "true" {
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.Header().Add("Vary", "Access-Control-Request-Private-Network")
	}

	w.Header().Add("Vary", "Access-Control-Request-Method")
	w.Header().Add("Vary", "Access-Control-Request-Headers")

	if writeStatus {
		w.WriteHeader(statusCode)
	}
}

// getAllMethodsForRoute returns all HTTP methods registered for routes
// matching the given request's path.
func getAllMethodsForRoute(router *mux.Router, req *http.Request) ([]string, error) {
	var allMethods []string

	router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		methods, err := route.GetMethods()
		if err != nil {
			return nil
		}

		for _, method := range methods {
			testReq := req.Clone(req.Context())
			testReq.Method = method
			match := &mux.RouteMatch{}
			if route.Match(testReq, match) {
				allMethods = append(allMethods, method)
			}
		}

		return nil
	})

	if len(allMethods) == 0 {
		return nil, mux.ErrNotFound
	}

	return allMethods, nil
}
