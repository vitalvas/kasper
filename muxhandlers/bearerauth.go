package muxhandlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// ErrNoTokenValidator is returned when BearerAuthConfig has no ValidateFunc configured.
var ErrNoTokenValidator = errors.New("bearer auth: ValidateFunc must be set")

// BearerAuthConfig configures the Bearer Auth middleware behaviour.
//
// Spec reference: https://www.rfc-editor.org/rfc/rfc6750
type BearerAuthConfig struct {
	// Realm is the authentication realm sent in the WWW-Authenticate header.
	// Defaults to "Restricted" when empty.
	Realm string

	// ValidateFunc is called to validate the bearer token.
	// It receives the request and the raw token string.
	// Return true to allow the request, false to reject it.
	ValidateFunc func(r *http.Request, token string) bool
}

// BearerAuthMiddleware returns a middleware that implements HTTP Bearer Token
// Authentication per RFC 6750. It extracts the token from the Authorization
// header and validates it using the configured ValidateFunc.
//
// When the Authorization header is missing, malformed, or the token is invalid,
// the middleware responds with 401 Unauthorized and a WWW-Authenticate: Bearer
// header per RFC 6750 Section 3.
//
// It returns ErrNoTokenValidator if ValidateFunc is nil.
func BearerAuthMiddleware(cfg BearerAuthConfig) (mux.MiddlewareFunc, error) {
	if cfg.ValidateFunc == nil {
		return nil, ErrNoTokenValidator
	}

	realm := cfg.Realm
	if realm == "" {
		realm = "Restricted"
	}

	validate := cfg.ValidateFunc

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearerToken(r)
			if !ok {
				bearerUnauthorized(w, realm)
				return
			}

			if !validate(r, token) {
				bearerUnauthorized(w, realm)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// extractBearerToken extracts the bearer token from the Authorization header.
// Returns the token and true if the header is present and well-formed,
// or an empty string and false otherwise.
func extractBearerToken(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if len(auth) <= len("Bearer ") {
		return "", false
	}

	if !strings.EqualFold(auth[:len("Bearer ")], "Bearer ") {
		return "", false
	}

	token := auth[len("Bearer "):]
	if token == "" {
		return "", false
	}

	return token, true
}

// bearerUnauthorized writes a 401 response with the WWW-Authenticate: Bearer
// header per RFC 6750 Section 3 and an empty body.
func bearerUnauthorized(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="`+realm+`"`)
	w.WriteHeader(http.StatusUnauthorized)
}
