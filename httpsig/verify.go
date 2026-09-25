package httpsig

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/vitalvas/kasper/sfv"
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
// Signature-Input header dictionary (an RFC 9651 dictionary parsed via sfv).
// When label is empty, the first entry is returned. The returned value is the
// serialized member (an inner list with parameters) suitable for
// parseSignatureParams.
func findSignatureInput(header, label string) (string, string, error) {
	dict, err := sfv.ParseDictionary(header)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrMalformedHeader, err)
	}

	for _, entry := range dict {
		if label == "" || entry.Key == label {
			if !entry.Member.IsInnerList {
				return "", "", fmt.Errorf("%w: signature input must be an inner list", ErrMalformedHeader)
			}
			return entry.Key, entry.Member.InnerList.String(), nil
		}
	}

	return "", "", ErrSignatureNotFound
}

// extractSignatureValue extracts the base64-decoded signature bytes for the
// given label from the Signature header dictionary (parsed via sfv). The
// signature value is a byte-sequence item.
func extractSignatureValue(header, label string) ([]byte, error) {
	dict, err := sfv.ParseDictionary(header)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedHeader, err)
	}

	for _, entry := range dict {
		if entry.Key != label {
			continue
		}
		if entry.Member.IsInnerList || entry.Member.Item.Value.Kind != sfv.KindByteSequence {
			return nil, fmt.Errorf("%w: signature value not byte-sequence encoded", ErrMalformedHeader)
		}
		return entry.Member.Item.Value.Bytes, nil
	}

	return nil, ErrSignatureNotFound
}
