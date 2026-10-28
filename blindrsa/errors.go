package blindrsa

import "errors"

// Key material errors.
var (
	ErrInvalidKey = errors.New("blindrsa: invalid key material")
)

// Input validation errors.
var (
	ErrInvalidInput       = errors.New("blindrsa: invalid input")
	ErrMessageTooLong     = errors.New("blindrsa: message too long for key size")
	ErrUnsupportedVariant = errors.New("blindrsa: unsupported variant")
)

// Cryptographic operation errors.
var (
	ErrBlindingFailed  = errors.New("blindrsa: blinding failed")
	ErrSignatureFailed = errors.New("blindrsa: blind signing failed")
	ErrFinalizeFailed  = errors.New("blindrsa: finalization failed")
	ErrVerifyFailed    = errors.New("blindrsa: verification failed")
)

// Configuration errors.
var (
	ErrNoSigner   = errors.New("blindrsa: signer key must not be nil")
	ErrNoVerifier = errors.New("blindrsa: verifier key must not be nil")
)
