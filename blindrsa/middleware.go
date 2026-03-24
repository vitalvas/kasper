package blindrsa

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

const defaultHeaderName = "Blind-Signature"

// MiddlewareConfig configures the blind signature verification middleware.
type MiddlewareConfig struct {
	// Key is the issuer's RSA public key used for verification.
	Key *rsa.PublicKey

	// Variant selects the RSABSSA variant.
	Variant Variant

	// HeaderName is the HTTP header containing the base64-encoded signature.
	// Defaults to "Blind-Signature".
	HeaderName string

	// MessageFunc extracts the message to verify from the request. The
	// returned bytes must be the prepared message (including the random
	// prefix for randomized variants).
	MessageFunc func(r *http.Request) ([]byte, error)

	// OnError is called when verification fails. When nil, a plain 401
	// Unauthorized response is sent.
	OnError func(w http.ResponseWriter, r *http.Request, err error)
}

// VerifyMiddleware returns a [mux.MiddlewareFunc] that verifies blind
// signatures on incoming requests.
//
// It returns [ErrNoVerifier] if Key is nil and [ErrInvalidInput] if
// MessageFunc is nil.
func VerifyMiddleware(cfg MiddlewareConfig) (mux.MiddlewareFunc, error) {
	if cfg.Key == nil {
		return nil, ErrNoVerifier
	}

	if err := validatePublicKey(cfg.Key); err != nil {
		return nil, err
	}

	if _, err := validateVariant(cfg.Variant); err != nil {
		return nil, err
	}

	if cfg.MessageFunc == nil {
		return nil, fmt.Errorf("%w: message func must not be nil", ErrInvalidInput)
	}

	pub := cfg.Key
	variant := cfg.Variant

	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = defaultHeaderName
	}

	msgFunc := cfg.MessageFunc

	onError := cfg.OnError
	if onError == nil {
		onError = defaultMiddlewareOnError
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sigHeader := r.Header.Get(headerName)
			if sigHeader == "" {
				onError(w, r, fmt.Errorf("%w: missing %s header", ErrVerifyFailed, headerName))
				return
			}

			sig, err := base64.StdEncoding.DecodeString(sigHeader)
			if err != nil {
				onError(w, r, fmt.Errorf("%w: invalid base64 in %s header", ErrVerifyFailed, headerName))
				return
			}

			msg, err := msgFunc(r)
			if err != nil {
				onError(w, r, fmt.Errorf("%w: %s", ErrVerifyFailed, err))
				return
			}

			if err := Verify(variant, pub, msg, sig); err != nil {
				onError(w, r, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// defaultMiddlewareOnError writes a 401 Unauthorized response with no body.
func defaultMiddlewareOnError(w http.ResponseWriter, _ *http.Request, _ error) {
	w.WriteHeader(http.StatusUnauthorized)
}
