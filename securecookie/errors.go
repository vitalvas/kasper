package securecookie

import "errors"

// Encoding and decoding errors.
var (
	ErrInvalidKey       = errors.New("securecookie: invalid key (must be 16, 24, or 32 bytes)")
	ErrEncodeFailed     = errors.New("securecookie: encode failed")
	ErrDecodeFailed     = errors.New("securecookie: decode failed")
	ErrValueTooLong     = errors.New("securecookie: encoded value too long")
	ErrTimestampExpired = errors.New("securecookie: cookie expired")
	ErrTimestampTooNew  = errors.New("securecookie: cookie too new")
	ErrTimestampFuture  = errors.New("securecookie: cookie timestamp is in the future")
	ErrNoCodecs         = errors.New("securecookie: no codecs provided")
)
