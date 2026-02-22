package httpsig

import "errors"

// Signing errors.
var (
	// ErrNoSigner is returned when SignConfig has no Signer configured.
	ErrNoSigner = errors.New("httpsig: signer must not be nil")

	// ErrNoCoveredComponents is returned when SignConfig has an empty
	// CoveredComponents slice.
	ErrNoCoveredComponents = errors.New("httpsig: covered components must not be empty")
)

// Verification errors.
var (
	// ErrNoResolver is returned when VerifyConfig has no KeyResolver configured.
	ErrNoResolver = errors.New("httpsig: key resolver must not be nil")

	// ErrSignatureNotFound is returned when the expected signature label is
	// not present in the Signature-Input header.
	ErrSignatureNotFound = errors.New("httpsig: signature not found")

	// ErrSignatureInvalid is returned when signature verification fails.
	ErrSignatureInvalid = errors.New("httpsig: signature verification failed")

	// ErrSignatureExpired is returned when the signature has exceeded its
	// maximum allowed age.
	ErrSignatureExpired = errors.New("httpsig: signature expired")

	// ErrCreatedRequired is returned when MaxAge is set but the signature
	// does not contain a created parameter.
	ErrCreatedRequired = errors.New("httpsig: created parameter required when MaxAge is set")

	// ErrMissingComponent is returned when a required covered component
	// is absent from the signature.
	ErrMissingComponent = errors.New("httpsig: required component missing from signature")

	// ErrMalformedHeader is returned when Signature or Signature-Input
	// headers cannot be parsed.
	ErrMalformedHeader = errors.New("httpsig: malformed signature header")
)

// Key material errors.
var (
	// ErrInvalidKey is returned when key material is invalid (nil, wrong
	// curve, insufficient size, etc.).
	ErrInvalidKey = errors.New("httpsig: invalid key material")
)

// Digest errors.
var (
	// ErrDigestMismatch is returned when Content-Digest verification fails.
	ErrDigestMismatch = errors.New("httpsig: content digest mismatch")

	// ErrDigestNotFound is returned when Content-Digest header is required
	// but not present.
	ErrDigestNotFound = errors.New("httpsig: content digest not found")

	// ErrUnsupportedDigest is returned when the digest algorithm is not
	// supported.
	ErrUnsupportedDigest = errors.New("httpsig: unsupported digest algorithm")
)

// Component errors.
var (
	// ErrUnknownComponent is returned when an unrecognized derived component
	// identifier is used.
	ErrUnknownComponent = errors.New("httpsig: unknown component identifier")
)
