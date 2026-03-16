package muxhandlers

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/vitalvas/kasper/mux"
)

// ErrCanonicalHostEmpty is returned when CanonicalHostConfig.URL is empty.
var ErrCanonicalHostEmpty = errors.New("canonical host: URL must not be empty")

// ErrCanonicalHostInvalid is returned when CanonicalHostConfig.URL cannot be parsed.
var ErrCanonicalHostInvalid = errors.New("canonical host: URL is not valid")

// CanonicalHostConfig configures the Canonical Host middleware.
type CanonicalHostConfig struct {
	// URL is the canonical base URL to redirect to, including scheme
	// and host (e.g. "https://www.example.com"). Required.
	URL string

	// StatusCode is the HTTP redirect status code. Defaults to
	// 301 Moved Permanently.
	StatusCode int
}

// CanonicalHostMiddleware returns a middleware that redirects requests to
// the canonical host when the incoming request scheme or host does not
// match. The request path and query string are preserved.
//
// This is useful for enforcing a single canonical URL (e.g. redirecting
// example.com to www.example.com, or HTTP to HTTPS).
func CanonicalHostMiddleware(cfg CanonicalHostConfig) (mux.MiddlewareFunc, error) {
	if cfg.URL == "" {
		return nil, ErrCanonicalHostEmpty
	}

	parsed, err := url.Parse(cfg.URL)
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return nil, ErrCanonicalHostInvalid
	}

	canonicalScheme := parsed.Scheme
	canonicalHost := parsed.Host

	statusCode := cfg.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusMovedPermanently
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme := r.URL.Scheme
			if scheme == "" {
				if r.TLS != nil {
					scheme = "https"
				} else {
					scheme = "http"
				}
			}

			if scheme != canonicalScheme || r.Host != canonicalHost {
				u := *r.URL
				u.Scheme = canonicalScheme
				u.Host = canonicalHost
				http.Redirect(w, r, u.String(), statusCode)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}
