package blindrsa

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueHandler(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey
	variant := VariantSHA384PSSDeterministic
	msg := []byte("handler test message")

	t.Run("valid request returns blind signature", func(t *testing.T) {
		handler, err := IssueHandler(IssuerConfig{
			Key:     priv,
			Variant: variant,
		})
		require.NoError(t, err)

		prepared, err := Prepare(variant, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(variant, pub, rand.Reader, prepared)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/issue", bytes.NewReader(blindedMsg))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))

		blindSig := w.Body.Bytes()
		assert.Len(t, blindSig, keyLen(pub))

		sig, err := Finalize(variant, pub, blindSig, state)
		require.NoError(t, err)

		err = Verify(variant, pub, state.InputMessage(), sig)
		assert.NoError(t, err)
	})

	t.Run("nil key returns error", func(t *testing.T) {
		_, err := IssueHandler(IssuerConfig{
			Variant: variant,
		})
		assert.ErrorIs(t, err, ErrNoSigner)
	})

	t.Run("small key rejected", func(t *testing.T) {
		smallKey, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)

		_, err = IssueHandler(IssuerConfig{
			Key:     smallKey,
			Variant: variant,
		})
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("unsupported variant", func(t *testing.T) {
		_, err := IssueHandler(IssuerConfig{
			Key:     priv,
			Variant: Variant("invalid"),
		})
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})

	t.Run("oversized body returns error", func(t *testing.T) {
		handler, err := IssueHandler(IssuerConfig{
			Key:         priv,
			Variant:     variant,
			MaxBodySize: 10,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/issue", bytes.NewReader(make([]byte, 100)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("wrong size body returns error", func(t *testing.T) {
		handler, err := IssueHandler(IssuerConfig{
			Key:     priv,
			Variant: variant,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/issue", bytes.NewReader(make([]byte, 10)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("custom error handler", func(t *testing.T) {
		var capturedErr error
		handler, err := IssueHandler(IssuerConfig{
			Key:     priv,
			Variant: variant,
			OnError: func(w http.ResponseWriter, _ *http.Request, err error) {
				capturedErr = err
				w.WriteHeader(http.StatusUnprocessableEntity)
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/issue", bytes.NewReader(make([]byte, 10)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		assert.Error(t, capturedErr)
	})

	t.Run("body read error returns error", func(t *testing.T) {
		handler, err := IssueHandler(IssuerConfig{
			Key:     priv,
			Variant: variant,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/issue",
			io.NopCloser(&failingReader{}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty body returns error", func(t *testing.T) {
		handler, err := IssueHandler(IssuerConfig{
			Key:     priv,
			Variant: variant,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/issue", http.NoBody)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("all variants via handler", func(t *testing.T) {
		for _, v := range allVariants() {
			t.Run(v.String(), func(t *testing.T) {
				handler, err := IssueHandler(IssuerConfig{
					Key:     priv,
					Variant: v,
				})
				require.NoError(t, err)

				prepared, err := Prepare(v, rand.Reader, msg)
				require.NoError(t, err)

				blindedMsg, state, err := Blind(v, pub, rand.Reader, prepared)
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodPost, "/issue", bytes.NewReader(blindedMsg))
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)

				require.Equal(t, http.StatusOK, w.Code)

				blindSig, err := io.ReadAll(w.Body)
				require.NoError(t, err)

				sig, err := Finalize(v, pub, blindSig, state)
				require.NoError(t, err)

				err = Verify(v, pub, state.InputMessage(), sig)
				assert.NoError(t, err)
			})
		}
	})
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read error")
}
