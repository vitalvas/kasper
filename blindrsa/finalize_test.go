package blindrsa

import (
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinalize(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey
	msg := []byte("message to finalize")

	t.Run("all variants produce valid signature", func(t *testing.T) {
		for _, variant := range allVariants() {
			t.Run(variant.String(), func(t *testing.T) {
				prepared, err := Prepare(variant, rand.Reader, msg)
				require.NoError(t, err)

				blindedMsg, state, err := Blind(variant, pub, rand.Reader, prepared)
				require.NoError(t, err)

				blindSig, err := BlindSign(variant, priv, blindedMsg)
				require.NoError(t, err)

				sig, err := Finalize(variant, pub, blindSig, state)
				require.NoError(t, err)
				assert.Len(t, sig, keyLen(pub))
			})
		}
	})

	t.Run("tampered blind signature fails", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		blindSig, err := BlindSign(VariantSHA384PSSDeterministic, priv, blindedMsg)
		require.NoError(t, err)

		blindSig[0] ^= 0xFF
		_, err = Finalize(VariantSHA384PSSDeterministic, pub, blindSig, state)
		assert.ErrorIs(t, err, ErrFinalizeFailed)
	})

	t.Run("nil state", func(t *testing.T) {
		_, err := Finalize(VariantSHA384PSSDeterministic, pub, make([]byte, keyLen(pub)), nil)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("nil public key", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		_, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		_, err = Finalize(VariantSHA384PSSDeterministic, nil, make([]byte, keyLen(pub)), state)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("wrong size blind signature", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		_, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		_, err = Finalize(VariantSHA384PSSDeterministic, pub, make([]byte, 10), state)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unsupported variant", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		_, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		_, err = Finalize(Variant("invalid"), pub, make([]byte, keyLen(pub)), state)
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})

	t.Run("wrong key fails", func(t *testing.T) {
		otherPriv, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		// Signing with the wrong key may fail at BlindSign (out of range)
		// or at Finalize (verification failure). Either way, we expect an error.
		blindSig, err := BlindSign(VariantSHA384PSSDeterministic, otherPriv, blindedMsg)
		if err != nil {
			assert.Error(t, err)
			return
		}

		_, err = Finalize(VariantSHA384PSSDeterministic, pub, blindSig, state)
		assert.ErrorIs(t, err, ErrFinalizeFailed)
	})

	t.Run("corrupted state blindInv causes finalize failure", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		blindSig, err := BlindSign(VariantSHA384PSSDeterministic, priv, blindedMsg)
		require.NoError(t, err)

		// Corrupt the blinding inverse to produce an invalid unblinded signature.
		state.blindInv = big.NewInt(42)

		_, err = Finalize(VariantSHA384PSSDeterministic, pub, blindSig, state)
		assert.ErrorIs(t, err, ErrFinalizeFailed)
	})
}
