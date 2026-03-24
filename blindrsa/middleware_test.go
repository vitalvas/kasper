package blindrsa

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestVerifyMiddleware(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey
	variant := VariantSHA384PSSDeterministic
	msg := []byte("middleware test message")

	msgFunc := func(_ *http.Request) ([]byte, error) {
		return msg, nil
	}

	t.Run("nil key returns error", func(t *testing.T) {
		_, err := VerifyMiddleware(MiddlewareConfig{
			Variant:     variant,
			MessageFunc: msgFunc,
		})
		assert.ErrorIs(t, err, ErrNoVerifier)
	})

	t.Run("small key rejected", func(t *testing.T) {
		smallKey, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)

		_, err = VerifyMiddleware(MiddlewareConfig{
			Key:         &smallKey.PublicKey,
			Variant:     variant,
			MessageFunc: msgFunc,
		})
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("unsupported variant", func(t *testing.T) {
		_, err := VerifyMiddleware(MiddlewareConfig{
			Key:         pub,
			Variant:     Variant("invalid"),
			MessageFunc: msgFunc,
		})
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})

	t.Run("nil message func", func(t *testing.T) {
		_, err := VerifyMiddleware(MiddlewareConfig{
			Key:     pub,
			Variant: variant,
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("valid signature passes through", func(t *testing.T) {
		prepared, err := Prepare(variant, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(variant, pub, rand.Reader, prepared)
		require.NoError(t, err)

		blindSig, err := BlindSign(variant, priv, blindedMsg)
		require.NoError(t, err)

		sig, err := Finalize(variant, pub, blindSig, state)
		require.NoError(t, err)

		inputMsg := state.InputMessage()
		mw, err := VerifyMiddleware(MiddlewareConfig{
			Key:     pub,
			Variant: variant,
			MessageFunc: func(_ *http.Request) ([]byte, error) {
				return inputMsg, nil
			},
		})
		require.NoError(t, err)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Blind-Signature", base64.StdEncoding.EncodeToString(sig))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing header returns 401", func(t *testing.T) {
		mw, err := VerifyMiddleware(MiddlewareConfig{
			Key:         pub,
			Variant:     variant,
			MessageFunc: msgFunc,
		})
		require.NoError(t, err)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid base64 returns 401", func(t *testing.T) {
		mw, err := VerifyMiddleware(MiddlewareConfig{
			Key:         pub,
			Variant:     variant,
			MessageFunc: msgFunc,
		})
		require.NoError(t, err)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Blind-Signature", "not-valid-base64!!!")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid signature returns 401", func(t *testing.T) {
		mw, err := VerifyMiddleware(MiddlewareConfig{
			Key:         pub,
			Variant:     variant,
			MessageFunc: msgFunc,
		})
		require.NoError(t, err)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		fakeSig := make([]byte, keyLen(pub))
		req.Header.Set("Blind-Signature", base64.StdEncoding.EncodeToString(fakeSig))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("custom header name", func(t *testing.T) {
		prepared, err := Prepare(variant, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(variant, pub, rand.Reader, prepared)
		require.NoError(t, err)

		blindSig, err := BlindSign(variant, priv, blindedMsg)
		require.NoError(t, err)

		sig, err := Finalize(variant, pub, blindSig, state)
		require.NoError(t, err)

		inputMsg := state.InputMessage()
		mw, err := VerifyMiddleware(MiddlewareConfig{
			Key:        pub,
			Variant:    variant,
			HeaderName: "X-Token",
			MessageFunc: func(_ *http.Request) ([]byte, error) {
				return inputMsg, nil
			},
		})
		require.NoError(t, err)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Token", base64.StdEncoding.EncodeToString(sig))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("custom error handler", func(t *testing.T) {
		var capturedErr error
		mw, err := VerifyMiddleware(MiddlewareConfig{
			Key:         pub,
			Variant:     variant,
			MessageFunc: msgFunc,
			OnError: func(w http.ResponseWriter, _ *http.Request, err error) {
				capturedErr = err
				w.WriteHeader(http.StatusForbidden)
			},
		})
		require.NoError(t, err)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Error(t, capturedErr)
	})

	t.Run("message func error returns 401", func(t *testing.T) {
		mw, err := VerifyMiddleware(MiddlewareConfig{
			Key:     pub,
			Variant: variant,
			MessageFunc: func(_ *http.Request) ([]byte, error) {
				return nil, fmt.Errorf("extraction failed")
			},
		})
		require.NoError(t, err)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Blind-Signature", base64.StdEncoding.EncodeToString(make([]byte, keyLen(pub))))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestVerifyMiddlewareEndToEnd(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey

	for _, variant := range allVariants() {
		t.Run(variant.String(), func(t *testing.T) {
			msg := []byte("e2e middleware test")

			handler, err := IssueHandler(IssuerConfig{
				Key:     priv,
				Variant: variant,
			})
			require.NoError(t, err)

			issuer := httptest.NewServer(handler)
			defer issuer.Close()

			client, err := NewClient(ClientConfig{
				Key:           pub,
				Variant:       variant,
				TokenEndpoint: issuer.URL,
			})
			require.NoError(t, err)

			sig, state, err := client.ObtainSignature(context.Background(), msg)
			require.NoError(t, err)

			inputMsg := state.InputMessage()
			mw, err := VerifyMiddleware(MiddlewareConfig{
				Key:     pub,
				Variant: variant,
				MessageFunc: func(_ *http.Request) ([]byte, error) {
					return inputMsg, nil
				},
			})
			require.NoError(t, err)

			r := mux.NewRouter()
			r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}).Methods(http.MethodGet)
			r.Use(mw)

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Blind-Signature", base64.StdEncoding.EncodeToString(sig))

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}
