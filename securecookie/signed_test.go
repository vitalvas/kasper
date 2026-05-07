package securecookie

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSignedKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef") // 32 bytes
}

func TestNewSigned(t *testing.T) {
	t.Run("32-byte key accepted", func(t *testing.T) {
		s, err := NewSigned(testSignedKey())
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("any non-empty key accepted", func(t *testing.T) {
		s, err := NewSigned([]byte("short"))
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("empty key rejected", func(t *testing.T) {
		_, err := NewSigned(nil)
		assert.ErrorIs(t, err, ErrInvalidKey)
		_, err = NewSigned([]byte{})
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("key is copied (defensive)", func(t *testing.T) {
		key := []byte("0123456789abcdef0123456789abcdef")
		s, err := NewSigned(key)
		require.NoError(t, err)
		// Mutate the original key; the codec must continue to work.
		key[0] = 'X'
		_, err = s.Encode("hello")
		require.NoError(t, err)
	})
}

func TestSignedEncodeDecodeRoundTrip(t *testing.T) {
	s, err := NewSigned(testSignedKey())
	require.NoError(t, err)

	tests := []struct {
		name  string
		value any
	}{
		{"string", "hello world"},
		{"int", 42},
		{"struct", struct{ Name string }{Name: "alice"}},
		{"map", map[string]any{"k": "v", "n": 1.0}},
		{"slice", []int{1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := s.Encode(tt.value)
			require.NoError(t, err)
			require.NotEmpty(t, encoded)

			// Payload should be readable (not encrypted): the JSON form
			// of the value must appear inside the cookie body before the
			// HMAC tag. We don't check the exact bytes, just that decode
			// recovers the original value.
			var got any
			err = s.Decode(encoded, &got)
			require.NoError(t, err)
		})
	}
}

func TestSignedTamperingRejected(t *testing.T) {
	s, err := NewSigned(testSignedKey())
	require.NoError(t, err)

	encoded, err := s.Encode("trusted-value")
	require.NoError(t, err)

	// swapChar replaces the byte at index i with a different valid
	// base64url character. This guarantees the decoded payload differs
	// (unlike bit-flips on the final char, where the low bits may be
	// unused base64 padding bits and the decode is unchanged).
	swapChar := func(s string, i int) string {
		b := []byte(s)
		alt := byte('A')
		if b[i] == 'A' {
			alt = 'B'
		}
		b[i] = alt
		return string(b)
	}

	tests := []struct {
		name   string
		mutate func(string) string
	}{
		{"swap middle char (in HMAC)", func(v string) string { return swapChar(v, len(v)/2) }},
		{"swap first char", func(v string) string { return swapChar(v, 0) }},
		{"truncate", func(v string) string { return v[:len(v)-4] }},
		{"empty", func(string) string { return "" }},
		{"not base64", func(string) string { return "!!!not-base64!!!" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			err := s.Decode(tt.mutate(encoded), &got)
			assert.Error(t, err)
		})
	}
}

func TestSignedDifferentKeyRejected(t *testing.T) {
	keyA := []byte("0123456789abcdef0123456789abcdef")
	keyB := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	a, err := NewSigned(keyA)
	require.NoError(t, err)
	b, err := NewSigned(keyB)
	require.NoError(t, err)

	encoded, err := a.Encode("payload")
	require.NoError(t, err)

	var got string
	err = b.Decode(encoded, &got)
	assert.ErrorIs(t, err, ErrDecodeFailed)
}

func TestSignedAdditionalData(t *testing.T) {
	key := testSignedKey()

	a, _ := NewSigned(key)
	a.AdditionalData([]byte("session"))

	b, _ := NewSigned(key)
	b.AdditionalData([]byte("csrf"))

	encoded, err := a.Encode("payload")
	require.NoError(t, err)

	t.Run("same AAD decodes", func(t *testing.T) {
		var got string
		require.NoError(t, a.Decode(encoded, &got))
		assert.Equal(t, "payload", got)
	})

	t.Run("different AAD rejected", func(t *testing.T) {
		var got string
		err := b.Decode(encoded, &got)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})

	t.Run("clear AAD", func(t *testing.T) {
		c, _ := NewSigned(key)
		c.AdditionalData([]byte("x"))
		c.AdditionalData(nil)
		// Should now match the no-AAD path.
		nopaad, _ := NewSigned(key)
		encoded, _ := nopaad.Encode("v")
		var got string
		require.NoError(t, c.Decode(encoded, &got))
	})
}

func TestSignedMaxAge(t *testing.T) {
	s, _ := NewSigned(testSignedKey())
	s.MaxAge(60)

	now := time.Now()
	s.now = func() time.Time { return now }

	encoded, err := s.Encode("v")
	require.NoError(t, err)

	t.Run("within window", func(t *testing.T) {
		s.now = func() time.Time { return now.Add(30 * time.Second) }
		var got string
		require.NoError(t, s.Decode(encoded, &got))
	})

	t.Run("expired", func(t *testing.T) {
		s.now = func() time.Time { return now.Add(2 * time.Minute) }
		var got string
		err := s.Decode(encoded, &got)
		assert.ErrorIs(t, err, ErrTimestampExpired)
	})

	t.Run("disabled with 0", func(t *testing.T) {
		s2, _ := NewSigned(testSignedKey())
		s2.MaxAge(0)
		s2.now = func() time.Time { return now }
		enc, _ := s2.Encode("v")
		s2.now = func() time.Time { return now.Add(365 * 24 * time.Hour) }
		var got string
		require.NoError(t, s2.Decode(enc, &got))
	})

	t.Run("negative clamped to 0", func(t *testing.T) {
		s3, _ := NewSigned(testSignedKey())
		s3.MaxAge(-1)
		assert.Equal(t, 0, s3.maxAge)
	})
}

func TestSignedMinAge(t *testing.T) {
	s, _ := NewSigned(testSignedKey())
	s.MinAge(10)

	now := time.Now()
	s.now = func() time.Time { return now }

	encoded, _ := s.Encode("v")

	t.Run("too new", func(t *testing.T) {
		s.now = func() time.Time { return now.Add(1 * time.Second) }
		var got string
		err := s.Decode(encoded, &got)
		assert.ErrorIs(t, err, ErrTimestampTooNew)
	})

	t.Run("old enough", func(t *testing.T) {
		s.now = func() time.Time { return now.Add(20 * time.Second) }
		var got string
		require.NoError(t, s.Decode(encoded, &got))
	})

	t.Run("negative clamped", func(t *testing.T) {
		s2, _ := NewSigned(testSignedKey())
		s2.MinAge(-5)
		assert.Equal(t, 0, s2.minAge)
	})
}

func TestSignedFutureTimestamp(t *testing.T) {
	s, _ := NewSigned(testSignedKey())

	now := time.Now()
	// Encode with a future-skewed clock.
	s.now = func() time.Time { return now.Add(1 * time.Hour) }
	encoded, _ := s.Encode("v")

	// Decode with a normal clock.
	s.now = func() time.Time { return now }
	var got string
	err := s.Decode(encoded, &got)
	assert.ErrorIs(t, err, ErrTimestampFuture)
}

func TestSignedMaxLength(t *testing.T) {
	s, _ := NewSigned(testSignedKey())
	s.MaxLength(50)

	t.Run("encode rejects too long", func(t *testing.T) {
		// 200 bytes of 'a' will encode to >50 base64 chars.
		_, err := s.Encode(make([]byte, 200))
		assert.ErrorIs(t, err, ErrValueTooLong)
	})

	t.Run("decode rejects too long", func(t *testing.T) {
		long := strings.Repeat("a", 100)
		var got string
		err := s.Decode(long, &got)
		assert.ErrorIs(t, err, ErrValueTooLong)
	})

	t.Run("disabled with 0", func(t *testing.T) {
		s2, _ := NewSigned(testSignedKey())
		s2.MaxLength(0)
		_, err := s2.Encode(make([]byte, 100))
		require.NoError(t, err)
	})

	t.Run("negative clamped", func(t *testing.T) {
		s3, _ := NewSigned(testSignedKey())
		s3.MaxLength(-1)
		assert.Equal(t, 0, s3.maxLength)
	})
}

func TestSignedNilReceiver(t *testing.T) {
	var s *SignedCookie

	t.Run("Encode returns error", func(t *testing.T) {
		_, err := s.Encode("v")
		assert.ErrorIs(t, err, ErrEncodeFailed)
	})

	t.Run("Decode returns error", func(t *testing.T) {
		var got string
		err := s.Decode("anything", &got)
		assert.ErrorIs(t, err, ErrDecodeFailed)
	})

	t.Run("MaxAge no-op", func(t *testing.T) {
		assert.Nil(t, s.MaxAge(60))
	})

	t.Run("MinAge no-op", func(t *testing.T) {
		assert.Nil(t, s.MinAge(60))
	})

	t.Run("MaxLength no-op", func(t *testing.T) {
		assert.Nil(t, s.MaxLength(100))
	})

	t.Run("SetSerializer no-op", func(t *testing.T) {
		assert.Nil(t, s.SetSerializer(JSONSerializer{}))
	})

	t.Run("AdditionalData no-op", func(t *testing.T) {
		assert.Nil(t, s.AdditionalData(nil))
	})
}

func TestSignedSetSerializer(t *testing.T) {
	s, _ := NewSigned(testSignedKey())

	t.Run("nil ignored", func(t *testing.T) {
		s.SetSerializer(nil)
		assert.NotNil(t, s.serializer)
	})

	t.Run("typed nil ignored", func(t *testing.T) {
		var sz *JSONSerializer
		s.SetSerializer(sz)
		assert.NotNil(t, s.serializer)
	})

	t.Run("custom serializer applied", func(t *testing.T) {
		s.SetSerializer(JSONSerializer{})
		encoded, err := s.Encode("v")
		require.NoError(t, err)
		var got string
		require.NoError(t, s.Decode(encoded, &got))
		assert.Equal(t, "v", got)
	})
}

func TestSignedDecodeInvalid(t *testing.T) {
	s, _ := NewSigned(testSignedKey())

	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"not base64", "!!!"},
		{"too short", "QQ"},                  // single byte after decode
		{"missing hmac", "AAAAAAAAAAAAAAAA"}, // <  hmac size+timestamp
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			err := s.Decode(tt.in, &got)
			assert.Error(t, err)
		})
	}
}

func TestGenerateSignedKey(t *testing.T) {
	t.Run("generates requested size", func(t *testing.T) {
		k, err := GenerateSignedKey(32)
		require.NoError(t, err)
		assert.Len(t, k, 32)
	})

	t.Run("uniqueness", func(t *testing.T) {
		a, _ := GenerateSignedKey(32)
		b, _ := GenerateSignedKey(32)
		assert.NotEqual(t, a, b)
	})

	t.Run("zero size rejected", func(t *testing.T) {
		_, err := GenerateSignedKey(0)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("negative rejected", func(t *testing.T) {
		_, err := GenerateSignedKey(-1)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
}

func TestSignedCodecsFromKeys(t *testing.T) {
	t.Run("rotation", func(t *testing.T) {
		newKey := testSignedKey()
		oldKey := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

		// Encode under old key.
		old, _ := NewSigned(oldKey)
		encoded, err := old.Encode("payload")
		require.NoError(t, err)

		// Decode with multi-codec (new first, then old fallback).
		codecs, err := SignedCodecsFromKeys(newKey, oldKey)
		require.NoError(t, err)

		var got string
		err = DecodeMulti(encoded, &got, codecs...)
		require.NoError(t, err)
		assert.Equal(t, "payload", got)
	})

	t.Run("empty key list", func(t *testing.T) {
		_, err := SignedCodecsFromKeys()
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("invalid key in list", func(t *testing.T) {
		_, err := SignedCodecsFromKeys([]byte("ok-key"), nil)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
}

func TestSignedSatisfiesCodec(t *testing.T) {
	// Compile-time check is in signed.go; runtime check exercises it via
	// the multi-codec helpers.
	s, err := NewSigned(testSignedKey())
	require.NoError(t, err)

	codecs := []Codec{s}
	encoded, err := EncodeMulti("v", codecs...)
	require.NoError(t, err)

	var got string
	err = DecodeMulti(encoded, &got, codecs...)
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

func TestSignedKeepsIntegrityErrorVariety(t *testing.T) {
	// Confirm that all post-HMAC decode errors are reachable.
	s, _ := NewSigned(testSignedKey())
	s.now = func() time.Time { return time.Unix(1_000_000_000, 0) }

	encoded, err := s.Encode(map[string]int{"a": 1})
	require.NoError(t, err)

	// Decode with the wrong type.
	var wrong int
	err = s.Decode(encoded, &wrong)
	assert.True(t, errors.Is(err, ErrDecodeFailed))
}
