package muxhandlers

import (
	"errors"
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// ErrInvalidMaxSize is returned when RequestSizeLimitConfig.MaxBytes is not
// greater than zero.
var ErrInvalidMaxSize = errors.New("request size limit: max size must be greater than zero")

// RequestSizeLimitConfig configures the Request Size Limit middleware behaviour.
type RequestSizeLimitConfig struct {
	// MaxBytes is the maximum allowed request body size in bytes.
	// Must be greater than zero.
	MaxBytes int64
}

// RequestSizeLimitMiddleware returns a middleware that limits the size of
// incoming request bodies. It wraps r.Body with http.MaxBytesReader so that
// downstream handlers receive an error when reading beyond the limit. The
// standard http.MaxBytesReader returns 413 Request Entity Too Large
// automatically when the limit is exceeded.
//
// It returns ErrInvalidMaxSize if MaxBytes is not greater than zero.
func RequestSizeLimitMiddleware(cfg RequestSizeLimitConfig) (mux.MiddlewareFunc, error) {
	if cfg.MaxBytes <= 0 {
		return nil, ErrInvalidMaxSize
	}

	maxBytes := cfg.MaxBytes

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}, nil
}
