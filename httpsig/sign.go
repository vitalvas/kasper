package httpsig

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"slices"
	"time"
)

// nonceSize is the number of random bytes used to generate a nonce.
const nonceSize = 16

// defaultCoveredComponents are the default components signed when
// SignConfig.CoveredComponents is empty.
var defaultCoveredComponents = []string{ComponentMethod, ComponentAuthority, ComponentPath}

// defaultResponseComponents are the default components signed by SignResponse
// when SignConfig.CoveredComponents is empty.
var defaultResponseComponents = []string{ComponentStatus}

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

// SignConfig configures HTTP message signing per RFC 9421. It is shared by
// SignRequest and SignResponse.
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

	// DigestAlgorithm, when set, causes SignRequest and SignResponse to
	// compute and set a Content-Digest header (RFC 9530) over the message
	// body before signing. The "content-digest" component is automatically
	// added to covered components if not already present.
	DigestAlgorithm DigestAlgorithm
}

// SignRequest signs an HTTP request in-place by adding Signature and
// Signature-Input headers per RFC 9421.
func SignRequest(r *http.Request, cfg SignConfig) error {
	if cfg.Signer == nil {
		return ErrNoSigner
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

	return signMessage(sigMessage{req: r}, components, cfg)
}

// SignResponse signs an HTTP response in-place by adding Signature and
// Signature-Input headers per RFC 9421. req is the originating request;
// covered components carrying the ";req" parameter (for example
// "@authority;req") resolve against it, binding the response signature to
// the request. req may be nil when no ";req" components are covered.
//
// When CoveredComponents is empty, [ComponentStatus] is signed. When
// DigestAlgorithm is set, a Content-Digest header is computed over the
// response body (which is read and restored) and "content-digest" is added
// to the covered components.
func SignResponse(resp *http.Response, req *http.Request, cfg SignConfig) error {
	if cfg.Signer == nil {
		return ErrNoSigner
	}

	components := cfg.CoveredComponents
	if len(components) == 0 {
		components = defaultResponseComponents
	}

	// Optionally set Content-Digest.
	if cfg.DigestAlgorithm != "" {
		if err := SetResponseContentDigest(resp, cfg.DigestAlgorithm); err != nil {
			return err
		}

		if !slices.Contains(components, "content-digest") {
			components = append(components, "content-digest")
		}
	}

	return signMessage(sigMessage{req: req, resp: resp}, components, cfg)
}

// signMessage builds the signature base for the message, signs it, and
// appends Signature and Signature-Input headers to the message.
func signMessage(m sigMessage, components []string, cfg SignConfig) error {
	label := cfg.Label
	if label == "" {
		label = "sig1"
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

	base, sigParamsStr, err := buildSignatureBase(m, params)
	if err != nil {
		return err
	}

	sig, err := cfg.Signer.Sign(base)
	if err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(sig)

	// Append to existing headers (supports multiple signatures).
	hdr := m.header()
	appendDictMember(hdr, "Signature-Input", label, sigParamsStr)
	appendDictMember(hdr, "Signature", label, fmt.Sprintf(":%s:", encoded))

	return nil
}

// appendDictMember appends a key=value member to an RFC 8941 dictionary
// header. If the header already has content, the new member is appended
// with a comma separator.
func appendDictMember(hdr http.Header, header, key, value string) {
	existing := hdr.Get(header)
	entry := fmt.Sprintf("%s=%s", key, value)

	if existing == "" {
		hdr.Set(header, entry)
	} else {
		hdr.Set(header, fmt.Sprintf("%s, %s", existing, entry))
	}
}
