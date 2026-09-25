package httpsig

import (
	"errors"
	"net/http"

	"github.com/vitalvas/kasper/muxhandlers"
)

// DigestAlgorithm identifies the hash algorithm for Content-Digest per
// RFC 9530. It aliases the shared implementation in kasper/muxhandlers so the
// two packages carry a single Content-Digest implementation.
type DigestAlgorithm = muxhandlers.DigestAlgorithm

const (
	// DigestSHA256 uses SHA-256 for content digest.
	DigestSHA256 = muxhandlers.DigestSHA256

	// DigestSHA512 uses SHA-512 for content digest.
	DigestSHA512 = muxhandlers.DigestSHA512
)

// SetContentDigest reads the request body, computes the digest using the
// specified algorithm, sets the Content-Digest header per RFC 9530, and
// replaces the body so it can be read again.
func SetContentDigest(r *http.Request, alg DigestAlgorithm) error {
	if err := muxhandlers.SetContentDigest(r, alg); err != nil {
		return translateDigestError(err)
	}
	return nil
}

// VerifyContentDigest verifies the Content-Digest header against the request
// body per RFC 9530. It supports multiple digest values in the header and
// verifies the first recognized algorithm.
func VerifyContentDigest(r *http.Request) error {
	if err := muxhandlers.VerifyContentDigest(r); err != nil {
		return translateDigestError(err)
	}
	return nil
}

// translateDigestError maps the shared muxhandlers digest errors onto the
// httpsig sentinel errors that this package's public API documents, preserving
// backward compatibility for callers using errors.Is.
func translateDigestError(err error) error {
	switch {
	case errors.Is(err, muxhandlers.ErrDigestMissing):
		return ErrDigestNotFound
	case errors.Is(err, muxhandlers.ErrDigestUnsupported):
		return ErrUnsupportedDigest
	case errors.Is(err, muxhandlers.ErrDigestMalformed):
		return ErrMalformedHeader
	case errors.Is(err, muxhandlers.ErrDigestMismatch):
		return ErrDigestMismatch
	default:
		return err
	}
}
