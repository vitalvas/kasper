package muxhandlers

import (
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// RecoveryConfig configures the Recovery middleware behaviour.
type RecoveryConfig struct {
	// LogFunc is an optional callback invoked with the request and the
	// recovered value when a panic occurs. When nil, no logging is performed.
	LogFunc func(r *http.Request, err any)
}

// RecoveryMiddleware returns a middleware that recovers from panics in
// downstream handlers. When a panic occurs it returns 500 Internal Server
// Error to the client and optionally invokes LogFunc.
func RecoveryMiddleware(cfg RecoveryConfig) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					if cfg.LogFunc != nil {
						cfg.LogFunc(r, err)
					}

					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
