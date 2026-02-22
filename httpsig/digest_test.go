package httpsig

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, fmt.Errorf("read error") }
func (errReader) Close() error             { return nil }

func TestSetContentDigest(t *testing.T) {
	t.Run("sha-256", func(t *testing.T) {
		body := "hello world"
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader(body))

		err := SetContentDigest(req, DigestSHA256)
		require.NoError(t, err)

		header := req.Header.Get("Content-Digest")
		assert.NotEmpty(t, header)

		expected := sha256.Sum256([]byte(body))
		want := fmt.Sprintf("sha-256=:%s:", base64.StdEncoding.EncodeToString(expected[:]))
		assert.Equal(t, want, header)

		// Body should still be readable.
		restoredBody, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.Equal(t, body, string(restoredBody))
	})

	t.Run("sha-512", func(t *testing.T) {
		body := "hello world"
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader(body))

		err := SetContentDigest(req, DigestSHA512)
		require.NoError(t, err)

		header := req.Header.Get("Content-Digest")
		expected := sha512.Sum512([]byte(body))
		want := fmt.Sprintf("sha-512=:%s:", base64.StdEncoding.EncodeToString(expected[:]))
		assert.Equal(t, want, header)
	})

	t.Run("nil body", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/", nil)
		req.Body = nil

		err := SetContentDigest(req, DigestSHA256)
		require.NoError(t, err)

		header := req.Header.Get("Content-Digest")
		assert.NotEmpty(t, header)
	})

	t.Run("unsupported algorithm", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader("body"))

		err := SetContentDigest(req, DigestAlgorithm("md5"))
		assert.ErrorIs(t, err, ErrUnsupportedDigest)
	})

	t.Run("broken body reader", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/", errReader{})

		err := SetContentDigest(req, DigestSHA256)
		assert.Error(t, err)
	})
}

func TestVerifyContentDigest(t *testing.T) {
	t.Run("valid sha-256 digest", func(t *testing.T) {
		body := "hello world"
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader(body))

		err := SetContentDigest(req, DigestSHA256)
		require.NoError(t, err)

		err = VerifyContentDigest(req)
		assert.NoError(t, err)
	})

	t.Run("valid sha-512 digest", func(t *testing.T) {
		body := "hello world"
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader(body))

		err := SetContentDigest(req, DigestSHA512)
		require.NoError(t, err)

		err = VerifyContentDigest(req)
		assert.NoError(t, err)
	})

	t.Run("missing digest header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader("body"))

		err := VerifyContentDigest(req)
		assert.ErrorIs(t, err, ErrDigestNotFound)
	})

	t.Run("tampered body", func(t *testing.T) {
		body := "original"
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader(body))

		err := SetContentDigest(req, DigestSHA256)
		require.NoError(t, err)

		// Replace body with different content.
		req.Body = io.NopCloser(strings.NewReader("tampered"))

		err = VerifyContentDigest(req)
		assert.ErrorIs(t, err, ErrDigestMismatch)
	})

	t.Run("unsupported algorithm in header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader("body"))
		req.Header.Set("Content-Digest", "md5=:abc123:")

		err := VerifyContentDigest(req)
		assert.ErrorIs(t, err, ErrUnsupportedDigest)
	})

	t.Run("malformed digest header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader("body"))
		req.Header.Set("Content-Digest", "sha-256=notcolonwrapped")

		err := VerifyContentDigest(req)
		assert.ErrorIs(t, err, ErrUnsupportedDigest)
	})

	t.Run("entry without equals is skipped", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader("body"))
		req.Header.Set("Content-Digest", "malformed-no-equals")

		err := VerifyContentDigest(req)
		assert.ErrorIs(t, err, ErrUnsupportedDigest)
	})

	t.Run("broken body reader", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/", errReader{})
		req.Header.Set("Content-Digest", "sha-256=:abc:")

		err := VerifyContentDigest(req)
		assert.Error(t, err)
	})

	t.Run("invalid base64 in digest", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://example.com/", strings.NewReader("body"))
		req.Header.Set("Content-Digest", "sha-256=:!!!invalid!!!:")

		err := VerifyContentDigest(req)
		assert.ErrorIs(t, err, ErrMalformedHeader)
	})
}
