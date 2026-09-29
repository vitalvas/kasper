package privacypass

import (
	"crypto/rsa"
	"errors"
	"net/http"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// ErrNoPublicKey is returned when a middleware is configured without an issuer
// public key.
var ErrNoPublicKey = errors.New("privacypass: issuer public key is required")

// ErrChallengeExpired means the configured challenge is no longer redeemable.
var ErrChallengeExpired = errors.New("privacypass: challenge expired")

// ErrInvalidTokenTTL means the nonce retention window could expire while the
// configured challenge still accepts tokens.
var ErrInvalidTokenTTL = errors.New("privacypass: token TTL must cover the challenge lifetime")

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

	// TokenTTL is the nonce retention window. Zero retains nonces indefinitely.
	// A positive value requires ChallengeExpires to be set and must cover its
	// remaining lifetime. Negative values are invalid.
	TokenTTL time.Duration

	// ChallengeExpires is the last time at which the configured challenge is
	// accepted. Zero means it does not expire. Replace the middleware with a
	// new challenge before this deadline to continue accepting tokens.
	ChallengeExpires time.Time

	// OnError is invoked when a presented token fails verification. When nil,
	// the middleware responds with 401 and the configured challenge, unless
	// that challenge has expired.
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
	if cfg.Challenge.TokenType != TokenType {
		return nil, ErrWrongTokenType
	}
	if cfg.TokenTTL < 0 || (cfg.TokenTTL > 0 && (cfg.ChallengeExpires.IsZero() || time.Until(cfg.ChallengeExpires) > cfg.TokenTTL)) {
		return nil, ErrInvalidTokenTTL
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

	challengeHeader, err := BuildChallengeHeader(cfg.Challenge, cfg.PublicKey)
	if err != nil {
		return nil, err
	}

	onError := cfg.OnError
	if onError == nil {
		onError = func(w http.ResponseWriter, _ *http.Request, err error) {
			if !errors.Is(err, ErrChallengeExpired) {
				w.Header().Set("WWW-Authenticate", challengeHeader)
			}
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.ChallengeExpires.IsZero() && !time.Now().Before(cfg.ChallengeExpires) {
				onError(w, r, ErrChallengeExpired)
				return
			}
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
