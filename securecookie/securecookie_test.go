package securecookie

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKey() []byte {
	return []byte("01234567890123456789012345678901") // 32 bytes
}

func TestNew(t *testing.T) {
	t.Run("AES-128", func(t *testing.T) {
		sc, err := New(make([]byte, 16))
		require.NoError(t, err)
		assert.NotNil(t, sc)
	})

	t.Run("AES-192", func(t *testing.T) {
		sc, err := New(make([]byte, 24))
		require.NoError(t, err)
		assert.NotNil(t, sc)
	})

	t.Run("AES-256", func(t *testing.T) {
		sc, err := New(testKey())
		require.NoError(t, err)
		assert.NotNil(t, sc)
	})

	t.Run("short key", func(t *testing.T) {
		_, err := New([]byte("short"))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("long key", func(t *testing.T) {
		_, err := New(make([]byte, 64))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("nil key", func(t *testing.T) {
		_, err := New(nil)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("17 bytes rejected", func(t *testing.T) {
		_, err := New(make([]byte, 17))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
}

func TestEncodeDecodeAllKeySizes(t *testing.T) {
	for _, size := range []int{16, 24, 32} {
		t.Run(fmt.Sprintf("AES-%d", size*8), func(t *testing.T) {
			key := make([]byte, size)
			for i := range key {
				key[i] = byte(i)
			}

			sc, err := New(key)
			require.NoError(t, err)

			encoded, err := sc.Encode("hello")
			require.NoError(t, err)

			var dst string
			err = sc.Decode(encoded, &dst)
			require.NoError(t, err)
			assert.Equal(t, "hello", dst)
		})
	}
}

func TestEncodeDecode(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	t.Run("round trip string", func(t *testing.T) {
		encoded, err := sc.Encode("hello world")
		require.NoError(t, err)
		assert.NotEmpty(t, encoded)

		var dst string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "hello world", dst)
	})

	t.Run("round trip struct", func(t *testing.T) {
		type data struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}

		src := data{Name: "test", Count: 42}
		encoded, err := sc.Encode(src)
		require.NoError(t, err)

		var dst data
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, src, dst)
	})

	t.Run("round trip map", func(t *testing.T) {
		src := map[string]string{"key": "value", "foo": "bar"}
		encoded, err := sc.Encode(src)
		require.NoError(t, err)

		var dst map[string]string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, src, dst)
	})
}

func TestNameNotBoundByDefault(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	encoded, err := sc.Encode("secret")
	require.NoError(t, err)

	// By default no AAD is used, so any name works.
	var dst string
	err = sc.Decode(encoded, &dst)
	require.NoError(t, err)
	assert.Equal(t, "secret", dst)
}

func TestAdditionalData(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	t.Run("custom AAD round trip", func(t *testing.T) {
		sc.AdditionalData([]byte("user-123"))

		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		var dst string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})

	t.Run("same AAD round trip", func(t *testing.T) {
		sc.AdditionalData([]byte("user-123"))

		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		var dst string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})

	t.Run("wrong custom AAD fails", func(t *testing.T) {
		sc.AdditionalData([]byte("user-123"))
		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		sc.AdditionalData([]byte("user-456"))
		var dst string
		err = sc.Decode(encoded, &dst)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})

	t.Run("clear AAD with nil", func(t *testing.T) {
		sc.AdditionalData(nil)

		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		// No AAD, so any name works.
		var dst string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})

	t.Run("disabled AAD with empty slice", func(t *testing.T) {
		sc.AdditionalData([]byte{})

		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		// Empty AAD works the same as no AAD.
		var dst string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})

	t.Run("caller mutation does not affect AAD", func(t *testing.T) {
		sc.AdditionalData(nil) // reset to default first

		aad := []byte("user-123")
		sc.AdditionalData(aad)

		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		// Mutate the original slice after setting AAD.
		aad[0] = 'X'

		// Decode should still work because AAD was copied.
		var dst string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})

	t.Run("nil receiver", func(t *testing.T) {
		var nilSC *SecureCookie
		assert.Nil(t, nilSC.AdditionalData([]byte("data")))
	})
}

func TestDifferentKeys(t *testing.T) {
	key1 := []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	key2 := []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")

	sc1, err := New(key1)
	require.NoError(t, err)

	sc2, err := New(key2)
	require.NoError(t, err)

	encoded, err := sc1.Encode("data")
	require.NoError(t, err)

	var dst string
	err = sc2.Decode(encoded, &dst)
	assert.ErrorIs(t, err, ErrDecodeFailed)
}

func TestMaxAge(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	now := time.Now()
	sc.now = func() time.Time { return now }
	sc.MaxAge(60) // 60 seconds

	encoded, err := sc.Encode("data")
	require.NoError(t, err)

	t.Run("fresh cookie accepted", func(t *testing.T) {
		sc.now = func() time.Time { return now.Add(30 * time.Second) }
		var dst string
		err := sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})

	t.Run("expired cookie rejected", func(t *testing.T) {
		sc.now = func() time.Time { return now.Add(61 * time.Second) }
		var dst string
		err := sc.Decode(encoded, &dst)
		assert.ErrorIs(t, err, ErrTimestampExpired)
	})

	t.Run("disabled max age", func(t *testing.T) {
		sc.MaxAge(0)
		sc.now = func() time.Time { return now.Add(999 * time.Hour) }
		var dst string
		err := sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})
}

func TestFutureTimestamp(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	now := time.Now()
	sc.now = func() time.Time { return now }
	sc.MaxAge(0) // disable max age to isolate future check

	t.Run("within clock skew accepted", func(t *testing.T) {
		// Encode at now+3m (within 5m tolerance).
		sc.now = func() time.Time { return now.Add(3 * time.Minute) }
		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		// Decode at now (3m before the timestamp).
		sc.now = func() time.Time { return now }
		var dst string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})

	t.Run("far future rejected", func(t *testing.T) {
		// Encode at now+2h.
		sc.now = func() time.Time { return now.Add(2 * time.Hour) }
		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		// Decode at now (2h before the timestamp).
		sc.now = func() time.Time { return now }
		var dst string
		err = sc.Decode(encoded, &dst)
		assert.ErrorIs(t, err, ErrTimestampFuture)
	})

	t.Run("exactly at skew boundary accepted", func(t *testing.T) {
		// Encode at now+5m (exactly at tolerance).
		sc.now = func() time.Time { return now.Add(5 * time.Minute) }
		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		// Decode at now.
		sc.now = func() time.Time { return now }
		var dst string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})

	t.Run("one second past skew rejected", func(t *testing.T) {
		// Encode at now+5m1s (just past tolerance).
		sc.now = func() time.Time { return now.Add(5*time.Minute + time.Second) }
		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		// Decode at now.
		sc.now = func() time.Time { return now }
		var dst string
		err = sc.Decode(encoded, &dst)
		assert.ErrorIs(t, err, ErrTimestampFuture)
	})
}

func TestMinAge(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	now := time.Now()
	sc.now = func() time.Time { return now }
	sc.MaxAge(0) // disable max age
	sc.MinAge(10)

	encoded, err := sc.Encode("data")
	require.NoError(t, err)

	t.Run("too new rejected", func(t *testing.T) {
		sc.now = func() time.Time { return now.Add(5 * time.Second) }
		var dst string
		err := sc.Decode(encoded, &dst)
		assert.ErrorIs(t, err, ErrTimestampTooNew)
	})

	t.Run("old enough accepted", func(t *testing.T) {
		sc.now = func() time.Time { return now.Add(11 * time.Second) }
		var dst string
		err := sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "data", dst)
	})
}

func TestMaxLength(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)
	sc.MaxLength(10)

	t.Run("encode rejects too long", func(t *testing.T) {
		_, err := sc.Encode("this is a very long string that will produce a long encoded value")
		assert.ErrorIs(t, err, ErrValueTooLong)
	})

	t.Run("decode rejects too long", func(t *testing.T) {
		err := sc.Decode("aaaaaaaaaaaaaaaaaaa", nil)
		assert.ErrorIs(t, err, ErrValueTooLong)
	})

	t.Run("disabled length check", func(t *testing.T) {
		sc.MaxLength(0)
		encoded, err := sc.Encode("this is a very long string")
		require.NoError(t, err)

		var dst string
		err = sc.Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "this is a very long string", dst)
	})
}

func TestNilReceiver(t *testing.T) {
	var sc *SecureCookie

	t.Run("encode", func(t *testing.T) {
		_, err := sc.Encode("data")
		assert.ErrorIs(t, err, ErrEncodeFailed)
	})

	t.Run("decode", func(t *testing.T) {
		err := sc.Decode("value", nil)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})

	t.Run("MaxAge", func(t *testing.T) {
		assert.Nil(t, sc.MaxAge(10))
	})

	t.Run("MinAge", func(t *testing.T) {
		assert.Nil(t, sc.MinAge(10))
	})

	t.Run("MaxLength", func(t *testing.T) {
		assert.Nil(t, sc.MaxLength(100))
	})

	t.Run("SetSerializer", func(t *testing.T) {
		assert.Nil(t, sc.SetSerializer(JSONSerializer{}))
	})
}

func TestZeroValueStruct(t *testing.T) {
	var sc SecureCookie

	t.Run("encode returns error", func(t *testing.T) {
		_, err := sc.Encode("data")
		assert.ErrorIs(t, err, ErrEncodeFailed)
	})

	t.Run("decode returns error", func(t *testing.T) {
		err := sc.Decode("value", nil)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})
}

func TestSetSerializerTypedNil(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	// Typed-nil serializer should be ignored.
	var sz *JSONSerializer
	sc.SetSerializer(sz)

	// Encode/Decode should still work with the default serializer.
	encoded, err := sc.Encode("hello")
	require.NoError(t, err)

	var dst string
	err = sc.Decode(encoded, &dst)
	require.NoError(t, err)
	assert.Equal(t, "hello", dst)
}

func TestDecodeInvalid(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	t.Run("invalid base64", func(t *testing.T) {
		err := sc.Decode("not!valid!base64!", nil)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})

	t.Run("too short ciphertext", func(t *testing.T) {
		err := sc.Decode("dG9v", nil) // "too" in base64
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})

	t.Run("corrupted ciphertext", func(t *testing.T) {
		encoded, err := sc.Encode("data")
		require.NoError(t, err)

		// Flip a byte in the middle.
		corrupted := []byte(encoded)
		corrupted[len(corrupted)/2] ^= 0xff
		err = sc.Decode(string(corrupted), nil)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})
}

func TestKeyRotation(t *testing.T) {
	key1 := []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	key2 := []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")

	codecs, err := CodecsFromKeys(key2, key1)
	require.NoError(t, err)

	t.Run("encode uses first key", func(t *testing.T) {
		encoded, err := EncodeMulti("new data", codecs...)
		require.NoError(t, err)

		// Should decode with first codec (key2).
		var dst string
		err = codecs[0].Decode(encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, "new data", dst)

		// Should fail with second codec (key1) alone.
		err = codecs[1].Decode(encoded, &dst)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})

	t.Run("decode tries old key", func(t *testing.T) {
		// Encode with old key only.
		oldEncoded, err := codecs[1].Encode("old data")
		require.NoError(t, err)

		// DecodeMulti should find it via second codec.
		var dst string
		err = DecodeMulti(oldEncoded, &dst, codecs...)
		require.NoError(t, err)
		assert.Equal(t, "old data", dst)
	})

	t.Run("all codecs fail", func(t *testing.T) {
		err := DecodeMulti("garbage", nil, codecs...)
		assert.Error(t, err)
	})
}

func TestCodecsFromKeys(t *testing.T) {
	t.Run("empty keys", func(t *testing.T) {
		_, err := CodecsFromKeys()
		assert.ErrorIs(t, err, ErrNoCodecs)
	})

	t.Run("invalid key in list", func(t *testing.T) {
		_, err := CodecsFromKeys(testKey(), []byte("short"))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
}

func TestEncodeMultiNilCodec(t *testing.T) {
	t.Run("untyped nil", func(t *testing.T) {
		var c Codec
		_, err := EncodeMulti("data", c)
		assert.ErrorIs(t, err, ErrEncodeFailed)
	})

	t.Run("typed nil", func(t *testing.T) {
		var c Codec = (*SecureCookie)(nil)
		_, err := EncodeMulti("data", c)
		assert.ErrorIs(t, err, ErrEncodeFailed)
	})
}

func TestDecodeMultiNilCodec(t *testing.T) {
	t.Run("all nil codecs", func(t *testing.T) {
		var c Codec
		var dst string
		err := DecodeMulti("data", &dst, c, c)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})

	t.Run("typed nil codec", func(t *testing.T) {
		var c Codec = (*SecureCookie)(nil)
		var dst string
		err := DecodeMulti("data", &dst, c)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})

	t.Run("nil codec followed by valid codec", func(t *testing.T) {
		sc, err := New(testKey())
		require.NoError(t, err)

		encoded, err := sc.Encode("hello")
		require.NoError(t, err)

		var c Codec
		var dst string
		err = DecodeMulti(encoded, &dst, c, sc)
		require.NoError(t, err)
		assert.Equal(t, "hello", dst)
	})

	t.Run("typed nil followed by valid codec", func(t *testing.T) {
		sc, err := New(testKey())
		require.NoError(t, err)

		encoded, err := sc.Encode("hello")
		require.NoError(t, err)

		var c Codec = (*SecureCookie)(nil)
		var dst string
		err = DecodeMulti(encoded, &dst, c, sc)
		require.NoError(t, err)
		assert.Equal(t, "hello", dst)
	})
}

func TestDecodeMultiDeserializeError(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	// Encode a string value.
	encoded, err := sc.Encode("not-a-struct")
	require.NoError(t, err)

	// Try to decode into an incompatible struct type.
	type strict struct {
		Field int `json:"field"`
	}

	var dst strict
	err = DecodeMulti(encoded, &dst, sc)
	// json.Unmarshal("not-a-struct" -> struct) fails.
	assert.ErrorIs(t, err, ErrDecodeFailed)
	assert.Contains(t, err.Error(), "deserialization")
}

func TestDecodeMultiDoesNotPoisonDst(t *testing.T) {
	key1 := []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	key2 := []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")

	sc1, err := New(key1)
	require.NoError(t, err)

	sc2, err := New(key2)
	require.NoError(t, err)

	// Encode with key2.
	encoded, err := sc2.Encode("correct")
	require.NoError(t, err)

	// DecodeMulti: first codec (key1) fails, second (key2) succeeds.
	// dst should only contain the result from the successful codec.
	var dst string
	err = DecodeMulti(encoded, &dst, sc1, sc2)
	require.NoError(t, err)
	assert.Equal(t, "correct", dst)
}

func TestEncodeDecodeMultiEmpty(t *testing.T) {
	t.Run("encode no codecs", func(t *testing.T) {
		_, err := EncodeMulti("data")
		assert.ErrorIs(t, err, ErrNoCodecs)
	})

	t.Run("decode no codecs", func(t *testing.T) {
		err := DecodeMulti("data", nil)
		assert.ErrorIs(t, err, ErrNoCodecs)
	})
}

func TestGenerateKey(t *testing.T) {
	for _, size := range []int{16, 24, 32} {
		t.Run(fmt.Sprintf("AES-%d", size*8), func(t *testing.T) {
			key, err := GenerateKey(size)
			require.NoError(t, err)
			assert.Len(t, key, size)
		})
	}

	t.Run("unique keys", func(t *testing.T) {
		key1, err := GenerateKey(32)
		require.NoError(t, err)

		key2, err := GenerateKey(32)
		require.NoError(t, err)

		assert.NotEqual(t, key1, key2)
	})

	t.Run("invalid size", func(t *testing.T) {
		_, err := GenerateKey(15)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("rand reader error", func(t *testing.T) {
		orig := cryptoRand
		cryptoRand = &errReader{}
		defer func() { cryptoRand = orig }()

		_, err := GenerateKey(32)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Contains(t, err.Error(), "random key generation")
	})
}

func TestSetSerializer(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	// Custom serializer that prepends a byte.
	sc.SetSerializer(&prefixSerializer{prefix: 0xAA})

	encoded, err := sc.Encode("hello")
	require.NoError(t, err)

	var dst string
	err = sc.Decode(encoded, &dst)
	require.NoError(t, err)
	assert.Equal(t, "hello", dst)
}

// prefixSerializer wraps JSONSerializer for testing custom serializers.
type prefixSerializer struct {
	prefix byte
	json   JSONSerializer
}

func (p *prefixSerializer) Serialize(src any) ([]byte, error) {
	data, err := p.json.Serialize(src)
	if err != nil {
		return nil, err
	}

	return append([]byte{p.prefix}, data...), nil
}

func (p *prefixSerializer) Deserialize(src []byte, dst any) error {
	if len(src) == 0 || src[0] != p.prefix {
		return ErrDecodeFailed
	}

	return p.json.Deserialize(src[1:], dst)
}

func TestBuilderChaining(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	result := sc.MaxAge(3600).MinAge(10).MaxLength(8192).SetSerializer(JSONSerializer{})
	assert.Same(t, sc, result)
	assert.Equal(t, 3600, sc.maxAge)
	assert.Equal(t, 10, sc.minAge)
	assert.Equal(t, 8192, sc.maxLength)
}

func TestNegativeValues(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	t.Run("negative MaxAge treated as 0", func(t *testing.T) {
		sc.MaxAge(-5)
		assert.Equal(t, 0, sc.maxAge)
	})

	t.Run("negative MinAge treated as 0", func(t *testing.T) {
		sc.MinAge(-10)
		assert.Equal(t, 0, sc.minAge)
	})

	t.Run("negative MaxLength treated as 0", func(t *testing.T) {
		sc.MaxLength(-1)
		assert.Equal(t, 0, sc.maxLength)
	})
}

func TestSetSerializerNil(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	// Nil serializer should be ignored, keeping the default.
	sc.SetSerializer(nil)

	encoded, err := sc.Encode("hello")
	require.NoError(t, err)

	var dst string
	err = sc.Decode(encoded, &dst)
	require.NoError(t, err)
	assert.Equal(t, "hello", dst)
}

func TestDecodeDeserializeErrorWrapped(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	// Encode a string value.
	encoded, err := sc.Encode("hello")
	require.NoError(t, err)

	// Try to decode into an incompatible type (int).
	var dst int
	err = sc.Decode(encoded, &dst)
	assert.ErrorIs(t, err, ErrDecodeFailed)
	assert.Contains(t, err.Error(), "deserialization")
}

func TestUniqueNonces(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	// Same value encoded twice should produce different ciphertext.
	e1, err := sc.Encode("same")
	require.NoError(t, err)

	e2, err := sc.Encode("same")
	require.NoError(t, err)

	assert.NotEqual(t, e1, e2)
}

func TestEncodeSerializerError(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)
	sc.SetSerializer(&failSerializer{})

	_, err = sc.Encode("data")
	assert.ErrorIs(t, err, ErrEncodeFailed)
	assert.Contains(t, err.Error(), "serialization")
}

// failSerializer always fails on Serialize.
type failSerializer struct{}

func (failSerializer) Serialize(_ any) ([]byte, error) {
	return nil, errors.New("serialize failed")
}

func (failSerializer) Deserialize(src []byte, dst any) error {
	return JSONSerializer{}.Deserialize(src, dst)
}

func TestEncodeNonceGenerationError(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)
	sc.randReader = &errReader{}

	_, err = sc.Encode("data")
	assert.ErrorIs(t, err, ErrEncodeFailed)
	assert.Contains(t, err.Error(), "nonce generation")
}

// errReader always returns an error on Read.
type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("entropy exhausted")
}

func TestDecodePayloadTooShort(t *testing.T) {
	// Craft a valid ciphertext that, when decrypted, yields a payload
	// shorter than the timestamp size (8 bytes).
	sc, err := New(testKey())
	require.NoError(t, err)

	// Bypass the normal Encode path by encrypting a short payload directly.
	nonce := make([]byte, sc.gcm.NonceSize())
	_, err = rand.Read(nonce)
	require.NoError(t, err)

	// Payload of only 4 bytes (less than 8-byte timestamp).
	shortPayload := []byte{0x01, 0x02, 0x03, 0x04}
	ciphertext := sc.gcm.Seal(nonce, nonce, shortPayload, nil)
	encoded := base64.RawURLEncoding.EncodeToString(ciphertext)

	var dst string
	err = sc.Decode(encoded, &dst)
	assert.ErrorIs(t, err, ErrDecodeFailed)
	assert.Contains(t, err.Error(), "payload too short")
}
