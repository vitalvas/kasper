package httpsig

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateNonce(t *testing.T) {
	t.Run("returns 22-char base64url string", func(t *testing.T) {
		nonce, err := GenerateNonce()
		require.NoError(t, err)
		assert.Len(t, nonce, 22)
	})

	t.Run("successive calls produce unique values", func(t *testing.T) {
		seen := make(map[string]bool)
		for range 100 {
			nonce, err := GenerateNonce()
			require.NoError(t, err)
			assert.False(t, seen[nonce], "duplicate nonce: %s", nonce)
			seen[nonce] = true
		}
	})
}

type errSigner struct {
	err error
}

func (s errSigner) Sign([]byte) ([]byte, error) { return nil, s.err }
func (s errSigner) Algorithm() Algorithm        { return AlgorithmEd25519 }
func (s errSigner) KeyID() string               { return "err-key" }

func TestSignRequest(t *testing.T) {
	t.Run("nil signer returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)

		err := SignRequest(req, SignConfig{})
		assert.ErrorIs(t, err, ErrNoSigner)
	})

	t.Run("ed25519 signing", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("ed-key", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		assert.NotEmpty(t, req.Header.Get("Signature"))
		assert.NotEmpty(t, req.Header.Get("Signature-Input"))
		assert.Contains(t, req.Header.Get("Signature-Input"), "sig1=")
		assert.Contains(t, req.Header.Get("Signature"), "sig1=")
	})

	t.Run("custom label", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{
			Signer: signer,
			Label:  "my-sig",
		})
		require.NoError(t, err)

		assert.Contains(t, req.Header.Get("Signature-Input"), "my-sig=")
		assert.Contains(t, req.Header.Get("Signature"), "my-sig=")
	})

	t.Run("custom covered components", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "https://example.com/path?q=1", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{
			Signer:            signer,
			CoveredComponents: []string{"@method", "@query"},
		})
		require.NoError(t, err)

		input := req.Header.Get("Signature-Input")
		assert.Contains(t, input, "\"@method\"")
		assert.Contains(t, input, "\"@query\"")
	})

	t.Run("with nonce and tag", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{
			Signer: signer,
			Nonce:  "abc123",
			Tag:    "my-app",
		})
		require.NoError(t, err)

		input := req.Header.Get("Signature-Input")
		assert.Contains(t, input, "nonce=\"abc123\"")
		assert.Contains(t, input, "tag=\"my-app\"")
	})

	t.Run("explicit created time", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{
			Signer:  signer,
			Created: time.Unix(1700000000, 0),
		})
		require.NoError(t, err)

		input := req.Header.Get("Signature-Input")
		assert.Contains(t, input, "created=1700000000")
	})

	t.Run("with expires", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{
			Signer:  signer,
			Expires: time.Unix(1700000300, 0),
		})
		require.NoError(t, err)

		input := req.Header.Get("Signature-Input")
		assert.Contains(t, input, "expires=1700000300")
	})

	t.Run("with content digest", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader("request body"))
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{
			Signer:          signer,
			DigestAlgorithm: DigestSHA256,
		})
		require.NoError(t, err)

		assert.NotEmpty(t, req.Header.Get("Content-Digest"))
		input := req.Header.Get("Signature-Input")
		assert.Contains(t, input, "\"content-digest\"")
	})

	t.Run("content-digest already in covered components is not duplicated", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader("body"))
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{
			Signer:            signer,
			CoveredComponents: []string{"@method", "content-digest"},
			DigestAlgorithm:   DigestSHA256,
		})
		require.NoError(t, err)

		input := req.Header.Get("Signature-Input")
		// Should appear exactly once.
		assert.Equal(t, 1, strings.Count(input, "\"content-digest\""))
	})

	t.Run("multiple signatures on same request", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{Signer: signer, Label: "sig1"})
		require.NoError(t, err)

		err = SignRequest(req, SignConfig{Signer: signer, Label: "sig2"})
		require.NoError(t, err)

		input := req.Header.Get("Signature-Input")
		assert.Contains(t, input, "sig1=")
		assert.Contains(t, input, "sig2=")

		sig := req.Header.Get("Signature")
		assert.Contains(t, sig, "sig1=")
		assert.Contains(t, sig, "sig2=")
	})

	t.Run("unknown component returns error", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{
			Signer:            signer,
			CoveredComponents: []string{"@method", "x-nonexistent"},
		})
		assert.ErrorIs(t, err, ErrUnknownComponent)
	})

	t.Run("signer error is propagated", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		sigErr := fmt.Errorf("sign failed")
		err := SignRequest(req, SignConfig{Signer: errSigner{err: sigErr}})
		assert.ErrorIs(t, err, sigErr)
	})

	t.Run("digest error is propagated", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer, err := NewEd25519Signer("k", priv)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader("body"))
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{
			Signer:          signer,
			DigestAlgorithm: DigestAlgorithm("unsupported"),
		})
		assert.ErrorIs(t, err, ErrUnsupportedDigest)
	})

	t.Run("all algorithms sign successfully", func(t *testing.T) {
		signers := createAllSigners(t)

		for _, s := range signers {
			t.Run(s.Algorithm().String(), func(t *testing.T) {
				req := httptest.NewRequest("GET", "https://example.com/api", nil)
				req.Host = "example.com"

				err := SignRequest(req, SignConfig{Signer: s})
				require.NoError(t, err)

				assert.NotEmpty(t, req.Header.Get("Signature"))
				assert.NotEmpty(t, req.Header.Get("Signature-Input"))
			})
		}
	})
}

// createAllSigners creates one signer per algorithm for testing.
func createAllSigners(t *testing.T) []Signer {
	t.Helper()

	signers := make([]Signer, 0, 6)

	// Ed25519
	_, edPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	edSigner, err := NewEd25519Signer("ed-key", edPriv)
	require.NoError(t, err)
	signers = append(signers, edSigner)

	// ECDSA P-256
	ecKey256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ec256Signer, err := NewECDSAP256Signer("ec256-key", ecKey256)
	require.NoError(t, err)
	signers = append(signers, ec256Signer)

	// ECDSA P-384
	ecKey384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	ec384Signer, err := NewECDSAP384Signer("ec384-key", ecKey384)
	require.NoError(t, err)
	signers = append(signers, ec384Signer)

	// RSA-PSS
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaPSSSigner, err := NewRSAPSSSigner("rsa-pss-key", rsaKey)
	require.NoError(t, err)
	signers = append(signers, rsaPSSSigner)

	// RSA v1.5
	rsaV15Signer, err := NewRSAv15Signer("rsa-v15-key", rsaKey)
	require.NoError(t, err)
	signers = append(signers, rsaV15Signer)

	// HMAC-SHA256
	hmacKey := make([]byte, 32)
	_, err = rand.Read(hmacKey)
	require.NoError(t, err)
	hmacSigner, err := NewHMACSHA256Signer("hmac-key", hmacKey)
	require.NoError(t, err)
	signers = append(signers, hmacSigner)

	return signers
}
