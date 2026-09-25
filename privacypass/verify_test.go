package privacypass

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/blindrsa"
)

// issueToken runs the full RFC 9578 issuance flow using the real blindrsa
// primitives and returns a redeemable Token. This exercises client blinding,
// issuer blind-signing, client finalization, and token assembly.
func issueToken(t *testing.T, priv *rsa.PrivateKey, challenge *TokenChallenge) *Token {
	t.Helper()

	keyID, err := TokenKeyID(&priv.PublicKey)
	require.NoError(t, err)

	digest, err := challengeDigest(challenge)
	require.NoError(t, err)

	nonce := make([]byte, nonceSize)
	_, err = rand.Read(nonce)
	require.NoError(t, err)

	input := tokenInput(nonce, digest, keyID)

	// Client: prepare (identity for deterministic variant) and blind.
	prepared, err := blindrsa.Prepare(verifyVariant, rand.Reader, input)
	require.NoError(t, err)
	blinded, state, err := blindrsa.Blind(verifyVariant, &priv.PublicKey, rand.Reader, prepared)
	require.NoError(t, err)

	// Issuer: blind-sign.
	blindSig, err := blindrsa.BlindSign(verifyVariant, priv, blinded)
	require.NoError(t, err)

	// Client: finalize to recover the authenticator.
	authenticator, err := blindrsa.Finalize(verifyVariant, &priv.PublicKey, blindSig, state)
	require.NoError(t, err)

	return &Token{
		TokenType:       TokenType,
		Nonce:           nonce,
		ChallengeDigest: digest,
		TokenKeyID:      keyID,
		Authenticator:   authenticator,
	}
}

func testChallenge() *TokenChallenge {
	return &TokenChallenge{
		TokenType:  TokenType,
		IssuerName: "issuer.example",
		OriginInfo: "origin.example",
	}
}

func TestVerifyTokenEndToEnd(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()
	tok := issueToken(t, priv, challenge)

	cache := NewMemoryNonceCache()
	err := VerifyToken(&priv.PublicKey, challenge, tok, cache, time.Hour)
	require.NoError(t, err)
}

func TestVerifyTokenReplay(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()
	tok := issueToken(t, priv, challenge)
	cache := NewMemoryNonceCache()

	require.NoError(t, VerifyToken(&priv.PublicKey, challenge, tok, cache, time.Hour))
	// Second redemption of the same nonce is rejected.
	require.ErrorIs(t, VerifyToken(&priv.PublicKey, challenge, tok, cache, time.Hour), ErrReplay)
}

func TestVerifyTokenNilCacheFailsClosed(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()
	tok := issueToken(t, priv, challenge)
	require.ErrorIs(t, VerifyToken(&priv.PublicKey, challenge, tok, nil, time.Hour), ErrNoNonceCache)
}

func TestVerifyTokenKeyIDMismatch(t *testing.T) {
	priv := testRSAKey(t)
	other := testRSAKey(t)
	challenge := testChallenge()
	tok := issueToken(t, priv, challenge)

	err := VerifyToken(&other.PublicKey, challenge, tok, NewMemoryNonceCache(), time.Hour)
	require.ErrorIs(t, err, ErrKeyIDMismatch)
}

func TestVerifyTokenChallengeMismatch(t *testing.T) {
	priv := testRSAKey(t)
	tok := issueToken(t, priv, testChallenge())

	other := &TokenChallenge{TokenType: TokenType, IssuerName: "different.example"}
	err := VerifyToken(&priv.PublicKey, other, tok, NewMemoryNonceCache(), time.Hour)
	require.ErrorIs(t, err, ErrChallengeMismatch)
}

func TestVerifyTokenBadSignature(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()
	tok := issueToken(t, priv, challenge)
	// Corrupt the authenticator.
	tok.Authenticator[0] ^= 0xff

	err := VerifyToken(&priv.PublicKey, challenge, tok, NewMemoryNonceCache(), time.Hour)
	require.ErrorIs(t, err, ErrBadSignature)
}

func TestVerifyTokenMalformedChallenge(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()
	tok := issueToken(t, priv, challenge)
	// A challenge that cannot be marshalled surfaces the encode error, after
	// the key id check passes.
	bad := &TokenChallenge{TokenType: TokenType, RedemptionContext: []byte{1, 2, 3}}
	err := VerifyToken(&priv.PublicKey, bad, tok, NewMemoryNonceCache(), time.Hour)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestMemoryNonceCacheExpiry(t *testing.T) {
	cache := NewMemoryNonceCache()
	now := time.Unix(1000, 0)
	cache.now = func() time.Time { return now }

	nonce := []byte("n1")
	assert.True(t, cache.StoreUnique(nonce, time.Minute))
	assert.False(t, cache.StoreUnique(nonce, time.Minute))

	// After expiry the nonce may be stored again.
	now = now.Add(2 * time.Minute)
	assert.True(t, cache.StoreUnique(nonce, time.Minute))
}

func BenchmarkVerifyToken(b *testing.B) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}
	challenge := testChallenge()
	digest, _ := challengeDigest(challenge)
	keyID, _ := TokenKeyID(&priv.PublicKey)
	nonce := make([]byte, nonceSize)
	input := tokenInput(nonce, digest, keyID)
	prepared, _ := blindrsa.Prepare(verifyVariant, rand.Reader, input)
	blinded, state, _ := blindrsa.Blind(verifyVariant, &priv.PublicKey, rand.Reader, prepared)
	blindSig, _ := blindrsa.BlindSign(verifyVariant, priv, blinded)
	auth, _ := blindrsa.Finalize(verifyVariant, &priv.PublicKey, blindSig, state)
	tok := &Token{TokenType: TokenType, Nonce: nonce, ChallengeDigest: digest, TokenKeyID: keyID, Authenticator: auth}

	b.ReportAllocs()
	for b.Loop() {
		// Fresh cache each iteration so replay does not trip.
		_ = VerifyToken(&priv.PublicKey, challenge, tok, NewMemoryNonceCache(), time.Hour)
	}
}
