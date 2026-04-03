package blindrsa

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReader is an io.Reader that always returns an error.
type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read error")
}

func TestPrepare(t *testing.T) {
	t.Run("randomized variants prepend 32-byte prefix", func(t *testing.T) {
		msg := []byte("test message")
		for _, variant := range []Variant{VariantSHA384PSSRandomized, VariantSHA384PSSZERORandomized} {
			t.Run(variant.String(), func(t *testing.T) {
				prepared, err := Prepare(variant, rand.Reader, msg)
				require.NoError(t, err)
				assert.Len(t, prepared, randomPrefixLen+len(msg))
				assert.Equal(t, msg, prepared[randomPrefixLen:])
			})
		}
	})

	t.Run("deterministic variants return copy", func(t *testing.T) {
		msg := []byte("test message")
		for _, variant := range []Variant{VariantSHA384PSSDeterministic, VariantSHA384PSSZERODeterministic} {
			t.Run(variant.String(), func(t *testing.T) {
				prepared, err := Prepare(variant, rand.Reader, msg)
				require.NoError(t, err)
				assert.Equal(t, msg, prepared)
			})
		}
	})

	t.Run("nil message", func(t *testing.T) {
		_, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, nil)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unsupported variant", func(t *testing.T) {
		_, err := Prepare(Variant("invalid"), rand.Reader, []byte("msg"))
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})

	t.Run("nil random for randomized variant", func(t *testing.T) {
		_, err := Prepare(VariantSHA384PSSRandomized, nil, []byte("msg"))
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("nil random for deterministic variant is ok", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, nil, []byte("msg"))
		require.NoError(t, err)
		assert.Equal(t, []byte("msg"), prepared)
	})

	t.Run("randomized with failing reader", func(t *testing.T) {
		_, err := Prepare(VariantSHA384PSSRandomized, errReader{}, []byte("msg"))
		assert.ErrorIs(t, err, ErrBlindingFailed)
	})

	t.Run("randomized prefix is non-deterministic", func(t *testing.T) {
		msg := []byte("msg")
		p1, err := Prepare(VariantSHA384PSSRandomized, rand.Reader, msg)
		require.NoError(t, err)

		p2, err := Prepare(VariantSHA384PSSRandomized, rand.Reader, msg)
		require.NoError(t, err)

		assert.NotEqual(t, p1, p2, "prefixes should differ")
	})
}

func TestBlind(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey
	msg := []byte("test message for blinding")

	t.Run("all variants produce valid blinded message", func(t *testing.T) {
		for _, variant := range allVariants() {
			t.Run(variant.String(), func(t *testing.T) {
				prepared, err := Prepare(variant, rand.Reader, msg)
				require.NoError(t, err)

				blindedMsg, state, err := Blind(variant, pub, rand.Reader, prepared)
				require.NoError(t, err)
				assert.Len(t, blindedMsg, keyLen(pub))
				assert.NotNil(t, state)
				assert.NotNil(t, state.blindInv)
				assert.NotEmpty(t, state.InputMessage())
			})
		}
	})

	t.Run("state stores prepared message", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		_, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		assert.Equal(t, prepared, state.InputMessage())
	})

	t.Run("output is non-deterministic", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		b1, _, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		b2, _, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		assert.NotEqual(t, b1, b2, "blinded messages should differ due to random blinding factor")
	})

	t.Run("nil public key", func(t *testing.T) {
		_, _, err := Blind(VariantSHA384PSSDeterministic, nil, rand.Reader, msg)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("small key rejected", func(t *testing.T) {
		smallKey, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)

		_, _, err = Blind(VariantSHA384PSSDeterministic, &smallKey.PublicKey, rand.Reader, msg)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("empty prepared message", func(t *testing.T) {
		_, _, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, nil)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("nil random", func(t *testing.T) {
		_, _, err := Blind(VariantSHA384PSSDeterministic, pub, nil, msg)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unsupported variant", func(t *testing.T) {
		_, _, err := Blind(Variant("invalid"), pub, rand.Reader, msg)
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})

	t.Run("InputMessage returns copy", func(t *testing.T) {
		prepared, err := Prepare(VariantSHA384PSSDeterministic, rand.Reader, msg)
		require.NoError(t, err)

		_, state, err := Blind(VariantSHA384PSSDeterministic, pub, rand.Reader, prepared)
		require.NoError(t, err)

		m1 := state.InputMessage()
		m2 := state.InputMessage()
		assert.Equal(t, m1, m2)

		m1[0] = 0xFF
		assert.NotEqual(t, m1, state.InputMessage(), "modifying returned slice should not affect state")
	})

	t.Run("failing random reader", func(t *testing.T) {
		// PSSZERO has sLen=0 so PSS encoding won't need random for salt,
		// but blinding factor generation will fail.
		_, _, err := Blind(VariantSHA384PSSZERODeterministic, pub, errReader{}, msg)
		assert.ErrorIs(t, err, ErrBlindingFailed)
	})

	t.Run("failing random reader with salt", func(t *testing.T) {
		// PSS variant needs random for salt generation in PSS encoding.
		_, _, err := Blind(VariantSHA384PSSDeterministic, pub, errReader{}, msg)
		assert.ErrorIs(t, err, ErrBlindingFailed)
	})
}

func TestFixedBlind(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &priv.PublicKey
	msg := []byte("fixed blind test")

	t.Run("nil public key", func(t *testing.T) {
		_, _, err := fixedBlind(fixedBlindParams{
			variant:     VariantSHA384PSSDeterministic,
			preparedMsg: msg,
			salt:        make([]byte, 48),
			r:           big.NewInt(1),
			rInv:        big.NewInt(1),
		})
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("unsupported variant", func(t *testing.T) {
		_, _, err := fixedBlind(fixedBlindParams{
			variant:     Variant("bad"),
			pub:         pub,
			preparedMsg: msg,
			r:           big.NewInt(1),
			rInv:        big.NewInt(1),
		})
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})

	t.Run("empty message", func(t *testing.T) {
		_, _, err := fixedBlind(fixedBlindParams{
			variant: VariantSHA384PSSDeterministic,
			pub:     pub,
			salt:    make([]byte, 48),
			r:       big.NewInt(1),
			rInv:    big.NewInt(1),
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("wrong salt length", func(t *testing.T) {
		_, _, err := fixedBlind(fixedBlindParams{
			variant:     VariantSHA384PSSDeterministic,
			pub:         pub,
			preparedMsg: msg,
			salt:        make([]byte, 10),
			r:           big.NewInt(1),
			rInv:        big.NewInt(1),
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})
}
