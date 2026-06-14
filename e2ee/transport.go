package e2ee

import (
	"mime"
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// Transport is an http.RoundTripper that encrypts outgoing request bodies and
// decrypts incoming response bodies using the E2EE-HTTP scheme.
//
// Use NewTransport to wrap a base transport. Each request is cloned before
// encryption so the caller's request is not mutated.
type Transport struct {
	base   http.RoundTripper
	config ClientConfig
}

// NewTransport creates an encrypting Transport that delegates to base after
// encrypting each request and before returning the decrypted response. When
// base is nil, a clone of http.DefaultTransport is used.
func NewTransport(base http.RoundTripper, cfg ClientConfig) *Transport {
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	}

	return &Transport{
		base:   base,
		config: cfg,
	}
}

// RoundTrip encrypts the request, delegates to the base transport, then
// decrypts the response. The original request is cloned before encryption.
// When the response is an RFC 9457 Problem Details error (non-success status
// with application/problem+json), it is returned undecrypted so the caller can
// inspect the protocol error, since e2ee error responses are sent in plaintext
// (draft-vasylenko-e2ee-http Sections 9, 11.6).
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())

	if clone.Body != nil && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}

		clone.Body = body
	}

	dec, err := EncryptRequest(clone, t.config)
	if err != nil {
		return nil, err
	}

	resp, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, err
	}

	if isProblem(resp) {
		return resp, nil
	}

	if err := dec.Decrypt(resp); err != nil {
		resp.Body.Close()

		return nil, err
	}

	return resp, nil
}

// isProblem reports whether resp is an RFC 9457 Problem Details error and
// should not be decrypted.
func isProblem(resp *http.Response) bool {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return false
	}

	// Compare the media type, ignoring any parameters such as charset.
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return false
	}

	return mediaType == mux.ContentTypeApplicationProblemJSON
}
