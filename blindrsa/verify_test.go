package blindrsa

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerify(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey
	msg := []byte("message to verify")

	t.Run("round trip all variants", func(t *testing.T) {
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

				err = Verify(variant, pub, state.InputMessage(), sig)
				assert.NoError(t, err)
			})
		}
	})

	t.Run("wrong key fails verification", func(t *testing.T) {
		otherPriv, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		blindSig, err := BlindSign(VariantSHA384PSSDeterministic, priv, blindedMsg)
		require.NoError(t, err)

		sig, err := Finalize(VariantSHA384PSSDeterministic, pub, blindSig, state)
		require.NoError(t, err)

		err = Verify(VariantSHA384PSSDeterministic, &otherPriv.PublicKey, state.InputMessage(), sig)
		assert.ErrorIs(t, err, ErrVerifyFailed)
	})

	t.Run("tampered signature fails", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		blindSig, err := BlindSign(VariantSHA384PSSDeterministic, priv, blindedMsg)
		require.NoError(t, err)

		sig, err := Finalize(VariantSHA384PSSDeterministic, pub, blindSig, state)
		require.NoError(t, err)

		sig[0] ^= 0xFF
		err = Verify(VariantSHA384PSSDeterministic, pub, state.InputMessage(), sig)
		assert.ErrorIs(t, err, ErrVerifyFailed)
	})

	t.Run("tampered message fails", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		blindSig, err := BlindSign(VariantSHA384PSSDeterministic, priv, blindedMsg)
		require.NoError(t, err)

		sig, err := Finalize(VariantSHA384PSSDeterministic, pub, blindSig, state)
		require.NoError(t, err)

		err = Verify(VariantSHA384PSSDeterministic, pub, []byte("wrong message"), sig)
		assert.ErrorIs(t, err, ErrVerifyFailed)
	})

	t.Run("nil public key", func(t *testing.T) {
		err := Verify(VariantSHA384PSSDeterministic, nil, msg, make([]byte, 256))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("empty input message", func(t *testing.T) {
		err := Verify(VariantSHA384PSSDeterministic, pub, nil, make([]byte, 256))
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("empty signature", func(t *testing.T) {
		err := Verify(VariantSHA384PSSDeterministic, pub, msg, nil)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unsupported variant", func(t *testing.T) {
		err := Verify(Variant("invalid"), pub, msg, make([]byte, 256))
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})

	t.Run("PSSZERO signature fails with PSS variant", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSZERODeterministic, rand.Reader, msg)
		require.NoError(t, err)

		blindedMsg, state, err := Blind(VariantSHA384PSSZERODeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		blindSig, err := BlindSign(VariantSHA384PSSZERODeterministic, priv, blindedMsg)
		require.NoError(t, err)

		sig, err := Finalize(VariantSHA384PSSZERODeterministic, pub, blindSig, state)
		require.NoError(t, err)

		err = Verify(VariantSHA384PSSDeterministic, pub, state.InputMessage(), sig)
		assert.ErrorIs(t, err, ErrVerifyFailed)
	})
}

// allVariants returns all four RSABSSA variants.
func allVariants() []Variant {
	return []Variant{
		VariantSHA384PSSRandomized,
		VariantSHA384PSSZERORandomized,
		VariantSHA384PSSDeterministic,
		VariantSHA384PSSZERODeterministic,
	}
}
