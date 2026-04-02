package securecookie

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"time"
)

const (
	// DefaultMaxAge is the default maximum cookie age: 30 days.
	DefaultMaxAge = 30 * 24 * 60 * 60

	// DefaultMaxLength is the default maximum encoded cookie length in bytes.
	DefaultMaxLength = 4096

	// maxDecompressSize is the maximum allowed decompressed payload size.
	// This prevents a small compressed cookie from expanding into an
	// arbitrarily large plaintext (zip-bomb defense).
	maxDecompressSize = 512 * 1024 // 512 KB

	// timestampSize is the size of the Unix timestamp prefix in bytes.
	timestampSize = 8

	// defaultFutureSkew is the maximum allowed clock skew for future
	// timestamps, in seconds. Cookies with timestamps more than this
	// far in the future are rejected.
	defaultFutureSkew = 5 * 60

	// Payload compression prefix bytes.
	prefixRaw      = 0x00
	prefixDeflated = 0x01
)

// Codec encodes and decodes cookie values.
type Codec interface {
	Encode(name string, value any) (string, error)
	Decode(name string, value string, dst any) error
}

// SecureCookie encodes and decodes authenticated, encrypted cookie values
// using AES-GCM (AES-128, AES-192, or AES-256 depending on key size).
type SecureCookie struct {
	gcm        cipher.AEAD
	maxAge     int
	minAge     int
	maxLength  int
	serializer Serializer
	now        func() time.Time
	randReader io.Reader
	aad        []byte // custom additional authenticated data
	aadSet     bool   // true when AdditionalData has been called with non-nil
}

// New creates a [SecureCookie] with the given AES key.
// The key must be 16 bytes (AES-128), 24 bytes (AES-192), or 32 bytes (AES-256).
func New(key []byte) (*SecureCookie, error) {
	if !validKeySize(len(key)) {
		return nil, ErrInvalidKey
	}

	// aes.NewCipher cannot fail with a valid key size (16, 24, or 32 bytes).
	block, _ := aes.NewCipher(key)

	// cipher.NewGCM cannot fail on a standard AES block cipher.
	gcm, _ := cipher.NewGCM(block)

	return &SecureCookie{
		gcm:        gcm,
		maxAge:     DefaultMaxAge,
		maxLength:  DefaultMaxLength,
		serializer: JSONSerializer{},
		now:        time.Now,
		randReader: rand.Reader,
	}, nil
}

// MaxAge sets the maximum age in seconds. Cookies older than this are
// rejected during decode. Set to 0 to disable age checking.
// Negative values are treated as 0.
func (s *SecureCookie) MaxAge(seconds int) *SecureCookie {
	if s == nil {
		return s
	}

	if seconds < 0 {
		seconds = 0
	}

	s.maxAge = seconds

	return s
}

// MinAge sets the minimum age in seconds. Cookies newer than this are
// rejected during decode. Default: 0 (no minimum).
// Negative values are treated as 0.
func (s *SecureCookie) MinAge(seconds int) *SecureCookie {
	if s == nil {
		return s
	}

	if seconds < 0 {
		seconds = 0
	}

	s.minAge = seconds

	return s
}

// MaxLength sets the maximum encoded cookie value length in bytes.
// Set to 0 to disable length checking.
// Negative values are treated as 0.
func (s *SecureCookie) MaxLength(length int) *SecureCookie {
	if s == nil {
		return s
	}

	if length < 0 {
		length = 0
	}

	s.maxLength = length

	return s
}

// SetSerializer sets the serializer used for encoding and decoding values.
// A nil serializer is ignored. Default: [JSONSerializer].
func (s *SecureCookie) SetSerializer(sz Serializer) *SecureCookie {
	if s == nil {
		return s
	}

	// Reject both untyped-nil and typed-nil (e.g., var sz *JSONSerializer).
	if sz == nil {
		return s
	}

	if v := reflect.ValueOf(sz); v.Kind() == reflect.Ptr && v.IsNil() {
		return s
	}

	s.serializer = sz

	return s
}

// AdditionalData sets custom additional authenticated data (AAD) bound into
// the GCM authentication tag. When set, this replaces the default behavior
// of using the cookie name as AAD.
//
//   - Custom data: binds cookies to context such as a user ID or session ID.
//   - Empty slice: disables AAD entirely (no binding).
//   - Nil: reverts to the default (cookie name as AAD).
func (s *SecureCookie) AdditionalData(data []byte) *SecureCookie {
	if s == nil {
		return s
	}

	if data == nil {
		s.aad = nil
		s.aadSet = false
	} else {
		s.aad = make([]byte, len(data))
		copy(s.aad, data)
		s.aadSet = true
	}

	return s
}

func (s *SecureCookie) buildAAD(name string) []byte {
	if s.aadSet {
		return s.aad
	}

	return []byte(name)
}

// Encode serializes, encrypts, and base64-encodes a value. By default the
// cookie name is bound as additional authenticated data (AAD) to prevent
// value transplant. Use [SecureCookie.AdditionalData] to override.
func (s *SecureCookie) Encode(name string, value any) (string, error) {
	if s == nil || s.gcm == nil {
		return "", ErrEncodeFailed
	}

	plaintext, err := s.serializer.Serialize(value)
	if err != nil {
		return "", fmt.Errorf("%w: serialization: %s", ErrEncodeFailed, err)
	}

	// Try deflate compression; keep only if strictly smaller.
	compressed := maybeCompress(plaintext)

	// Prepend timestamp to payload.
	ts := make([]byte, timestampSize)
	binary.BigEndian.PutUint64(ts, uint64(s.now().Unix()))

	payload := make([]byte, 0, timestampSize+len(compressed))
	payload = append(payload, ts...)
	payload = append(payload, compressed...)

	// Encrypt with cookie name as AAD.
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(s.randReader, nonce); err != nil {
		return "", fmt.Errorf("%w: nonce generation: %s", ErrEncodeFailed, err)
	}

	ciphertext := s.gcm.Seal(nonce, nonce, payload, s.buildAAD(name))

	encoded := base64.RawURLEncoding.EncodeToString(ciphertext)

	if s.maxLength > 0 && len(encoded) > s.maxLength {
		return "", ErrValueTooLong
	}

	return encoded, nil
}

// Decode base64-decodes, decrypts, validates the timestamp, and deserializes
// a cookie value into dst.
func (s *SecureCookie) Decode(name string, value string, dst any) error {
	if s == nil || s.gcm == nil {
		return ErrDecodeFailed
	}

	if s.maxLength > 0 && len(value) > s.maxLength {
		return ErrValueTooLong
	}

	ciphertext, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("%w: base64: %s", ErrDecodeFailed, err)
	}

	nonceSize := s.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return fmt.Errorf("%w: ciphertext too short", ErrDecodeFailed)
	}

	nonce := ciphertext[:nonceSize]
	payload, err := s.gcm.Open(nil, nonce, ciphertext[nonceSize:], s.buildAAD(name))
	if err != nil {
		return fmt.Errorf("%w: decryption failed", ErrDecodeFailed)
	}

	if len(payload) < timestampSize {
		return fmt.Errorf("%w: payload too short", ErrDecodeFailed)
	}

	// Validate timestamp.
	ts := int64(binary.BigEndian.Uint64(payload[:timestampSize]))
	now := s.now().Unix()

	// Reject cookies with timestamps in the future (beyond clock skew tolerance).
	if ts > now+defaultFutureSkew {
		return ErrTimestampFuture
	}

	if s.maxAge > 0 {
		age := now - ts
		if age > int64(s.maxAge) {
			return ErrTimestampExpired
		}
	}

	if s.minAge > 0 {
		age := now - ts
		if age < int64(s.minAge) {
			return ErrTimestampTooNew
		}
	}

	data, err := maybeDecompress(payload[timestampSize:])
	if err != nil {
		return fmt.Errorf("%w: decompression: %s", ErrDecodeFailed, err)
	}

	if err := s.serializer.Deserialize(data, dst); err != nil {
		return fmt.Errorf("%w: deserialization: %s", ErrDecodeFailed, err)
	}

	return nil
}

// cryptoRand is the random reader used by GenerateKey. Tests may override it.
var cryptoRand io.Reader = rand.Reader

// GenerateKey returns a cryptographically random key suitable for use
// with [New]. The size must be 16 (AES-128), 24 (AES-192), or 32 (AES-256).
func GenerateKey(size int) ([]byte, error) {
	if !validKeySize(size) {
		return nil, ErrInvalidKey
	}

	key := make([]byte, size)

	if _, err := io.ReadFull(cryptoRand, key); err != nil {
		return nil, fmt.Errorf("%w: random key generation: %s", ErrInvalidKey, err)
	}

	return key, nil
}

func validKeySize(n int) bool {
	return n == 16 || n == 24 || n == 32
}

// CodecsFromKeys creates a [Codec] slice from one or more AES keys.
// Each key must be 16, 24, or 32 bytes.
// The first key is used for encoding; all keys are tried for decoding.
func CodecsFromKeys(keys ...[]byte) ([]Codec, error) {
	if len(keys) == 0 {
		return nil, ErrNoCodecs
	}

	codecs := make([]Codec, 0, len(keys))

	for i, key := range keys {
		sc, err := New(key)
		if err != nil {
			return nil, fmt.Errorf("key[%d]: %w", i, err)
		}

		codecs = append(codecs, sc)
	}

	return codecs, nil
}

// EncodeMulti encodes a value using the first codec in the list.
func EncodeMulti(name string, value any, codecs ...Codec) (string, error) {
	if len(codecs) == 0 {
		return "", ErrNoCodecs
	}

	if isNilCodec(codecs[0]) {
		return "", ErrEncodeFailed
	}

	return codecs[0].Encode(name, value)
}

// DecodeMulti tries each codec in order and returns the first successful decode.
// Returns the last error if all codecs fail.
//
// For [SecureCookie] codecs (produced by [CodecsFromKeys]), dst is only
// written on a fully successful decode because GCM decryption and timestamp
// validation occur before deserialization. Custom [Codec] implementations
// should follow the same pattern: validate before writing to dst.
func DecodeMulti(name string, value string, dst any, codecs ...Codec) error {
	if len(codecs) == 0 {
		return ErrNoCodecs
	}

	var lastErr error

	for _, c := range codecs {
		if isNilCodec(c) {
			lastErr = ErrDecodeFailed
			continue
		}

		if err := c.Decode(name, value, dst); err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return lastErr
}

// isNilCodec returns true if c is a nil interface or a typed-nil pointer.
func isNilCodec(c Codec) bool {
	if c == nil {
		return true
	}

	v := reflect.ValueOf(c)

	return v.Kind() == reflect.Ptr && v.IsNil()
}
