package httpsig

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"slices"
	"time"
)

// nonceSize is the number of random bytes used to generate a nonce.
const nonceSize = 16

// defaultCoveredComponents are the default components signed when
// SignConfig.CoveredComponents is empty.
var defaultCoveredComponents = []string{ComponentMethod, ComponentAuthority, ComponentPath}

// GenerateNonce returns a cryptographically random nonce string suitable
// for use in SignConfig.Nonce. The returned value is 16 random bytes
// encoded as unpadded base64url (22 characters).
func GenerateNonce() (string, error) {
	b := make([]byte, nonceSize)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SignConfig configures HTTP request signing per RFC 9421.
type SignConfig struct {
	// Signer produces signatures. Required.
	Signer Signer

	// Label identifies the signature in Signature/Signature-Input headers.
	// Defaults to "sig1".
	Label string

	// CoveredComponents lists the component identifiers to include in the
	// signature base. Defaults to [ComponentMethod, ComponentAuthority, ComponentPath].
	CoveredComponents []string

	// Nonce is an optional nonce value included in signature parameters.
	Nonce string

	// Tag is an optional application-specific tag for the signature.
	Tag string

	// Created sets the signature creation time. When zero, time.Now() is
	// used.
	Created time.Time

	// Expires sets the signature expiration time. When zero, no expiration
	// is set.
	Expires time.Time

	// DigestAlgorithm, when set, causes SignRequest to compute and set a
	// Content-Digest header (RFC 9530) before signing. The
	// "content-digest" component is automatically added to covered
	// components if not already present.
	DigestAlgorithm DigestAlgorithm
}

// SignRequest signs an HTTP request in-place by adding Signature and
// Signature-Input headers per RFC 9421.
func SignRequest(r *http.Request, cfg SignConfig) error {
	if cfg.Signer == nil {
		return ErrNoSigner
	}

	label := cfg.Label
	if label == "" {
		label = "sig1"
	}

	components := cfg.CoveredComponents
	if len(components) == 0 {
		components = defaultCoveredComponents
	}

	// Optionally set Content-Digest.
	if cfg.DigestAlgorithm != "" {
		if err := SetContentDigest(r, cfg.DigestAlgorithm); err != nil {
			return err
		}

		if !slices.Contains(components, "content-digest") {
			components = append(components, "content-digest")
		}
	}

	// Determine created time.
	created := cfg.Created
	if created.IsZero() {
		created = time.Now()
	}

	params := signatureParams{
		components: components,
		created:    created,
		expires:    cfg.Expires,
		nonce:      cfg.Nonce,
		alg:        cfg.Signer.Algorithm(),
		keyID:      cfg.Signer.KeyID(),
		tag:        cfg.Tag,
	}

	base, sigParamsStr, err := buildSignatureBase(r, params)
	if err != nil {
		return err
	}

	sig, err := cfg.Signer.Sign(base)
	if err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(sig)

	// Append to existing headers (supports multiple signatures).
	appendDictMember(r, "Signature-Input", label, sigParamsStr)
	appendDictMember(r, "Signature", label, ":"+encoded+":")

	return nil
}

// appendDictMember appends a key=value member to an RFC 8941 dictionary
// header. If the header already has content, the new member is appended
// with a comma separator.
func appendDictMember(r *http.Request, header, key, value string) {
	existing := r.Header.Get(header)
	entry := key + "=" + value

	if existing == "" {
		r.Header.Set(header, entry)
	} else {
		r.Header.Set(header, existing+", "+entry)
	}
}
