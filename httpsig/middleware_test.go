package httpsig

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
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

		resp, err := client.Post(fmt.Sprintf("%s/api/data", server.URL), "application/json", strings.NewReader(`{"key":"value"}`))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestSignMiddleware(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := NewEd25519Signer("srv-key", priv)
	require.NoError(t, err)

	verifier, err := NewEd25519Verifier("srv-key", pub)
	require.NoError(t, err)

	resolver := func(_ *http.Request, keyID string, alg Algorithm) (Verifier, error) {
		if keyID == "srv-key" && alg == AlgorithmEd25519 {
			return verifier, nil
		}
		return nil, ErrInvalidKey
	}

	t.Run("nil signer returns error", func(t *testing.T) {
		_, err := SignMiddleware(SignConfig{})
		assert.ErrorIs(t, err, ErrNoSigner)
	})

	t.Run("signs buffered response verifiable by client", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"created"}`))
		}).Methods(http.MethodGet)

		mw, err := SignMiddleware(SignConfig{
			Signer:            signer,
			CoveredComponents: []string{"@status", "@authority;req", "@path;req", "content-type"},
			DigestAlgorithm:   DigestSHA256,
		})
		require.NoError(t, err)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Host = "example.com"

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, `{"status":"created"}`, w.Body.String())

		err = VerifyResponse(w.Result(), req, VerifyConfig{
			Resolver:           resolver,
			RequiredComponents: []string{"@status", "@authority;req", "content-digest"},
			RequireDigest:      true,
		})
		require.NoError(t, err)
	})

	t.Run("implicit 200 when handler only writes body", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/ok", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}).Methods(http.MethodGet)

		mw, err := SignMiddleware(SignConfig{Signer: signer})
		require.NoError(t, err)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/api/ok", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "ok", w.Body.String())
		require.NoError(t, VerifyResponse(w.Result(), req, VerifyConfig{Resolver: resolver}))
	})

	t.Run("signing failure returns 500", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/fail", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SignMiddleware(SignConfig{Signer: errSigner{err: assert.AnError}})
		require.NoError(t, err)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/api/fail", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("full request-response flow over HTTP", func(t *testing.T) {
		verifyMW, err := Middleware(MiddlewareConfig{
			Verify: VerifyConfig{Resolver: resolver},
		})
		require.NoError(t, err)

		signMW, err := SignMiddleware(SignConfig{
			Signer:            signer,
			CoveredComponents: []string{"@status", "@method;req", "@path;req"},
			DigestAlgorithm:   DigestSHA256,
		})
		require.NoError(t, err)

		r := mux.NewRouter()
		r.Use(verifyMW, signMW)
		r.HandleFunc("/api/echo", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("pong"))
		}).Methods(http.MethodGet)

		server := httptest.NewServer(r)
		defer server.Close()

		client := &http.Client{
			Transport: NewTransport(nil, SignConfig{Signer: signer}),
		}

		resp, err := client.Get(fmt.Sprintf("%s/api/echo", server.URL))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		// resp.Request is set by the client and used for ";req" components.
		err = VerifyResponse(resp, nil, VerifyConfig{Resolver: resolver, RequireDigest: true})
		require.NoError(t, err)
	})
}
