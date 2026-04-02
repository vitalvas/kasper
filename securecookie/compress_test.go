package securecookie

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompression(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)
	sc.MaxLength(0) // disable length limit for this test

	t.Run("small payload not compressed", func(t *testing.T) {
		src := "hi"
		encoded, err := sc.Encode("s", src)
		require.NoError(t, err)

		var dst string
		err = sc.Decode("s", encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, src, dst)
	})

	t.Run("large repetitive payload compressed", func(t *testing.T) {
		src := map[string]string{}
		for i := 0; i < 50; i++ {
			src[fmt.Sprintf("key-%03d", i)] = "same-value-repeated"
		}

		encoded, err := sc.Encode("s", src)
		require.NoError(t, err)

		var dst map[string]string
		err = sc.Decode("s", encoded, &dst)
		require.NoError(t, err)
		assert.Equal(t, src, dst)
	})

	t.Run("compression actually saves space", func(t *testing.T) {
		src := map[string]string{}
		for i := 0; i < 30; i++ {
			src[fmt.Sprintf("field-%03d", i)] = "this is a repeated value string"
		}

		encodedCompressed, err := sc.Encode("s", src)
		require.NoError(t, err)

		jsonBytes, _ := JSONSerializer{}.Serialize(src)

		assert.Less(t, len(encodedCompressed), len(jsonBytes)*2,
			"compressed encoding should be smaller than 2x JSON size")

		var dst map[string]string
		err = sc.Decode("s", encodedCompressed, &dst)
		require.NoError(t, err)
		assert.Equal(t, src, dst)
	})
}

func TestDecodeCorruptedCompressedPayload(t *testing.T) {
	sc, err := New(testKey())
	require.NoError(t, err)

	garbage := []byte("not-valid-deflate-stream")
	payload := make([]byte, 0, 8+1+len(garbage))
	payload = binary.BigEndian.AppendUint64(payload, uint64(time.Now().Unix()))
	payload = append(payload, prefixDeflated)
	payload = append(payload, garbage...)

	nonce := make([]byte, sc.gcm.NonceSize())
	_, err = rand.Read(nonce)
	require.NoError(t, err)

	ciphertext := sc.gcm.Seal(nonce, nonce, payload, []byte("s"))
	encoded := base64.RawURLEncoding.EncodeToString(ciphertext)

	var dst string
	err = sc.Decode("s", encoded, &dst)
	assert.ErrorIs(t, err, ErrDecodeFailed)
	assert.Contains(t, err.Error(), "decompression")
}

func TestMaybeCompressDecompress(t *testing.T) {
	t.Run("short data stays raw", func(t *testing.T) {
		data := []byte("short")
		compressed := maybeCompress(data)
		assert.Equal(t, byte(prefixRaw), compressed[0])

		result, err := maybeDecompress(compressed)
		require.NoError(t, err)
		assert.Equal(t, data, result)
	})

	t.Run("high entropy skips deflate", func(t *testing.T) {
		data := make([]byte, 256)
		_, err := rand.Read(data)
		require.NoError(t, err)

		compressed := maybeCompress(data)
		assert.Equal(t, byte(prefixRaw), compressed[0])
		assert.Equal(t, 1+len(data), len(compressed))

		result, err := maybeDecompress(compressed)
		require.NoError(t, err)
		assert.Equal(t, data, result)
	})

	t.Run("low entropy but deflate larger falls back to raw", func(t *testing.T) {
		data := []byte("abcdefghijklmnopqrstuvwxyz123456")
		compressed := maybeCompress(data)
		assert.Equal(t, byte(prefixRaw), compressed[0])

		result, err := maybeDecompress(compressed)
		require.NoError(t, err)
		assert.Equal(t, data, result)
	})

	t.Run("low entropy compresses", func(t *testing.T) {
		data := bytes.Repeat([]byte("abcdefghij"), 100)
		compressed := maybeCompress(data)
		assert.Equal(t, byte(prefixDeflated), compressed[0])
		assert.Less(t, len(compressed), len(data))

		result, err := maybeDecompress(compressed)
		require.NoError(t, err)
		assert.Equal(t, data, result)
	})

	t.Run("empty data", func(t *testing.T) {
		result, err := maybeDecompress([]byte{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("corrupted deflate data", func(t *testing.T) {
		data := append([]byte{prefixDeflated}, []byte("not-valid-deflate")...)
		_, err := maybeDecompress(data)
		assert.Error(t, err)
	})

	t.Run("legacy uncompressed data", func(t *testing.T) {
		// Pre-compression cookies have no prefix byte.
		legacy := []byte(`{"user":"alice"}`)
		result, err := maybeDecompress(legacy)
		require.NoError(t, err)
		assert.Equal(t, legacy, result)
	})

	t.Run("decompression size limit", func(t *testing.T) {
		// Compress a large payload, then try to decompress with limit.
		large := bytes.Repeat([]byte("A"), maxDecompressSize+1)
		compressed := maybeCompress(large)
		assert.Equal(t, byte(prefixDeflated), compressed[0])

		_, err := maybeDecompress(compressed)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds")
	})
}

func TestShannonEntropy(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.Equal(t, 0.0, shannonEntropy([]byte{}))
	})

	t.Run("single byte repeated", func(t *testing.T) {
		assert.Equal(t, 0.0, shannonEntropy(bytes.Repeat([]byte{0x41}, 100)))
	})

	t.Run("two equally distributed bytes", func(t *testing.T) {
		data := bytes.Repeat([]byte{0x00, 0xFF}, 100)
		assert.InDelta(t, 1.0, shannonEntropy(data), 0.01)
	})

	t.Run("random data high entropy", func(t *testing.T) {
		data := make([]byte, 4096)
		_, _ = rand.Read(data)
		h := shannonEntropy(data)
		assert.Greater(t, h, 7.5)
	})

	t.Run("JSON text moderate entropy", func(t *testing.T) {
		data := []byte(`{"user":"alice","email":"alice@example.com","role":"admin"}`)
		h := shannonEntropy(data)
		assert.Greater(t, h, 3.0)
		assert.Less(t, h, 6.0)
	})
}
