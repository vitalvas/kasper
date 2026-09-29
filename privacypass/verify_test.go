package privacypass

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/asn1"
	"encoding/hex"
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

func TestVerifyRejectsWrongTokenType(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()
	tok := issueToken(t, priv, challenge)
	tok.TokenType = 1
	require.ErrorIs(t, VerifyToken(&priv.PublicKey, challenge, tok, NewMemoryNonceCache(), time.Hour), ErrWrongTokenType)
}

func TestVerifyMalformedInputs(t *testing.T) {
	priv := testRSAKey(t)
	challenge := testChallenge()
	tok := issueToken(t, priv, challenge)
	cache := NewMemoryNonceCache()
	require.ErrorIs(t, VerifyToken(nil, challenge, tok, cache, time.Hour), ErrInvalidPublicKey)
	require.ErrorIs(t, VerifyToken(&priv.PublicKey, nil, tok, cache, time.Hour), ErrMalformed)
	require.ErrorIs(t, VerifyToken(&priv.PublicKey, challenge, nil, cache, time.Hour), ErrMalformed)
	_, err := MarshalPublicKey(&rsa.PublicKey{})
	require.ErrorIs(t, err, ErrInvalidPublicKey)
	tok.Nonce = tok.Nonce[:31]
	require.ErrorIs(t, VerifyToken(&priv.PublicKey, challenge, tok, cache, time.Hour), ErrMalformed)
}

func TestNoncesWithoutExpirationRemainSpent(t *testing.T) {
	cache := NewMemoryNonceCache()
	now := time.Now()
	cache.now = func() time.Time { return now }
	require.True(t, cache.StoreUnique([]byte("nonce"), 0))
	now = now.Add(24 * time.Hour)
	require.False(t, cache.StoreUnique([]byte("nonce"), 0))
}

// RFC 9578 Appendix A.2, test vector 1:
// https://www.rfc-editor.org/rfc/rfc9578.html#appendix-A.2
func TestRFC9578TokenVector(t *testing.T) {
	encodedKey, err := hex.DecodeString("30820152303d06092a864886f70d01010a3030a00d300b0609608648016503040202a11a301806092a864886f70d010108300b0609608648016503040202a2030201300382010f003082010a0282010100cb1aed6b6a95f5b1ce013a4cfcab25b94b2e64a23034e4250a7eab43c0df3a8c12993af12b111908d4b471bec31d4b6c9ad9cdda90612a2ee903523e6de5a224d6b02f09e5c374d0cfe01d8f529c500a78a2f67908fa682b5a2b430c81eaf1af72d7b5e794fc98a3139276879757ce453b526ef9bf6ceb99979b8423b90f4461a22af37aab0cf5733f7597abe44d31c732db68a181c6cbbe607d8c0e52e0655fd9996dc584eca0be87afbcd78a337d17b1dba9e828bbd81e291317144e7ff89f55619709b096cbb9ea474cead264c2073fe49740c01f00e109106066983d21e5f83f086e2e823c879cd43cef700d2a352a9babd612d03cad02db134b7e225a5f0203010001")
	require.NoError(t, err)
	encodedChallenge, err := hex.DecodeString("0002000e6973737565722e6578616d706c65208e7acc900e393381e8810b7c9e4a68b5163f1f880ab6688a6ffe780923609e88000e6f726967696e2e6578616d706c65")
	require.NoError(t, err)
	encodedToken, err := hex.DecodeString("0002aa72019d1f951df197021ce63876fe8b0a02dc1c31a12b0a2dd1508d07827f055969f643b4cfda5196d4aa86aeb5368834f4f06de46950ed435b3b81bd036d44ca572f8982a9ca248a3056186322d93ca147266121ddeb5632c07f1f71cd2708bc6a21b533d07294b5e900faf5537dd3eb33cee4e08c9670d1e5358fd184b0e00c637174f5206b14c7bb0e724ebf6b56271e5aa2ed94c051c4a433d302b23bc52460810d489fb050f9de5c868c6c1b06e3849fd087629f704cc724bc0d0984d5c339686fcdd75f9a9cdd25f37f855f6f4c584d84f716864f546b696d620c5bd41a811498de84ff9740ba3003ba2422d26b91eb745c084758974642a42078201543246ddb58030ea8e722376aa82484dca9610a8fb7e018e396165462e17a03e40ea7e128c090a911ecc708066cb201833010c1ebd4e910fc8e27a1be467f78671836a508257123a45e4e0ae2180a434bd1037713466347a8ebe46439d3da1970")
	require.NoError(t, err)

	var info subjectPublicKeyInfo
	_, err = asn1.Unmarshal(encodedKey, &info)
	require.NoError(t, err)
	var key rsaPublicKeyDER
	_, err = asn1.Unmarshal(info.SubjectPublicKey.Bytes, &key)
	require.NoError(t, err)
	pub := &rsa.PublicKey{N: key.N, E: key.E}
	encoded, err := MarshalPublicKey(pub)
	require.NoError(t, err)
	require.Equal(t, encodedKey, encoded)
	challenge, err := UnmarshalTokenChallenge(encodedChallenge)
	require.NoError(t, err)
	tok, err := UnmarshalToken(encodedToken)
	require.NoError(t, err)
	require.NoError(t, VerifyToken(pub, challenge, tok, NewMemoryNonceCache(), 0))
}
