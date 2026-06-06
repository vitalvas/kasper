package e2ee

import (
	"bytes"
	"crypto/cipher"
	"crypto/ecdh"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReader is an io.Reader that always fails. Used to drive error branches
// that read randomness or request/response bodies.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("forced read error") }

// errReadCloser wraps errReader with a Close method for use as a body.
type errReadCloser struct{ errReader }

func (errReadCloser) Close() error { return nil }

// forcedErr is a sentinel error used to drive seam failure branches.
func forcedErr() error { return errors.New("forced crypto error") }

// withFailingGCM swaps newGCM for one that always errors, restoring it after.
func withFailingGCM(t *testing.T) {
	t.Helper()

	prev := newGCM
	newGCM = func([]byte) (cipher.AEAD, error) { return nil, errors.New("forced gcm error") }

	t.Cleanup(func() { newGCM = prev })
}

// deriveBetween derives keys for one peer using the standard salt convention.
func deriveBetween(priv *ecdh.PrivateKey, peer *ecdh.PublicKey, cpk, spk []byte, issuer string, aead AEAD, kid string) (ekReq, ekRes []byte, err error) {
	return deriveKeys(deriveParams{
		ourPriv:   priv,
		peerPub:   peer,
		cpk:       cpk,
		serverPub: spk,
		issuer:    issuer,
		aead:      aead,
		kid:       kid,
	})
}

func TestAEADKeyLength(t *testing.T) {
	tests := []struct {
		aead AEAD
		want int
		ok   bool
	}{
		{AEADAES128GCM, 16, true},
		{AEADAES192GCM, 24, true},
		{AEADAES256GCM, 32, true},
		{AEAD("bogus"), 0, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.aead), func(t *testing.T) {
			n, ok := tt.aead.keyLength()
			assert.Equal(t, tt.want, n)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.ok, tt.aead.valid())
		})
	}
}

func TestDeriveKeysSymmetry(t *testing.T) {
	client, err := generateKeyPair(nil)
	require.NoError(t, err)

	server, err := generateKeyPair(nil)
	require.NoError(t, err)

	cpk := client.PublicKey().Bytes()
	spk := server.PublicKey().Bytes()

	// Client derives using its private key against the server public key.
	cReq, cRes, err := deriveBetween(client, server.PublicKey(), cpk, spk, "https://api.example.com", AEADAES256GCM, "kid1")
	require.NoError(t, err)

	// Server derives using its private key against the client public key.
	sReq, sRes, err := deriveBetween(server, client.PublicKey(), cpk, spk, "https://api.example.com", AEADAES256GCM, "kid1")
	require.NoError(t, err)

	assert.Equal(t, cReq, sReq, "request keys must match")
	assert.Equal(t, cRes, sRes, "response keys must match")
	assert.NotEqual(t, cReq, cRes, "directional keys must differ")
	assert.Len(t, cReq, 32)
}

func TestDeriveKeysBindsContext(t *testing.T) {
	client, _ := generateKeyPair(nil)
	server, _ := generateKeyPair(nil)
	cpk := client.PublicKey().Bytes()
	spk := server.PublicKey().Bytes()

	base, _, err := deriveBetween(client, server.PublicKey(), cpk, spk, "https://a.example", AEADAES256GCM, "kid1")
	require.NoError(t, err)

	// Changing issuer, aead, or kid must change the derived key.
	diffIssuer, _, _ := deriveBetween(client, server.PublicKey(), cpk, spk, "https://b.example", AEADAES256GCM, "kid1")
	diffKID, _, _ := deriveBetween(client, server.PublicKey(), cpk, spk, "https://a.example", AEADAES256GCM, "kid2")

	assert.NotEqual(t, base, diffIssuer)
	assert.NotEqual(t, base, diffKID)
}

func TestDeriveKeysUnknownAEAD(t *testing.T) {
	client, _ := generateKeyPair(nil)
	server, _ := generateKeyPair(nil)

	_, _, err := deriveBetween(client, server.PublicKey(), nil, nil, "iss", AEAD("x"), "k")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAEADUnsupported))
}

func TestDeriveKeysLowOrderPointRejected(t *testing.T) {
	// A low-order (all-zero) public point makes the real X25519 ECDH reject
	// the exchange, exercising the ECDH error branch in deriveKeys.
	client, err := generateKeyPair(nil)
	require.NoError(t, err)

	lowOrder := make([]byte, keySize) // all zeros: low-order point

	pub, perr := publicKeyFromBytes(lowOrder)
	if perr != nil {
		t.Skipf("platform rejects zero public key at parse: %v", perr)
	}

	_, _, err = deriveBetween(client, pub, client.PublicKey().Bytes(), lowOrder, "iss", AEADAES256GCM, "k")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidKey)
}

func TestDeriveKeysECDHError(t *testing.T) {
	prev := ecdhShared
	ecdhShared = func(*ecdh.PrivateKey, *ecdh.PublicKey) ([]byte, error) { return nil, forcedErr() }
	t.Cleanup(func() { ecdhShared = prev })

	client, _ := generateKeyPair(nil)
	server, _ := generateKeyPair(nil)

	_, _, err := deriveBetween(client, server.PublicKey(), nil, nil, "iss", AEADAES256GCM, "k")
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestDeriveKeysZeroSecret(t *testing.T) {
	prev := ecdhShared
	ecdhShared = func(*ecdh.PrivateKey, *ecdh.PublicKey) ([]byte, error) { return make([]byte, 32), nil }
	t.Cleanup(func() { ecdhShared = prev })

	client, _ := generateKeyPair(nil)
	server, _ := generateKeyPair(nil)

	_, _, err := deriveBetween(client, server.PublicKey(), nil, nil, "iss", AEADAES256GCM, "k")
	require.ErrorIs(t, err, ErrInvalidKey)
	assert.Contains(t, err.Error(), "zero shared secret")
}

func TestDeriveKeysExtractError(t *testing.T) {
	prev := hkdfExtract
	hkdfExtract = func([]byte, []byte) ([]byte, error) { return nil, forcedErr() }
	t.Cleanup(func() { hkdfExtract = prev })

	client, _ := generateKeyPair(nil)
	server, _ := generateKeyPair(nil)

	_, _, err := deriveBetween(client, server.PublicKey(), nil, nil, "iss", AEADAES256GCM, "k")
	require.ErrorIs(t, err, ErrInvalidKey)
	assert.Contains(t, err.Error(), "extract")
}

func TestDeriveKeysExpandReqError(t *testing.T) {
	prev := hkdfExpand
	hkdfExpand = func([]byte, string, int) ([]byte, error) { return nil, forcedErr() }
	t.Cleanup(func() { hkdfExpand = prev })

	client, _ := generateKeyPair(nil)
	server, _ := generateKeyPair(nil)

	_, _, err := deriveBetween(client, server.PublicKey(), nil, nil, "iss", AEADAES256GCM, "k")
	require.ErrorIs(t, err, ErrInvalidKey)
	assert.Contains(t, err.Error(), "expand req")
}

func TestDeriveKeysExpandResError(t *testing.T) {
	// Fail only on the response (second) expand call.
	prev := hkdfExpand

	var calls int

	hkdfExpand = func(prk []byte, info string, n int) ([]byte, error) {
		calls++
		if calls == 1 {
			return prev(prk, info, n)
		}

		return nil, forcedErr()
	}
	t.Cleanup(func() { hkdfExpand = prev })

	client, _ := generateKeyPair(nil)
	server, _ := generateKeyPair(nil)

	cpk := client.PublicKey().Bytes()
	spk := server.PublicKey().Bytes()

	_, _, err := deriveBetween(client, server.PublicKey(), cpk, spk, "iss", AEADAES256GCM, "k")
	require.ErrorIs(t, err, ErrInvalidKey)
	assert.Contains(t, err.Error(), "expand res")
}

func TestSealOpenRoundTrip(t *testing.T) {
	ek := bytes.Repeat([]byte{0x42}, 32)
	plaintext := []byte("hello, world")
	aad := []byte("aad")

	body, err := seal(ek, plaintext, aad, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(body), minBodySize)

	got, err := open(ek, body, aad)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestOpenRejectsTamperedBody(t *testing.T) {
	ek := bytes.Repeat([]byte{0x42}, 32)

	body, err := seal(ek, []byte("data"), []byte("aad"), nil)
	require.NoError(t, err)

	body[len(body)-1] ^= 0xff

	_, err = open(ek, body, []byte("aad"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptFailed))
}

func TestOpenRejectsWrongAAD(t *testing.T) {
	ek := bytes.Repeat([]byte{0x42}, 32)

	body, err := seal(ek, []byte("data"), []byte("aad1"), nil)
	require.NoError(t, err)

	_, err = open(ek, body, []byte("aad2"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptFailed))
}

func TestOpenRejectsShortBody(t *testing.T) {
	ek := bytes.Repeat([]byte{0x42}, 16)

	_, err := open(ek, []byte("short"), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMalformed))
}

func TestSealNonceFailure(t *testing.T) {
	ek := bytes.Repeat([]byte{0x42}, 32)

	_, err := seal(ek, []byte("data"), nil, errReader{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDecryptFailed)
}

func TestSealGCMFailure(t *testing.T) {
	withFailingGCM(t)

	_, err := seal(make([]byte, 32), []byte("data"), nil, nil)
	require.Error(t, err)
}

func TestOpenGCMFailure(t *testing.T) {
	withFailingGCM(t)

	_, err := open(make([]byte, 32), make([]byte, minBodySize+1), nil)
	require.Error(t, err)
}

func TestNewGCMBadKeySize(t *testing.T) {
	_, err := newGCM([]byte{1, 2, 3})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidKey)
}

func TestNewGCMCipherConstructorError(t *testing.T) {
	prev := newCipherGCM
	newCipherGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, forcedErr() }
	t.Cleanup(func() { newCipherGCM = prev })

	_, err := newGCM(make([]byte, 32))
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestGenerateKeyPairRandFailure(t *testing.T) {
	_, err := generateKeyPair(errReader{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidKey)
}

func TestGenerateKeyPairConstructorError(t *testing.T) {
	prev := x25519GenerateKey
	x25519GenerateKey = func(io.Reader) (*ecdh.PrivateKey, error) { return nil, forcedErr() }
	t.Cleanup(func() { x25519GenerateKey = prev })

	_, err := generateKeyPair(nil)
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestPublicKeyFromBytesValidatesLength(t *testing.T) {
	_, err := publicKeyFromBytes([]byte{1, 2, 3})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidKey))
}

func TestPublicKeyFromBytesConstructorError(t *testing.T) {
	prev := x25519NewPublic
	x25519NewPublic = func([]byte) (*ecdh.PublicKey, error) { return nil, forcedErr() }
	t.Cleanup(func() { x25519NewPublic = prev })

	_, err := publicKeyFromBytes(make([]byte, keySize))
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestPrivateKeyFromBytes(t *testing.T) {
	priv, err := generateKeyPair(nil)
	require.NoError(t, err)

	parsed, err := privateKeyFromBytes(priv.Bytes())
	require.NoError(t, err)
	assert.Equal(t, priv.PublicKey().Bytes(), parsed.PublicKey().Bytes())

	_, err = privateKeyFromBytes([]byte{1, 2, 3})
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestPrivateKeyFromBytesConstructorError(t *testing.T) {
	prev := x25519NewPrivate
	x25519NewPrivate = func([]byte) (*ecdh.PrivateKey, error) { return nil, forcedErr() }
	t.Cleanup(func() { x25519NewPrivate = prev })

	_, err := privateKeyFromBytes(make([]byte, keySize))
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestFingerprint(t *testing.T) {
	fp := fingerprint([]byte("public-key"))
	assert.Len(t, fp, fingerprintSize)
}

func TestInfoString(t *testing.T) {
	got := infoString(dirRequest, "https://api.example.com", AEADAES256GCM, "kid1")
	assert.Equal(t, "e2ee/v1:req https://api.example.com AES-256-GCM kid1", got)
}

func TestAllZero(t *testing.T) {
	assert.True(t, allZero([]byte{0, 0, 0}))
	assert.False(t, allZero([]byte{0, 1, 0}))
}
