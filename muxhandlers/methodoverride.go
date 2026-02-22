package muxhandlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// ErrInvalidOverrideMethod is returned when MethodOverrideConfig.AllowedMethods
// or MethodOverrideConfig.OriginalMethods contains an invalid HTTP method.
var ErrInvalidOverrideMethod = errors.New("method override: allowed methods must be valid HTTP methods")

// MethodOverrideConfig configures the Method Override middleware behaviour.
type MethodOverrideConfig struct {
	// HeaderNames is the list of header names checked in order.
	// The first non-empty header value is used as the override.
	// When nil, defaults to
	// ["X-HTTP-Method-Override", "X-Method-Override", "X-HTTP-Method"].
	HeaderNames []string

	// OriginalMethods is the set of HTTP methods eligible for override.
	// When nil, defaults to [POST].
	OriginalMethods []string

	// AllowedMethods restricts which methods can be used as overrides.
	// When nil, defaults to PUT, PATCH, DELETE, HEAD, OPTIONS.
	AllowedMethods []string
}

// defaultOverrideHeaders is the default set of header names checked for
// method override when HeaderNames is nil.
var defaultOverrideHeaders = []string{
	"X-HTTP-Method-Override",
	"X-Method-Override",
	"X-HTTP-Method",
}

// defaultOriginalMethods is the set of HTTP methods eligible for override
// when OriginalMethods is nil.
var defaultOriginalMethods = []string{http.MethodPost}

// defaultOverrideMethods is the set of methods allowed as overrides when
// AllowedMethods is nil.
var defaultOverrideMethods = []string{
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodHead,
	http.MethodOptions,
}

// MethodOverrideMiddleware returns a middleware that allows clients to override
// the HTTP method via a configurable header. The first non-empty header value
// from HeaderNames is uppercased and checked against the allowed set. When
// allowed, r.Method is set to the override value and the header is removed.
// Override is only applied when the original request method is in
// OriginalMethods (defaults to POST).
//
// It returns ErrInvalidOverrideMethod if AllowedMethods or OriginalMethods
// contains an invalid method.
func MethodOverrideMiddleware(cfg MethodOverrideConfig) (mux.MiddlewareFunc, error) {
	headers := cfg.HeaderNames
	if len(headers) == 0 {
		headers = defaultOverrideHeaders
	}

	originals := cfg.OriginalMethods
	if originals == nil {
		originals = defaultOriginalMethods
	}

	methods := cfg.AllowedMethods
	if methods == nil {
		methods = defaultOverrideMethods
	}

	for _, m := range originals {
		if m == "" || m != strings.ToUpper(m) {
			return nil, ErrInvalidOverrideMethod
		}
	}

	for _, m := range methods {
		if m == "" || m != strings.ToUpper(m) {
			return nil, ErrInvalidOverrideMethod
		}
	}

	headerNames := make([]string, len(headers))
	copy(headerNames, headers)

	originalSet := make(map[string]struct{}, len(originals))
	for _, m := range originals {
		originalSet[m] = struct{}{}
	}

	allowed := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		allowed[m] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := originalSet[r.Method]; ok {
				for _, h := range headerNames {
					if v := r.Header.Get(h); v != "" {
						override := strings.ToUpper(v)
						if _, ok := allowed[override]; ok {
							r.Method = override
							r.Header.Del(h)
						}

						break
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}
