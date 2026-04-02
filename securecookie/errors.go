package securecookie

import "errors"

// Encoding and decoding errors.
var (
	// ErrInvalidKey is returned when the encryption key is not 16, 24, or 32 bytes.
	ErrInvalidKey = errors.New("securecookie: invalid key (must be 16, 24, or 32 bytes)")

	// ErrEncodeFailed is returned when encoding a cookie value fails.
	ErrEncodeFailed = errors.New("securecookie: encode failed")

	// ErrDecodeFailed is returned when decoding a cookie value fails.
	ErrDecodeFailed = errors.New("securecookie: decode failed")

	// ErrValueTooLong is returned when the encoded value exceeds MaxLength.
	ErrValueTooLong = errors.New("securecookie: encoded value too long")

	// ErrTimestampExpired is returned when the cookie timestamp exceeds MaxAge.
	ErrTimestampExpired = errors.New("securecookie: cookie expired")

	// ErrTimestampTooNew is returned when the cookie timestamp is newer than MinAge allows.
	ErrTimestampTooNew = errors.New("securecookie: cookie too new")

	// ErrTimestampFuture is returned when the cookie timestamp is in the future.
	ErrTimestampFuture = errors.New("securecookie: cookie timestamp is in the future")

	// ErrNoCodecs is returned when an empty codec list is passed to EncodeMulti or DecodeMulti.
	ErrNoCodecs = errors.New("securecookie: no codecs provided")
)
