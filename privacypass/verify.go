package privacypass

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"sync"
	"time"

	"github.com/vitalvas/kasper/blindrsa"
)

// verifyVariant is the RSABSSA variant used by token type 0x0002.
const verifyVariant = blindrsa.VariantSHA384PSSDeterministic

// Verification errors.
var (
	// ErrKeyIDMismatch is returned when a token's token_key_id does not match
	// the verifying public key.
	ErrKeyIDMismatch = errors.New("privacypass: token key id mismatch")
	// ErrChallengeMismatch is returned when a token's challenge_digest does not
	// match the expected challenge.
	ErrChallengeMismatch = errors.New("privacypass: challenge digest mismatch")
	// ErrBadSignature is returned when the token authenticator fails RSA-PSS
	// verification against the issuer public key.
	ErrBadSignature = errors.New("privacypass: invalid token signature")
	// ErrReplay is returned when a token nonce has already been redeemed.
	ErrReplay = errors.New("privacypass: token already redeemed")
	// ErrNoNonceCache is returned when verification is requested without a
	// nonce cache. Redemption fails closed rather than allowing double-spend.
	ErrNoNonceCache = errors.New("privacypass: no nonce cache configured")
)

// NonceCache records redeemed token nonces to enforce single use. It must be
// safe for concurrent use. StoreUnique atomically records nonce and returns
// true when it was newly inserted (not previously redeemed) and false when it
// was already present. Entries may be retained for the token's lifetime,
// expressed via ttl.
type NonceCache interface {
	StoreUnique(nonce []byte, ttl time.Duration) bool
}

// VerifyToken verifies a redeemed token against the issuer public key and the
// expected challenge, and records the nonce for single-use enforcement.
//
// It checks, in order: the token_key_id matches pub; the challenge_digest
// matches challenge; the authenticator is a valid RSA-PSS signature over
// token_input; and the nonce has not been redeemed before. A nil cache fails
// closed with ErrNoNonceCache.
func VerifyToken(pub *rsa.PublicKey, challenge *TokenChallenge, tok *Token, cache NonceCache, ttl time.Duration) error {
	if cache == nil {
		return ErrNoNonceCache
	}

	keyID, err := TokenKeyID(pub)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(keyID, tok.TokenKeyID) != 1 {
		return ErrKeyIDMismatch
	}

	digest, err := challengeDigest(challenge)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(digest, tok.ChallengeDigest) != 1 {
		return ErrChallengeMismatch
	}

	input := tokenInput(tok.Nonce, tok.ChallengeDigest, tok.TokenKeyID)
	if err := blindrsa.Verify(verifyVariant, pub, input, tok.Authenticator); err != nil {
		return ErrBadSignature
	}

	// Insert the nonce only after signature verification succeeds, so an
	// unauthenticated token cannot poison the cache.
	if !cache.StoreUnique(tok.Nonce, ttl) {
		return ErrReplay
	}
	return nil
}

// challengeDigest returns SHA-256 of the encoded TokenChallenge.
func challengeDigest(c *TokenChallenge) ([]byte, error) {
	enc, err := c.Marshal()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(enc)
	return sum[:], nil
}

// MemoryNonceCache is an in-memory NonceCache with time-based eviction, safe
// for concurrent use. Expired entries are pruned lazily on access.
type MemoryNonceCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
	now     func() time.Time
}

// NewMemoryNonceCache creates an empty in-memory nonce cache.
func NewMemoryNonceCache() *MemoryNonceCache {
	return &MemoryNonceCache{
		entries: make(map[string]time.Time),
		now:     time.Now,
	}
}

// StoreUnique records nonce with the given ttl and reports whether it was newly
// inserted.
func (c *MemoryNonceCache) StoreUnique(nonce []byte, ttl time.Duration) bool {
	key := string(nonce)
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for k, exp := range c.entries {
		if now.After(exp) {
			delete(c.entries, k)
		}
	}

	if exp, ok := c.entries[key]; ok && now.Before(exp) {
		return false
	}
	c.entries[key] = now.Add(ttl)
	return true
}
