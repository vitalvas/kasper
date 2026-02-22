package httpsig

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTransport(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := NewEd25519Signer("transport-key", priv)
	require.NoError(t, err)

	verifier, err := NewEd25519Verifier("transport-key", pub)
	require.NoError(t, err)

	resolver := func(_ *http.Request, keyID string, alg Algorithm) (Verifier, error) {
		if keyID == "transport-key" && alg == AlgorithmEd25519 {
			return verifier, nil
		}
		return nil, ErrInvalidKey
	}

	t.Run("nil base clones default transport", func(t *testing.T) {
		transport := NewTransport(nil, SignConfig{Signer: signer})
		assert.NotNil(t, transport)
		assert.NotNil(t, transport.base)

		// Should be a distinct instance, not the global default.
		assert.NotSame(t, http.DefaultTransport, transport.base)
	})

	t.Run("custom base is used", func(t *testing.T) {
		base := &http.Transport{
			IdleConnTimeout: 42 * time.Second,
		}

		transport := NewTransport(base, SignConfig{Signer: signer})
		assert.Same(t, base, transport.base)
	})

	t.Run("signs requests automatically", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.NotEmpty(t, r.Header.Get("Signature"))
			assert.NotEmpty(t, r.Header.Get("Signature-Input"))

			err := VerifyRequest(r, VerifyConfig{Resolver: resolver})
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &http.Client{
			Transport: NewTransport(nil, SignConfig{Signer: signer}),
		}

		resp, err := client.Get(server.URL + "/api/items")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("nil signer returns error", func(t *testing.T) {
		client := &http.Client{
			Transport: NewTransport(nil, SignConfig{}),
		}

		_, err := client.Get("http://localhost/test")
		assert.Error(t, err)
	})

	t.Run("does not mutate original request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &http.Client{
			Transport: NewTransport(nil, SignConfig{Signer: signer}),
		}

		req, err := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
		require.NoError(t, err)

		origSig := req.Header.Get("Signature")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, origSig, req.Header.Get("Signature"))
	})

	t.Run("does not consume original request body with digest", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.NotEmpty(t, r.Header.Get("Content-Digest"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &http.Client{
			Transport: NewTransport(nil, SignConfig{
				Signer:          signer,
				DigestAlgorithm: DigestSHA256,
			}),
		}

		bodyContent := "test body content"
		req, err := http.NewRequest(http.MethodPost, server.URL+"/test", strings.NewReader(bodyContent))
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// The original request's GetBody should still work.
		if req.GetBody != nil {
			body, err := req.GetBody()
			require.NoError(t, err)

			data, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, bodyContent, string(data))
		}
	})

	t.Run("custom base with TLS config", func(t *testing.T) {
		base := &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		}

		transport := NewTransport(base, SignConfig{Signer: signer})

		underlying, ok := transport.base.(*http.Transport)
		require.True(t, ok)
		assert.Equal(t, uint16(tls.VersionTLS13), underlying.TLSClientConfig.MinVersion)
	})

	t.Run("custom base with proxy", func(t *testing.T) {
		base := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}

		transport := NewTransport(base, SignConfig{Signer: signer})

		underlying, ok := transport.base.(*http.Transport)
		require.True(t, ok)
		assert.NotNil(t, underlying.Proxy)
	})
}
