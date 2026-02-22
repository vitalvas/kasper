package httpsig

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyRequest(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := NewEd25519Signer("test-key", priv)
	require.NoError(t, err)

	verifier, err := NewEd25519Verifier("test-key", pub)
	require.NoError(t, err)

	resolver := func(_ *http.Request, keyID string, alg Algorithm) (Verifier, error) {
		if keyID == "test-key" && alg == AlgorithmEd25519 {
			return verifier, nil
		}
		return nil, ErrInvalidKey
	}

	t.Run("nil resolver returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		err := VerifyRequest(req, VerifyConfig{})
		assert.ErrorIs(t, err, ErrNoResolver)
	})

	t.Run("missing signature headers returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		err := VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("sign and verify round trip", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.NoError(t, err)
	})

	t.Run("tampered header fails verification", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		// Tamper with the request after signing.
		req.Host = "attacker.com"

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureInvalid)
	})

	t.Run("tampered method fails verification", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		req.Method = "DELETE"

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureInvalid)
	})

	t.Run("tampered path fails verification", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api/items", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		req.URL.Path = "/api/admin"

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureInvalid)
	})

	t.Run("custom label verification", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer: signer,
			Label:  "my-sig",
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			Label:    "my-sig",
		})
		assert.NoError(t, err)
	})

	t.Run("wrong label not found", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer: signer,
			Label:  "real-sig",
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			Label:    "wrong-sig",
		})
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("required components present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/path", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:            signer,
			CoveredComponents: []string{"@method", "@authority", "@path"},
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver:           resolver,
			RequiredComponents: []string{"@method", "@path"},
		})
		assert.NoError(t, err)
	})

	t.Run("required component missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/path", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:            signer,
			CoveredComponents: []string{"@method"},
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver:           resolver,
			RequiredComponents: []string{"@authority"},
		})
		assert.ErrorIs(t, err, ErrMissingComponent)
	})

	t.Run("expired signature is rejected", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:  signer,
			Expires: time.Now().Add(-1 * time.Minute),
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureExpired)
	})

	t.Run("non-expired signature is accepted", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:  signer,
			Expires: time.Now().Add(5 * time.Minute),
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.NoError(t, err)
	})

	t.Run("signature age within max age", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			MaxAge:   1 * time.Minute,
		})
		assert.NoError(t, err)
	})

	t.Run("max age with missing created returns ErrCreatedRequired", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		// Manually craft Signature-Input without created to test the
		// ErrCreatedRequired path (SignRequest always sets created now).
		req.Header.Set("Signature-Input", `sig1=("@method" "@authority" "@path");alg="ed25519";keyid="test-key"`)
		req.Header.Set("Signature", "sig1=:dGVzdA==:")

		err := VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			MaxAge:   1 * time.Minute,
		})
		assert.ErrorIs(t, err, ErrCreatedRequired)
	})

	t.Run("max age with future created returns ErrSignatureExpired", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:  signer,
			Created: time.Now().Add(1 * time.Hour),
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			MaxAge:   1 * time.Minute,
		})
		assert.ErrorIs(t, err, ErrSignatureExpired)
	})

	t.Run("signature age exceeds max age", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:  signer,
			Created: time.Now().Add(-2 * time.Hour),
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver: resolver,
			MaxAge:   1 * time.Hour,
		})
		assert.ErrorIs(t, err, ErrSignatureExpired)
	})

	t.Run("with content digest verification", func(t *testing.T) {
		body := "request body content"
		req := httptest.NewRequest("POST", "https://example.com/api", strings.NewReader(body))
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:          signer,
			DigestAlgorithm: DigestSHA256,
		})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver:      resolver,
			RequireDigest: true,
		})
		assert.NoError(t, err)
	})

	t.Run("tampered body with digest fails", func(t *testing.T) {
		body := "original body"
		req := httptest.NewRequest("POST", "https://example.com/api", strings.NewReader(body))
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{
			Signer:          signer,
			DigestAlgorithm: DigestSHA256,
		})
		require.NoError(t, err)

		// Replace body after signing.
		req.Body = io.NopCloser(strings.NewReader("tampered body"))

		err = VerifyRequest(req, VerifyConfig{
			Resolver:      resolver,
			RequireDigest: true,
		})
		assert.ErrorIs(t, err, ErrDigestMismatch)
	})

	t.Run("require digest but no digest header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/api", strings.NewReader("body"))
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{
			Resolver:      resolver,
			RequireDigest: true,
		})
		assert.ErrorIs(t, err, ErrDigestNotFound)
	})

	t.Run("unknown key ID", func(t *testing.T) {
		_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		otherSigner, err := NewEd25519Signer("unknown-key", otherPriv)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{Signer: otherSigner})
		require.NoError(t, err)

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("signature-input present but signature header missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		// Remove only the Signature header.
		req.Header.Del("Signature")

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("unknown component in signature fails verification", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		// Manually set headers with a signature that references an unknown component.
		req.Header.Set("Signature-Input", `sig1=("@method" "x-nonexistent");alg="ed25519";keyid="test-key"`)
		req.Header.Set("Signature", "sig1=:dGVzdA==:")

		err := VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrUnknownComponent)
	})

	t.Run("malformed signature value in Signature header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Host = "example.com"

		err := SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		// Replace Signature with a malformed value.
		req.Header.Set("Signature", "sig1=notcolonwrapped")

		err = VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("malformed signature-input header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Header.Set("Signature-Input", "sig1=noparen")
		req.Header.Set("Signature", "sig1=:dGVzdA==:")

		err := VerifyRequest(req, VerifyConfig{Resolver: resolver})
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})
}

func TestFindSignatureInput(t *testing.T) {
	t.Run("first entry when label empty", func(t *testing.T) {
		header := `sig1=("@method" "@authority");created=123;alg="ed25519";keyid="k"`
		label, value, err := findSignatureInput(header, "")
		require.NoError(t, err)
		assert.Equal(t, "sig1", label)
		assert.Contains(t, value, "@method")
	})

	t.Run("specific label", func(t *testing.T) {
		header := `sig1=("@method");alg="ed25519";keyid="k1", sig2=("@path");alg="ed25519";keyid="k2"`
		label, value, err := findSignatureInput(header, "sig2")
		require.NoError(t, err)
		assert.Equal(t, "sig2", label)
		assert.Contains(t, value, "@path")
	})

	t.Run("comma-only separator", func(t *testing.T) {
		header := `sig1=("@method");alg="ed25519";keyid="k1",sig2=("@path");alg="ed25519";keyid="k2"`
		label, value, err := findSignatureInput(header, "sig2")
		require.NoError(t, err)
		assert.Equal(t, "sig2", label)
		assert.Contains(t, value, "@path")
	})

	t.Run("label not found", func(t *testing.T) {
		header := `sig1=("@method");alg="ed25519";keyid="k"`
		_, _, err := findSignatureInput(header, "sig2")
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("entry without equals is skipped", func(t *testing.T) {
		header := `malformed, sig1=("@method");alg="ed25519";keyid="k"`
		label, _, err := findSignatureInput(header, "sig1")
		require.NoError(t, err)
		assert.Equal(t, "sig1", label)
	})

	t.Run("empty entries are skipped", func(t *testing.T) {
		header := `, , sig1=("@method");alg="ed25519";keyid="k"`
		label, _, err := findSignatureInput(header, "")
		require.NoError(t, err)
		assert.Equal(t, "sig1", label)
	})
}

func TestExtractSignatureValue(t *testing.T) {
	t.Run("valid signature", func(t *testing.T) {
		header := `sig1=:dGVzdA==:`
		sig, err := extractSignatureValue(header, "sig1")
		require.NoError(t, err)
		assert.Equal(t, []byte("test"), sig)
	})

	t.Run("comma-only separator", func(t *testing.T) {
		header := `sig1=:dGVzdA==:,sig2=:YWJj:`
		sig, err := extractSignatureValue(header, "sig2")
		require.NoError(t, err)
		assert.Equal(t, []byte("abc"), sig)
	})

	t.Run("label not found", func(t *testing.T) {
		header := `sig1=:dGVzdA==:`
		_, err := extractSignatureValue(header, "sig2")
		assert.ErrorIs(t, err, ErrSignatureNotFound)
	})

	t.Run("malformed value not byte sequence", func(t *testing.T) {
		header := `sig1=notcolonwrapped`
		_, err := extractSignatureValue(header, "sig1")
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("invalid base64", func(t *testing.T) {
		header := `sig1=:!!!:`
		_, err := extractSignatureValue(header, "sig1")
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})

	t.Run("entry without equals is skipped", func(t *testing.T) {
		header := `malformed, sig1=:dGVzdA==:`
		sig, err := extractSignatureValue(header, "sig1")
		require.NoError(t, err)
		assert.Equal(t, []byte("test"), sig)
	})

	t.Run("empty entries are skipped", func(t *testing.T) {
		header := `, , sig1=:dGVzdA==:`
		sig, err := extractSignatureValue(header, "sig1")
		require.NoError(t, err)
		assert.Equal(t, []byte("test"), sig)
	})
}
