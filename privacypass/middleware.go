package privacypass

import (
	"crypto/rsa"
	"errors"
	"net/http"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// defaultTokenTTL is the nonce retention window used when Config.TokenTTL is
// zero.
const defaultTokenTTL = time.Hour

// ErrNoPublicKey is returned when a middleware is configured without an issuer
// public key.
var ErrNoPublicKey = errors.New("privacypass: issuer public key is required")

// Config configures the origin-side PrivateToken middleware.
type Config struct {
	// PublicKey is the issuer public key used to verify redeemed tokens.
	// Required.
	PublicKey *rsa.PublicKey

	// Challenge is the TokenChallenge advertised to clients that present no
	// valid token, and against which redeemed tokens are checked. Required.
	Challenge *TokenChallenge

	// Cache records redeemed nonces for single-use enforcement. When nil, a
	// process-local MemoryNonceCache is created.
	Cache NonceCache

	// TokenTTL is the nonce retention window. When zero, defaultTokenTTL is
	// used.
	TokenTTL time.Duration

	// OnError is invoked when a presented token fails verification. When nil,
	// the middleware responds with 401 and a fresh challenge.
	OnError func(w http.ResponseWriter, r *http.Request, err error)
}

// Middleware returns a middleware that requires a valid PrivateToken (RFC 9577)
// on each request. Requests without a token, or with an invalid token, receive
// 401 Unauthorized with a WWW-Authenticate challenge. Requests with a valid,
// not-yet-redeemed token proceed to the next handler.
//
// It returns ErrNoPublicKey or ErrMalformed when the configuration is
// incomplete.
func Middleware(cfg Config) (mux.MiddlewareFunc, error) {
	if cfg.PublicKey == nil {
		return nil, ErrNoPublicKey
	}
	if cfg.Challenge == nil {
		return nil, ErrMalformed
	}
	// Validate the challenge encodes cleanly up front.
	if _, err := cfg.Challenge.Marshal(); err != nil {
		return nil, err
	}

	cache := cfg.Cache
	if cache == nil {
		cache = NewMemoryNonceCache()
	}
	ttl := cfg.TokenTTL
	if ttl <= 0 {
		ttl = defaultTokenTTL
	}

	challengeHeader, err := BuildChallengeHeader(cfg.Challenge, cfg.PublicKey)
	if err != nil {
		return nil, err
	}

	onError := cfg.OnError
	if onError == nil {
		onError = func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.Header().Set("WWW-Authenticate", challengeHeader)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok, err := ParseAuthorizationHeader(r.Header.Get("Authorization"))
			if err != nil {
				onError(w, r, err)
				return
			}
			if err := VerifyToken(cfg.PublicKey, cfg.Challenge, tok, cache, ttl); err != nil {
				onError(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}
