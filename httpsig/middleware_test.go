package httpsig

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestMiddleware(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := NewEd25519Signer("mw-key", priv)
	require.NoError(t, err)

	verifier, err := NewEd25519Verifier("mw-key", pub)
	require.NoError(t, err)

	resolver := func(_ *http.Request, keyID string, alg Algorithm) (Verifier, error) {
		if keyID == "mw-key" && alg == AlgorithmEd25519 {
			return verifier, nil
		}
		return nil, ErrInvalidKey
	}

	t.Run("nil resolver returns error", func(t *testing.T) {
		_, err := Middleware(MiddlewareConfig{})
		assert.ErrorIs(t, err, ErrNoResolver)
	})

	t.Run("valid signed request passes through", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := Middleware(MiddlewareConfig{
			Verify: VerifyConfig{Resolver: resolver},
		})
		require.NoError(t, err)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("unsigned request returns 401", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := Middleware(MiddlewareConfig{
			Verify: VerifyConfig{Resolver: resolver},
		})
		require.NoError(t, err)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("tampered request returns 401", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := Middleware(MiddlewareConfig{
			Verify: VerifyConfig{Resolver: resolver},
		})
		require.NoError(t, err)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Host = "example.com"

		err = SignRequest(req, SignConfig{Signer: signer})
		require.NoError(t, err)

		// Tamper with the request.
		req.Host = "attacker.com"

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("custom error handler", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		var capturedErr error
		mw, err := Middleware(MiddlewareConfig{
			Verify: VerifyConfig{Resolver: resolver},
			OnError: func(w http.ResponseWriter, _ *http.Request, err error) {
				capturedErr = err
				w.WriteHeader(http.StatusForbidden)
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.ErrorIs(t, capturedErr, ErrSignatureNotFound)
	})

	t.Run("end to end with transport and middleware", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/data", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}).Methods(http.MethodPost)

		mw, err := Middleware(MiddlewareConfig{
			Verify: VerifyConfig{
				Resolver:      resolver,
				RequireDigest: true,
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		server := httptest.NewServer(r)
		defer server.Close()

		client := &http.Client{
			Transport: NewTransport(nil, SignConfig{
				Signer:          signer,
				DigestAlgorithm: DigestSHA256,
			}),
		}

		resp, err := client.Post(server.URL+"/api/data", "application/json", strings.NewReader(`{"key":"value"}`))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
