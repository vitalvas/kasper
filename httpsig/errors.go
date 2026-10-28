package httpsig

import "errors"

// Signing errors.
var (
	ErrNoSigner            = errors.New("httpsig: signer must not be nil")
	ErrNoCoveredComponents = errors.New("httpsig: covered components must not be empty")
)

// Verification errors.
var (
	ErrNoResolver        = errors.New("httpsig: key resolver must not be nil")
	ErrSignatureNotFound = errors.New("httpsig: signature not found")
	ErrSignatureInvalid  = errors.New("httpsig: signature verification failed")
	ErrSignatureExpired  = errors.New("httpsig: signature expired")
	ErrCreatedRequired   = errors.New("httpsig: created parameter required when MaxAge is set")
	ErrMissingComponent  = errors.New("httpsig: required component missing from signature")
	ErrMalformedHeader   = errors.New("httpsig: malformed signature header")
)

// Key material errors.
var (
	ErrInvalidKey = errors.New("httpsig: invalid key material")
)

// Digest errors.
var (
	ErrDigestMismatch    = errors.New("httpsig: content digest mismatch")
	ErrDigestNotFound    = errors.New("httpsig: content digest not found")
	ErrUnsupportedDigest = errors.New("httpsig: unsupported digest algorithm")
)

// Component errors.
var (
	ErrUnknownComponent = errors.New("httpsig: unknown component identifier")
	// ErrInvalidComponent covers "@status" or a ";req" component on a request,
	// a request-derived component without ";req" on a response, or a ";req"
	// component without an originating request.
	ErrInvalidComponent = errors.New("httpsig: component not valid for message")
)
