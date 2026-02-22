package httpsig

import (
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// MiddlewareConfig configures the server-side signature verification
// middleware.
type MiddlewareConfig struct {
	// Verify configures how signatures are verified.
	Verify VerifyConfig

	// OnError is called when verification fails. When nil, a plain 401
	// Unauthorized response is sent.
	OnError func(w http.ResponseWriter, r *http.Request, err error)
}

// Middleware returns a mux.MiddlewareFunc that verifies HTTP message
// signatures on incoming requests per RFC 9421.
//
// It returns ErrNoResolver if VerifyConfig.Resolver is nil.
func Middleware(cfg MiddlewareConfig) (mux.MiddlewareFunc, error) {
	if cfg.Verify.Resolver == nil {
		return nil, ErrNoResolver
	}

	onError := cfg.OnError
	if onError == nil {
		onError = defaultOnError
	}

	verifyCfg := cfg.Verify

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := VerifyRequest(r, verifyCfg); err != nil {
				onError(w, r, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// defaultOnError writes a 401 Unauthorized response with no body.
func defaultOnError(w http.ResponseWriter, _ *http.Request, _ error) {
	w.WriteHeader(http.StatusUnauthorized)
}
