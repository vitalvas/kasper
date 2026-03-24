package blindrsa

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlindSign(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey
	msg := []byte("message to blind sign")

	t.Run("all variants produce valid blind signature", func(t *testing.T) {
		for _, variant := range allVariants() {
			t.Run(variant.String(), func(t *testing.T) {
				prepared, err := Prepare(variant, rand.Reader, msg)
				require.NoError(t, err)

				blindedMsg, _, err := Blind(variant, pub, rand.Reader, prepared)
				require.NoError(t, err)

				blindSig, err := BlindSign(variant, priv, blindedMsg)
				require.NoError(t, err)
				assert.Len(t, blindSig, keyLen(pub))
			})
		}
	})

	t.Run("nil private key", func(t *testing.T) {
		_, err := BlindSign(VariantSHA384PSSDeterministic, nil, make([]byte, keyLen(pub)))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("small key rejected", func(t *testing.T) {
		smallKey, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)

		_, err = BlindSign(VariantSHA384PSSDeterministic, smallKey, make([]byte, 128))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("empty blinded message", func(t *testing.T) {
		_, err := BlindSign(VariantSHA384PSSDeterministic, priv, nil)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("wrong size blinded message", func(t *testing.T) {
		_, err := BlindSign(VariantSHA384PSSDeterministic, priv, make([]byte, 10))
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unsupported variant", func(t *testing.T) {
		_, err := BlindSign(Variant("invalid"), priv, make([]byte, keyLen(pub)))
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})

	t.Run("out of range message", func(t *testing.T) {
		// Create a blinded message with all 0xFF bytes, which is >= n for any key.
		bigMsg := make([]byte, keyLen(pub))
		for i := range bigMsg {
			bigMsg[i] = 0xFF
		}

		_, err := BlindSign(VariantSHA384PSSDeterministic, priv, bigMsg)
		assert.ErrorIs(t, err, ErrSignatureFailed)
	})
}
