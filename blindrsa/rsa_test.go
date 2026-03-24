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

func TestValidatePublicKey(t *testing.T) {
	t.Run("nil key", func(t *testing.T) {
		err := validatePublicKey(nil)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("small key", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)

		err = validatePublicKey(&key.PublicKey)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("small exponent", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		pub := key.PublicKey
		pub.E = 3

		err = validatePublicKey(&pub)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("valid key", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		err = validatePublicKey(&key.PublicKey)
		assert.NoError(t, err)
	})
}

func TestValidatePrivateKey(t *testing.T) {
	t.Run("nil key", func(t *testing.T) {
		err := validatePrivateKey(nil)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("small key", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)

		err = validatePrivateKey(key)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("small exponent", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		key.E = 3

		err = validatePrivateKey(key)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("valid key", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		err = validatePrivateKey(key)
		assert.NoError(t, err)
	})
}

func TestKeyLen(t *testing.T) {
	t.Run("2048 bit key", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		assert.Equal(t, 256, keyLen(&key.PublicKey))
	})

	t.Run("4096 bit key", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 4096)
		require.NoError(t, err)

		assert.Equal(t, 512, keyLen(&key.PublicKey))
	})
}

func TestValidateVariant(t *testing.T) {
	t.Run("valid variants", func(t *testing.T) {
		for _, v := range allVariants() {
			t.Run(v.String(), func(t *testing.T) {
				sLen, err := validateVariant(v)
				assert.NoError(t, err)
				assert.GreaterOrEqual(t, sLen, 0)
			})
		}
	})

	t.Run("invalid variant", func(t *testing.T) {
		_, err := validateVariant(Variant("invalid"))
		assert.ErrorIs(t, err, ErrUnsupportedVariant)
	})
}

func TestGenerateBlindingFactor(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub := &key.PublicKey

	t.Run("returns valid blinding factors", func(t *testing.T) {
		rE, rInv, err := generateBlindingFactor(rand.Reader, pub)
		require.NoError(t, err)
		assert.NotNil(t, rE)
		assert.NotNil(t, rInv)

		assert.True(t, rE.Sign() > 0)
		assert.True(t, rInv.Sign() > 0)
		assert.True(t, rE.Cmp(pub.N) < 0)
		assert.True(t, rInv.Cmp(pub.N) < 0)
	})

	t.Run("produces different values each call", func(t *testing.T) {
		rE1, _, err := generateBlindingFactor(rand.Reader, pub)
		require.NoError(t, err)

		rE2, _, err := generateBlindingFactor(rand.Reader, pub)
		require.NoError(t, err)

		assert.NotEqual(t, rE1.Bytes(), rE2.Bytes())
	})
}

func TestEmsaPSSEncode(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	emBits := key.N.BitLen() - 1

	t.Run("valid encoding with salt", func(t *testing.T) {
		mHash := make([]byte, sha384Size)
		em, err := emsaPSSEncode(rand.Reader, mHash, emBits, sha384Size)
		require.NoError(t, err)

		emLen := (emBits + 7) / 8
		assert.Len(t, em, emLen)
		assert.Equal(t, byte(0xBC), em[len(em)-1])
	})

	t.Run("valid encoding without salt", func(t *testing.T) {
		mHash := make([]byte, sha384Size)
		em, err := emsaPSSEncode(rand.Reader, mHash, emBits, 0)
		require.NoError(t, err)

		emLen := (emBits + 7) / 8
		assert.Len(t, em, emLen)
		assert.Equal(t, byte(0xBC), em[len(em)-1])
	})

	t.Run("wrong hash length", func(t *testing.T) {
		mHash := make([]byte, 32)
		_, err := emsaPSSEncode(rand.Reader, mHash, emBits, 0)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("emLen too small", func(t *testing.T) {
		mHash := make([]byte, sha384Size)
		_, err := emsaPSSEncode(rand.Reader, mHash, 8, sha384Size)
		assert.ErrorIs(t, err, ErrMessageTooLong)
	})

	t.Run("failing reader", func(t *testing.T) {
		mHash := make([]byte, sha384Size)
		_, err := emsaPSSEncode(failReader{}, mHash, emBits, sha384Size)
		assert.ErrorIs(t, err, ErrBlindingFailed)
	})
}

type failReader struct{}

func (failReader) Read([]byte) (int, error) {
	return 0, errors.New("read error")
}

func TestGenerateBlindingFactorFailingReader(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	_, _, err = generateBlindingFactor(failReader{}, &key.PublicKey)
	assert.ErrorIs(t, err, ErrBlindingFailed)
}

func TestEmsaPSSEncodeWithSalt(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	emBits := key.N.BitLen() - 1

	t.Run("deterministic with fixed salt", func(t *testing.T) {
		mHash := make([]byte, sha384Size)
		salt := make([]byte, sha384Size)

		em1, err := emsaPSSEncodeWithSalt(mHash, emBits, salt)
		require.NoError(t, err)

		em2, err := emsaPSSEncodeWithSalt(mHash, emBits, salt)
		require.NoError(t, err)

		assert.Equal(t, em1, em2, "same inputs should produce same output")
	})

	t.Run("zero-length salt", func(t *testing.T) {
		mHash := make([]byte, sha384Size)
		em, err := emsaPSSEncodeWithSalt(mHash, emBits, nil)
		require.NoError(t, err)

		emLen := (emBits + 7) / 8
		assert.Len(t, em, emLen)
		assert.Equal(t, byte(0xBC), em[len(em)-1])
	})
}

func TestI2OSP(t *testing.T) {
	t.Run("zero value", func(t *testing.T) {
		result, err := i2osp(big.NewInt(0), 4)
		require.NoError(t, err)
		assert.Equal(t, []byte{0, 0, 0, 0}, result)
	})

	t.Run("small value with padding", func(t *testing.T) {
		result, err := i2osp(big.NewInt(256), 4)
		require.NoError(t, err)
		assert.Equal(t, []byte{0, 0, 1, 0}, result)
	})

	t.Run("value too large", func(t *testing.T) {
		_, err := i2osp(big.NewInt(256), 1)
		assert.ErrorIs(t, err, ErrMessageTooLong)
	})

	t.Run("negative value", func(t *testing.T) {
		_, err := i2osp(big.NewInt(-1), 4)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("exact fit", func(t *testing.T) {
		result, err := i2osp(big.NewInt(255), 1)
		require.NoError(t, err)
		assert.Equal(t, []byte{255}, result)
	})
}

func TestOS2IP(t *testing.T) {
	t.Run("zero bytes", func(t *testing.T) {
		result := os2ip([]byte{0, 0, 0})
		assert.Equal(t, 0, result.Sign())
	})

	t.Run("round trip with i2osp", func(t *testing.T) {
		original := big.NewInt(123456789)
		encoded, err := i2osp(original, 8)
		require.NoError(t, err)

		decoded := os2ip(encoded)
		assert.Equal(t, original, decoded)
	})
}

func TestMgf1SHA384(t *testing.T) {
	seed := []byte("test seed for mgf1")

	t.Run("produces requested length", func(t *testing.T) {
		for _, length := range []int{1, 48, 100, 256} {
			mask := mgf1SHA384(seed, length)
			assert.Len(t, mask, length)
		}
	})

	t.Run("deterministic for same seed", func(t *testing.T) {
		mask1 := mgf1SHA384(seed, 100)
		mask2 := mgf1SHA384(seed, 100)
		assert.Equal(t, mask1, mask2)
	})

	t.Run("different seeds produce different output", func(t *testing.T) {
		mask1 := mgf1SHA384([]byte("seed1"), 100)
		mask2 := mgf1SHA384([]byte("seed2"), 100)
		assert.NotEqual(t, mask1, mask2)
	})
}
