package blindrsa

import "errors"

// Key material errors.
var (
	// ErrInvalidKey is returned when key material is invalid (nil, wrong
	// type, insufficient size, etc.).
	ErrInvalidKey = errors.New("blindrsa: invalid key material")
)

// Input validation errors.
var (
	// ErrInvalidInput is returned when an input parameter is nil, empty,
	// or has an unexpected size.
	ErrInvalidInput = errors.New("blindrsa: invalid input")

	// ErrMessageTooLong is returned when the message is too long for the
	// key size.
	ErrMessageTooLong = errors.New("blindrsa: message too long for key size")

	// ErrUnsupportedVariant is returned when an unknown variant is used.
	ErrUnsupportedVariant = errors.New("blindrsa: unsupported variant")
)

// Cryptographic operation errors.
var (
	// ErrBlindingFailed is returned when the blinding operation fails.
	ErrBlindingFailed = errors.New("blindrsa: blinding failed")

	// ErrSignatureFailed is returned when blind signing fails.
	ErrSignatureFailed = errors.New("blindrsa: blind signing failed")

	// ErrFinalizeFailed is returned when unblinding or finalization fails.
	ErrFinalizeFailed = errors.New("blindrsa: finalization failed")

	// ErrVerifyFailed is returned when signature verification fails.
	ErrVerifyFailed = errors.New("blindrsa: verification failed")
)

// Configuration errors.
var (
	// ErrNoSigner is returned when a handler config is missing the signing key.
	ErrNoSigner = errors.New("blindrsa: signer key must not be nil")

	// ErrNoVerifier is returned when a config is missing the verification key.
	ErrNoVerifier = errors.New("blindrsa: verifier key must not be nil")
)
