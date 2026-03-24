package blindrsa

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey

	t.Run("nil key returns error", func(t *testing.T) {
		_, err := NewClient(ClientConfig{
			Variant:       VariantSHA384PSSDeterministic,
			TokenEndpoint: "http://localhost/issue",
		})
		assert.ErrorIs(t, err, ErrNoVerifier)
	})

	t.Run("small key rejected", func(t *testing.T) {
		smallKey, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)

		_, err = NewClient(ClientConfig{
			Key:           &smallKey.PublicKey,
			Variant:       VariantSHA384PSSDeterministic,
			TokenEndpoint: "http://localhost/issue",
		})
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("unsupported variant", func(t *testing.T) {
		_, err := NewClient(ClientConfig{
			Key:           pub,
			Variant:       Variant("invalid"),
			TokenEndpoint: "http://localhost/issue",
		})
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})

	t.Run("empty endpoint", func(t *testing.T) {
		_, err := NewClient(ClientConfig{
			Key:     pub,
			Variant: VariantSHA384PSSDeterministic,
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("valid config with nil transport", func(t *testing.T) {
		client, err := NewClient(ClientConfig{
			Key:           pub,
			Variant:       VariantSHA384PSSDeterministic,
			TokenEndpoint: "http://localhost/issue",
		})
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("custom transport", func(t *testing.T) {
		client, err := NewClient(ClientConfig{
			Key:           pub,
			Variant:       VariantSHA384PSSDeterministic,
			TokenEndpoint: "http://localhost/issue",
			Transport:     &http.Transport{},
		})
		require.NoError(t, err)
		assert.NotNil(t, client)
	})
}

func TestClientObtainSignature(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey
	msg := []byte("message for transport test")

	t.Run("end to end with httptest", func(t *testing.T) {
		variant := VariantSHA384PSSDeterministic

		handler, err := IssueHandler(IssuerConfig{
			Key:     priv,
			Variant: variant,
		})
		require.NoError(t, err)

		server := httptest.NewServer(handler)
		defer server.Close()

		client, err := NewClient(ClientConfig{
			Key:           pub,
			Variant:       variant,
			TokenEndpoint: server.URL,
		})
		require.NoError(t, err)

		sig, state, err := client.ObtainSignature(context.Background(), msg)
		require.NoError(t, err)
		assert.Len(t, sig, keyLen(pub))

		err = Verify(variant, pub, state.InputMessage(), sig)
		assert.NoError(t, err)
	})

	t.Run("all variants end to end", func(t *testing.T) {
		for _, variant := range allVariants() {
			t.Run(variant.String(), func(t *testing.T) {
				handler, err := IssueHandler(IssuerConfig{
					Key:     priv,
					Variant: variant,
				})
				require.NoError(t, err)

				server := httptest.NewServer(handler)
				defer server.Close()

				client, err := NewClient(ClientConfig{
					Key:           pub,
					Variant:       variant,
					TokenEndpoint: server.URL,
				})
				require.NoError(t, err)

				sig, state, err := client.ObtainSignature(context.Background(), msg)
				require.NoError(t, err)

				err = Verify(variant, pub, state.InputMessage(), sig)
				assert.NoError(t, err)
			})
		}
	})

	t.Run("server error propagates", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client, err := NewClient(ClientConfig{
			Key:           pub,
			Variant:       VariantSHA384PSSDeterministic,
			TokenEndpoint: server.URL,
		})
		require.NoError(t, err)

		_, _, err = client.ObtainSignature(context.Background(), msg)
		assert.ErrorIs(t, err, ErrSignatureFailed)
	})

	t.Run("server returns garbage", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(bytes.Repeat([]byte{0xAB}, keyLen(pub)))
		}))
		defer server.Close()

		client, err := NewClient(ClientConfig{
			Key:           pub,
			Variant:       VariantSHA384PSSDeterministic,
			TokenEndpoint: server.URL,
		})
		require.NoError(t, err)

		_, _, err = client.ObtainSignature(context.Background(), msg)
		assert.Error(t, err)
	})

	t.Run("nil message fails prepare", func(t *testing.T) {
		handler, err := IssueHandler(IssuerConfig{
			Key:     priv,
			Variant: VariantSHA384PSSDeterministic,
		})
		require.NoError(t, err)

		server := httptest.NewServer(handler)
		defer server.Close()

		client, err := NewClient(ClientConfig{
			Key:           pub,
			Variant:       VariantSHA384PSSDeterministic,
			TokenEndpoint: server.URL,
		})
		require.NoError(t, err)

		_, _, err = client.ObtainSignature(context.Background(), nil)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unreachable server", func(t *testing.T) {
		client, err := NewClient(ClientConfig{
			Key:           pub,
			Variant:       VariantSHA384PSSDeterministic,
			TokenEndpoint: "http://127.0.0.1:1",
		})
		require.NoError(t, err)

		_, _, err = client.ObtainSignature(context.Background(), msg)
		assert.ErrorIs(t, err, ErrSignatureFailed)
	})

	t.Run("server returns wrong size", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "too short")
		}))
		defer server.Close()

		client, err := NewClient(ClientConfig{
			Key:           pub,
			Variant:       VariantSHA384PSSDeterministic,
			TokenEndpoint: server.URL,
		})
		require.NoError(t, err)

		_, _, err = client.ObtainSignature(context.Background(), msg)
		assert.Error(t, err)
	})
}
