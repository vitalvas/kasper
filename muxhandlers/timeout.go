package muxhandlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// ErrInvalidTimeout is returned when TimeoutConfig.Duration is not greater
// than zero.
var ErrInvalidTimeout = errors.New("timeout: duration must be greater than zero")

// TimeoutConfig configures the Timeout middleware behaviour.
type TimeoutConfig struct {
	// Duration is the maximum time allowed for the handler to complete.
	// Must be greater than zero.
	Duration time.Duration

	// Message is the response body returned when the handler times out.
	// When empty, the standard library default is used.
	Message string
}

// TimeoutMiddleware returns a middleware that limits handler execution time.
// It wraps the handler with http.TimeoutHandler, which returns 503 Service
// Unavailable when the handler does not complete within the configured
// duration.
//
// It returns ErrInvalidTimeout if Duration is not greater than zero.
func TimeoutMiddleware(cfg TimeoutConfig) (mux.MiddlewareFunc, error) {
	if cfg.Duration <= 0 {
		return nil, ErrInvalidTimeout
	}

	duration := cfg.Duration
	message := cfg.Message

	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, duration, message)
	}, nil
}
