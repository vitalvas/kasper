package httpsig

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEd25519(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	t.Run("sign and verify round trip", func(t *testing.T) {
		signer, err := NewEd25519Signer("test-key", priv)
		require.NoError(t, err)

		verifier, err := NewEd25519Verifier("test-key", pub)
		require.NoError(t, err)

		message := []byte("test message")
		sig, err := signer.Sign(message)
		require.NoError(t, err)

		assert.NoError(t, verifier.Verify(message, sig))
		assert.Equal(t, AlgorithmEd25519, signer.Algorithm())
		assert.Equal(t, AlgorithmEd25519, verifier.Algorithm())
		assert.Equal(t, "test-key", signer.KeyID())
		assert.Equal(t, "test-key", verifier.KeyID())
	})

	t.Run("wrong message fails verification", func(t *testing.T) {
		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		verifier, err := NewEd25519Verifier("k", pub)
		require.NoError(t, err)

		sig, err := signer.Sign([]byte("original"))
		require.NoError(t, err)

		assert.ErrorIs(t, verifier.Verify([]byte("tampered"), sig), ErrSignatureInvalid)
	})

	t.Run("invalid private key size", func(t *testing.T) {
		_, err := NewEd25519Signer("k", ed25519.PrivateKey(make([]byte, 10)))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("invalid public key size", func(t *testing.T) {
		_, err := NewEd25519Verifier("k", ed25519.PublicKey(make([]byte, 10)))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
}

func TestECDSA(t *testing.T) {
	type ecdsaFactory struct {
		name       string
		curve      elliptic.Curve
		wrongCurve elliptic.Curve
		alg        Algorithm
		newSigner  func(string, *ecdsa.PrivateKey) (Signer, error)
		newVerif   func(string, *ecdsa.PublicKey) (Verifier, error)
	}

	factories := []ecdsaFactory{
		{
			name:       "P-256",
			curve:      elliptic.P256(),
			wrongCurve: elliptic.P384(),
			alg:        AlgorithmECDSAP256SHA256,
			newSigner:  NewECDSAP256Signer,
			newVerif:   NewECDSAP256Verifier,
		},
		{
			name:       "P-384",
			curve:      elliptic.P384(),
			wrongCurve: elliptic.P256(),
			alg:        AlgorithmECDSAP384SHA384,
			newSigner:  NewECDSAP384Signer,
			newVerif:   NewECDSAP384Verifier,
		},
	}

	for _, f := range factories {
		t.Run(f.name, func(t *testing.T) {
			key, err := ecdsa.GenerateKey(f.curve, rand.Reader)
			require.NoError(t, err)

			t.Run("sign and verify round trip", func(t *testing.T) {
				signer, err := f.newSigner("ec-key", key)
				require.NoError(t, err)

				verifier, err := f.newVerif("ec-key", &key.PublicKey)
				require.NoError(t, err)

				message := []byte("ecdsa test")
				sig, err := signer.Sign(message)
				require.NoError(t, err)

				assert.NoError(t, verifier.Verify(message, sig))
				assert.Equal(t, f.alg, signer.Algorithm())
				assert.Equal(t, f.alg, verifier.Algorithm())
				assert.Equal(t, "ec-key", signer.KeyID())
				assert.Equal(t, "ec-key", verifier.KeyID())
			})

			t.Run("wrong message fails verification", func(t *testing.T) {
				signer, err := f.newSigner("k", key)
				require.NoError(t, err)

				verifier, err := f.newVerif("k", &key.PublicKey)
				require.NoError(t, err)

				sig, err := signer.Sign([]byte("original"))
				require.NoError(t, err)

				assert.ErrorIs(t, verifier.Verify([]byte("tampered"), sig), ErrSignatureInvalid)
			})

			t.Run("wrong curve rejected", func(t *testing.T) {
				wrongKey, err := ecdsa.GenerateKey(f.wrongCurve, rand.Reader)
				require.NoError(t, err)

				_, err = f.newSigner("k", wrongKey)
				assert.ErrorIs(t, err, ErrInvalidKey)

				_, err = f.newVerif("k", &wrongKey.PublicKey)
				assert.ErrorIs(t, err, ErrInvalidKey)
			})

			t.Run("nil key rejected", func(t *testing.T) {
				_, err := f.newSigner("k", nil)
				assert.ErrorIs(t, err, ErrInvalidKey)

				_, err = f.newVerif("k", nil)
				assert.ErrorIs(t, err, ErrInvalidKey)
			})
		})
	}
}

func TestRSA(t *testing.T) {
	type rsaFactory struct {
		name      string
		alg       Algorithm
		newSigner func(string, *rsa.PrivateKey) (Signer, error)
		newVerif  func(string, *rsa.PublicKey) (Verifier, error)
	}

	factories := []rsaFactory{
		{
			name:      "RSA-PSS",
			alg:       AlgorithmRSAPSSSHA512,
			newSigner: NewRSAPSSSigner,
			newVerif:  NewRSAPSSVerifier,
		},
		{
			name:      "RSA-v1.5",
			alg:       AlgorithmRSAv15SHA256,
			newSigner: NewRSAv15Signer,
			newVerif:  NewRSAv15Verifier,
		},
	}

	for _, f := range factories {
		t.Run(f.name, func(t *testing.T) {
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			require.NoError(t, err)

			t.Run("sign and verify round trip", func(t *testing.T) {
				signer, err := f.newSigner("rsa-key", key)
				require.NoError(t, err)

				verifier, err := f.newVerif("rsa-key", &key.PublicKey)
				require.NoError(t, err)

				message := []byte("rsa test message")
				sig, err := signer.Sign(message)
				require.NoError(t, err)

				assert.NoError(t, verifier.Verify(message, sig))
				assert.Equal(t, f.alg, signer.Algorithm())
				assert.Equal(t, f.alg, verifier.Algorithm())
				assert.Equal(t, "rsa-key", signer.KeyID())
				assert.Equal(t, "rsa-key", verifier.KeyID())
			})

			t.Run("wrong message fails verification", func(t *testing.T) {
				signer, err := f.newSigner("k", key)
				require.NoError(t, err)

				verifier, err := f.newVerif("k", &key.PublicKey)
				require.NoError(t, err)

				sig, err := signer.Sign([]byte("original"))
				require.NoError(t, err)

				assert.ErrorIs(t, verifier.Verify([]byte("tampered"), sig), ErrSignatureInvalid)
			})

			t.Run("nil key rejected", func(t *testing.T) {
				_, err := f.newSigner("k", nil)
				assert.ErrorIs(t, err, ErrInvalidKey)

				_, err = f.newVerif("k", nil)
				assert.ErrorIs(t, err, ErrInvalidKey)
			})

			t.Run("small key rejected", func(t *testing.T) {
				smallKey, err := rsa.GenerateKey(rand.Reader, 1024)
				require.NoError(t, err)

				_, err = f.newSigner("k", smallKey)
				assert.ErrorIs(t, err, ErrInvalidKey)

				_, err = f.newVerif("k", &smallKey.PublicKey)
				assert.ErrorIs(t, err, ErrInvalidKey)
			})
		})
	}
}

func TestHMACSHA256(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	t.Run("sign and verify round trip", func(t *testing.T) {
		signer, err := NewHMACSHA256Signer("hmac-key", key)
		require.NoError(t, err)

		verifier, err := NewHMACSHA256Verifier("hmac-key", key)
		require.NoError(t, err)

		message := []byte("hmac test")
		sig, err := signer.Sign(message)
		require.NoError(t, err)

		assert.NoError(t, verifier.Verify(message, sig))
		assert.Equal(t, AlgorithmHMACSHA256, signer.Algorithm())
		assert.Equal(t, AlgorithmHMACSHA256, verifier.Algorithm())
		assert.Equal(t, "hmac-key", signer.KeyID())
		assert.Equal(t, "hmac-key", verifier.KeyID())
	})

	t.Run("wrong message fails verification", func(t *testing.T) {
		signer, err := NewHMACSHA256Signer("k", key)
		require.NoError(t, err)

		verifier, err := NewHMACSHA256Verifier("k", key)
		require.NoError(t, err)

		sig, err := signer.Sign([]byte("original"))
		require.NoError(t, err)

		assert.ErrorIs(t, verifier.Verify([]byte("tampered"), sig), ErrSignatureInvalid)
	})

	t.Run("wrong key fails verification", func(t *testing.T) {
		otherKey := make([]byte, 32)
		_, err := rand.Read(otherKey)
		require.NoError(t, err)

		signer, err := NewHMACSHA256Signer("k", key)
		require.NoError(t, err)

		verifier, err := NewHMACSHA256Verifier("k", otherKey)
		require.NoError(t, err)

		sig, err := signer.Sign([]byte("message"))
		require.NoError(t, err)

		assert.ErrorIs(t, verifier.Verify([]byte("message"), sig), ErrSignatureInvalid)
	})

	t.Run("short key rejected", func(t *testing.T) {
		_, err := NewHMACSHA256Signer("k", make([]byte, 16))
		assert.ErrorIs(t, err, ErrInvalidKey)

		_, err = NewHMACSHA256Verifier("k", make([]byte, 16))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("key is copied", func(t *testing.T) {
		keyCopy := make([]byte, 32)
		copy(keyCopy, key)

		signer, err := NewHMACSHA256Signer("k", keyCopy)
		require.NoError(t, err)

		verifier, err := NewHMACSHA256Verifier("k", key)
		require.NoError(t, err)

		// Mutate the original slice used for signer.
		keyCopy[0] ^= 0xff

		message := []byte("test key isolation")
		sig, err := signer.Sign(message)
		require.NoError(t, err)

		assert.NoError(t, verifier.Verify(message, sig))
	})
}
