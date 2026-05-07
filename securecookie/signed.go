package securecookie

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"time"
)

// hmacSize is the byte length of an HMAC-SHA256 tag.
const hmacSize = sha256.Size

// SignedCookie encodes and decodes authenticated (HMAC-SHA256) but
// unencrypted cookie values. The payload is integrity-protected against
// tampering but readable to anyone who can read the cookie.
//
// Use SignedCookie when the cookie payload is already opaque or
// non-sensitive: JWTs, OAuth state, opaque server-issued IDs, or
// anti-CSRF tokens. Prefer [SecureCookie] for any data that must remain
// secret from the client.
//
// SignedCookie avoids the AES key schedule on every request and uses a
// 32-byte HMAC tag instead of a 12-byte nonce + 16-byte auth tag, which
// can give smaller cookies for some payloads after compression.
type SignedCookie struct {
	hashKey    []byte
	maxAge     int
	minAge     int
	maxLength  int
	serializer Serializer
	now        func() time.Time
	aad        []byte
}

// NewSigned creates a [SignedCookie] with the given HMAC-SHA256 key.
// Any non-empty key is accepted; 32 bytes is recommended (RFC 2104 §3).
// Use [GenerateSignedKey] to produce a fresh key.
func NewSigned(key []byte) (*SignedCookie, error) {
	if len(key) == 0 {
		return nil, ErrInvalidKey
	}

	dup := make([]byte, len(key))
	copy(dup, key)

	return &SignedCookie{
		hashKey:    dup,
		maxAge:     DefaultMaxAge,
		maxLength:  DefaultMaxLength,
		serializer: JSONSerializer{},
		now:        time.Now,
	}, nil
}

// MaxAge sets the maximum age in seconds. Cookies older than this are
// rejected during decode. Set to 0 to disable age checking.
// Negative values are treated as 0.
func (s *SignedCookie) MaxAge(seconds int) *SignedCookie {
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
func (s *SignedCookie) MinAge(seconds int) *SignedCookie {
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
func (s *SignedCookie) MaxLength(length int) *SignedCookie {
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
func (s *SignedCookie) SetSerializer(sz Serializer) *SignedCookie {
	if s == nil {
		return s
	}

	if sz == nil {
		return s
	}

	if v := reflect.ValueOf(sz); v.Kind() == reflect.Pointer && v.IsNil() {
		return s
	}

	s.serializer = sz

	return s
}

// AdditionalData sets custom additional authenticated data bound into the
// HMAC. Use this to namespace cookies that share a key (e.g., separate
// "session" from "csrf" tokens) and to bind cookies to context such as a
// user ID. Cookies signed with one AAD will not verify under another.
//
// By default no AAD is used. Pass nil to clear.
func (s *SignedCookie) AdditionalData(data []byte) *SignedCookie {
	if s == nil {
		return s
	}

	if data == nil {
		s.aad = nil
	} else {
		s.aad = make([]byte, len(data))
		copy(s.aad, data)
	}

	return s
}

// Encode serializes, signs, and base64-encodes a value. The wire format is
// base64url(timestamp || payload || hmac-sha256(timestamp || payload || aad)).
func (s *SignedCookie) Encode(value any) (string, error) {
	if s == nil || len(s.hashKey) == 0 {
		return "", ErrEncodeFailed
	}

	plaintext, err := s.serializer.Serialize(value)
	if err != nil {
		return "", fmt.Errorf("%w: serialization: %s", ErrEncodeFailed, err)
	}

	compressed := maybeCompress(plaintext)

	ts := make([]byte, timestampSize)
	binary.BigEndian.PutUint64(ts, uint64(s.now().Unix()))

	body := make([]byte, 0, timestampSize+len(compressed)+hmacSize)
	body = append(body, ts...)
	body = append(body, compressed...)

	mac := s.computeHMAC(body)
	body = append(body, mac...)

	encoded := base64.RawURLEncoding.EncodeToString(body)

	if s.maxLength > 0 && len(encoded) > s.maxLength {
		return "", ErrValueTooLong
	}

	return encoded, nil
}

// Decode base64-decodes, verifies the HMAC, validates the timestamp, and
// deserializes a cookie value into dst.
func (s *SignedCookie) Decode(value string, dst any) error {
	if s == nil || len(s.hashKey) == 0 {
		return ErrDecodeFailed
	}

	if s.maxLength > 0 && len(value) > s.maxLength {
		return ErrValueTooLong
	}

	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("%w: base64: %s", ErrDecodeFailed, err)
	}

	if len(raw) < timestampSize+hmacSize {
		return fmt.Errorf("%w: payload too short", ErrDecodeFailed)
	}

	bodyLen := len(raw) - hmacSize
	body := raw[:bodyLen]
	gotMAC := raw[bodyLen:]
	wantMAC := s.computeHMAC(body)

	if subtle.ConstantTimeCompare(gotMAC, wantMAC) != 1 {
		return fmt.Errorf("%w: signature mismatch", ErrDecodeFailed)
	}

	ts := int64(binary.BigEndian.Uint64(body[:timestampSize]))
	now := s.now().Unix()

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

	data, err := maybeDecompress(body[timestampSize:])
	if err != nil {
		return fmt.Errorf("%w: decompression: %s", ErrDecodeFailed, err)
	}

	if err := s.serializer.Deserialize(data, dst); err != nil {
		return fmt.Errorf("%w: deserialization: %s", ErrDecodeFailed, err)
	}

	return nil
}

// computeHMAC returns HMAC-SHA256(body || aad) under hashKey.
func (s *SignedCookie) computeHMAC(body []byte) []byte {
	h := hmac.New(sha256.New, s.hashKey)
	h.Write(body)
	if len(s.aad) > 0 {
		h.Write(s.aad)
	}
	return h.Sum(nil)
}

// GenerateSignedKey returns a cryptographically random key of the given
// size suitable for use with [NewSigned]. 32 bytes is recommended.
func GenerateSignedKey(size int) ([]byte, error) {
	if size <= 0 {
		return nil, ErrInvalidKey
	}

	key := make([]byte, size)
	if _, err := io.ReadFull(cryptoRand, key); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidKey, err)
	}

	return key, nil
}

// SignedCodecsFromKeys returns a slice of [Codec] backed by [SignedCookie]
// instances, one per key, for graceful key rotation. The first key is used
// for new cookies; all keys are tried for decoding.
func SignedCodecsFromKeys(keys ...[]byte) ([]Codec, error) {
	if len(keys) == 0 {
		return nil, ErrInvalidKey
	}

	codecs := make([]Codec, 0, len(keys))
	for _, k := range keys {
		c, err := NewSigned(k)
		if err != nil {
			return nil, err
		}
		codecs = append(codecs, c)
	}

	return codecs, nil
}

// Compile-time check that SignedCookie satisfies the Codec interface.
var _ Codec = (*SignedCookie)(nil)
