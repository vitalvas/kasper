package privacypass

import (
	"container/heap"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"reflect"
	"sync"
	"time"

	"go.vitalvas.com/kasper/blindrsa"
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
// was already present. A zero ttl requires indefinite retention; a positive ttl
// requires retention for at least that duration.
type NonceCache interface {
	StoreUnique(nonce []byte, ttl time.Duration) bool
}

// VerifyToken verifies a redeemed token against the issuer public key and the
// expected challenge, and records the nonce for single-use enforcement.
//
// It checks, in order: the token_key_id matches pub; the challenge_digest
// matches challenge; the authenticator is a valid RSA-PSS signature over
// token_input; and the nonce has not been redeemed before. A nil cache fails
// closed with ErrNoNonceCache. A zero ttl retains nonces indefinitely. With a
// positive ttl, the caller must stop accepting this challenge before any cache
// entry can expire: the token itself contains no expiration timestamp.
func VerifyToken(pub *rsa.PublicKey, challenge *TokenChallenge, tok *Token, cache NonceCache, ttl time.Duration) error {
	if isNilNonceCache(cache) {
		return ErrNoNonceCache
	}

	if ttl < 0 {
		return ErrInvalidTokenTTL
	}
	if tok == nil || challenge == nil {
		return ErrMalformed
	}
	if tok.TokenType != TokenType || challenge.TokenType != TokenType {
		return ErrWrongTokenType
	}
	if _, err := tok.Marshal(); err != nil {
		return err
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
	mu       sync.Mutex
	entries  map[string]time.Time
	expiries nonceExpiryHeap
	now      func() time.Time
}

type nonceExpiry struct {
	key string
	at  time.Time
}

type nonceExpiryHeap []nonceExpiry

func (h nonceExpiryHeap) Len() int           { return len(h) }
func (h nonceExpiryHeap) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h nonceExpiryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *nonceExpiryHeap) Push(value any)    { *h = append(*h, value.(nonceExpiry)) }
func (h *nonceExpiryHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	old[last] = nonceExpiry{}
	*h = old[:last]
	return value
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
	if c == nil {
		return false
	}

	key := string(nonce)
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		c.entries = make(map[string]time.Time)
	}
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	for c.expiries.Len() > 0 && !now.Before(c.expiries[0].at) {
		expired := heap.Pop(&c.expiries).(nonceExpiry)
		if current, ok := c.entries[expired.key]; ok && current.Equal(expired.at) {
			delete(c.entries, expired.key)
		}
	}

	if exp, ok := c.entries[key]; ok && (exp.IsZero() || now.Before(exp)) {
		return false
	}
	var expires time.Time
	if ttl > 0 {
		expires = now.Add(ttl)
		heap.Push(&c.expiries, nonceExpiry{key: key, at: expires})
	}
	c.entries[key] = expires
	return true
}

func isNilNonceCache(cache NonceCache) bool {
	if cache == nil {
		return true
	}
	value := reflect.ValueOf(cache)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
