package muxhandlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// ErrNoAuthSource is returned when BasicAuthConfig has neither ValidateFunc
// nor Credentials configured.
var ErrNoAuthSource = errors.New("basic auth: at least one of ValidateFunc or Credentials must be set")

// BasicAuthConfig configures the Basic Auth middleware behaviour.
//
// Spec reference: https://www.rfc-editor.org/rfc/rfc7617
type BasicAuthConfig struct {
	// Realm is the authentication realm sent in the WWW-Authenticate header.
	// Defaults to "Restricted" when empty.
	Realm string

	// ValidateFunc is called to validate credentials dynamically.
	// Takes priority over Credentials when both are set.
	ValidateFunc func(username, password string) bool

	// Credentials is a static map of username -> password pairs.
	// Compared using SHA-256 hashed constant-time comparison to prevent
	// timing attacks, including length-based leaks.
	Credentials map[string]string
}

// BasicAuthMiddleware returns a middleware that implements HTTP Basic
// Authentication per RFC 7617. It validates the Authorization header and
// responds with 401 Unauthorized when credentials are missing or invalid.
//
// It returns ErrNoAuthSource if both ValidateFunc and Credentials are nil/empty.
func BasicAuthMiddleware(cfg BasicAuthConfig) (mux.MiddlewareFunc, error) {
	if cfg.ValidateFunc == nil && len(cfg.Credentials) == 0 {
		return nil, ErrNoAuthSource
	}

	realm := cfg.Realm
	if realm == "" {
		realm = "Restricted"
	}

	wwwAuthenticate := fmt.Sprintf("Basic realm=%q", realm)

	validate := cfg.ValidateFunc
	credentials := cfg.Credentials

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok {
				unauthorized(w, wwwAuthenticate)
				return
			}

			if validate != nil {
				if !validate(username, password) {
					unauthorized(w, wwwAuthenticate)
					return
				}
			} else {
				expectedPassword, exists := credentials[username]
				// Always perform the password comparison to prevent timing
				// leaks that reveal whether a username exists in the map.
				passwordMatch := constantTimeEqual(password, expectedPassword)
				if !exists || !passwordMatch {
					unauthorized(w, wwwAuthenticate)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// constantTimeEqual compares two strings in constant time by first hashing
// them with SHA-256. This prevents both value leaks and length-based timing
// leaks that raw ConstantTimeCompare would allow on different-length inputs.
func constantTimeEqual(a, b string) bool {
	aHash := sha256.Sum256([]byte(a))
	bHash := sha256.Sum256([]byte(b))

	return subtle.ConstantTimeCompare(aHash[:], bHash[:]) == 1
}

// unauthorized writes a 401 response with the WWW-Authenticate header and
// an empty body.
func unauthorized(w http.ResponseWriter, wwwAuthenticate string) {
	w.Header().Set("WWW-Authenticate", wwwAuthenticate)
	w.WriteHeader(http.StatusUnauthorized)
}
