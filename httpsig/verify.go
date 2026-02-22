package httpsig

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// KeyResolver returns a Verifier for the given key ID and algorithm.
// It is called during request verification to look up the appropriate key.
// The request is provided for context (e.g., to select keys based on
// the request host or path).
type KeyResolver func(r *http.Request, keyID string, alg Algorithm) (Verifier, error)

// VerifyConfig configures HTTP request signature verification per RFC 9421.
type VerifyConfig struct {
	// Resolver looks up a Verifier for a given key ID and algorithm.
	// Required.
	Resolver KeyResolver

	// Label identifies which signature to verify. When empty, the first
	// signature found in the Signature-Input header is used.
	Label string

	// RequiredComponents lists component identifiers that must be present
	// in the signature's covered components. Verification fails if any
	// required component is missing.
	RequiredComponents []string

	// MaxAge is the maximum acceptable age of the signature. When non-zero,
	// signatures older than MaxAge are rejected. Requires the "created"
	// parameter in the signature.
	MaxAge time.Duration

	// RequireDigest, when true, requires a Content-Digest header and
	// verifies it against the request body before signature verification.
	RequireDigest bool
}

// VerifyRequest verifies an HTTP request signature per RFC 9421.
func VerifyRequest(r *http.Request, cfg VerifyConfig) error {
	if cfg.Resolver == nil {
		return ErrNoResolver
	}

	// Optionally verify Content-Digest.
	if cfg.RequireDigest {
		if err := VerifyContentDigest(r); err != nil {
			return err
		}
	}

	// Parse the Signature-Input header to find the target signature.
	sigInputHeader := r.Header.Get("Signature-Input")
	if sigInputHeader == "" {
		return ErrSignatureNotFound
	}

	label, sigParamsRaw, err := findSignatureInput(sigInputHeader, cfg.Label)
	if err != nil {
		return err
	}

	// Parse signature parameters.
	params, err := parseSignatureParams(sigParamsRaw)
	if err != nil {
		return err
	}

	// Check required components.
	for _, req := range cfg.RequiredComponents {
		if !slices.Contains(params.components, req) {
			return fmt.Errorf("%w: %s", ErrMissingComponent, req)
		}
	}

	// Check signature expiration.
	if !params.expires.IsZero() && time.Now().After(params.expires) {
		return ErrSignatureExpired
	}

	// Check signature age.
	if cfg.MaxAge > 0 {
		if params.created.IsZero() {
			return ErrCreatedRequired
		}

		age := time.Since(params.created)
		if age < 0 || age > cfg.MaxAge {
			return ErrSignatureExpired
		}
	}

	// Resolve the verifier.
	verifier, err := cfg.Resolver(r, params.keyID, params.alg)
	if err != nil {
		return err
	}

	// Reconstruct the signature base.
	base, _, err := buildSignatureBase(r, params)
	if err != nil {
		return err
	}

	// Extract the signature value.
	sigHeader := r.Header.Get("Signature")
	if sigHeader == "" {
		return ErrSignatureNotFound
	}

	sigBytes, err := extractSignatureValue(sigHeader, label)
	if err != nil {
		return err
	}

	return verifier.Verify(base, sigBytes)
}

// findSignatureInput finds the signature input for the given label in the
// Signature-Input header dictionary. When label is empty, the first entry
// is returned.
func findSignatureInput(header, label string) (string, string, error) {
	for _, entry := range splitQuoteAware(header, ',') {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if label == "" || key == label {
			return key, value, nil
		}
	}

	return "", "", ErrSignatureNotFound
}

// extractSignatureValue extracts the base64-decoded signature bytes for the
// given label from the Signature header dictionary.
func extractSignatureValue(header, label string) ([]byte, error) {
	for _, entry := range splitQuoteAware(header, ',') {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if key != label {
			continue
		}

		// Value should be :base64:
		if len(value) < 2 || value[0] != ':' || value[len(value)-1] != ':' {
			return nil, fmt.Errorf("%w: signature value not byte-sequence encoded", ErrMalformedHeader)
		}

		encoded := value[1 : len(value)-1]

		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid base64 in signature", ErrMalformedHeader)
		}

		return decoded, nil
	}

	return nil, ErrSignatureNotFound
}
