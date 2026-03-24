package blindrsa

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
)

// ClientConfig configures the blind signature client.
type ClientConfig struct {
	// Key is the issuer's RSA public key.
	Key *rsa.PublicKey

	// Variant selects the RSABSSA variant.
	Variant Variant

	// TokenEndpoint is the URL of the blind signature issuance endpoint.
	TokenEndpoint string

	// Transport is the HTTP transport used for requests to the issuer.
	// When nil, a clone of [http.DefaultTransport] is used, giving an
	// independent connection pool.
	Transport *http.Transport
}

// Client obtains RSA blind signatures from a remote issuer. Use [NewClient]
// to create a properly configured instance.
type Client struct {
	httpClient *http.Client
	pub        *rsa.PublicKey
	variant    Variant
	endpoint   string
}

// NewClient creates a blind signature [Client]. When [ClientConfig.Transport]
// is nil, a clone of [http.DefaultTransport] is used, giving an independent
// connection pool.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Key == nil {
		return nil, ErrNoVerifier
	}

	if err := validatePublicKey(cfg.Key); err != nil {
		return nil, err
	}

	if _, err := validateVariant(cfg.Variant); err != nil {
		return nil, err
	}

	if cfg.TokenEndpoint == "" {
		return nil, fmt.Errorf("%w: token endpoint must not be empty", ErrInvalidInput)
	}

	var rt http.RoundTripper
	if cfg.Transport != nil {
		rt = cfg.Transport
	} else {
		rt = http.DefaultTransport.(*http.Transport).Clone()
	}

	return &Client{
		httpClient: &http.Client{Transport: rt},
		pub:        cfg.Key,
		variant:    cfg.Variant,
		endpoint:   cfg.TokenEndpoint,
	}, nil
}

// ObtainSignature executes the full blind signature protocol: blind the
// message, send it to the issuer, and finalize the response into a standard
// RSA-PSS signature.
//
// It returns the final signature and the [State] whose InputMessage must be
// used for subsequent [Verify] calls.
func (c *Client) ObtainSignature(ctx context.Context, msg []byte) (sig []byte, state *State, err error) {
	preparedMsg, err := Prepare(c.variant, rand.Reader, msg)
	if err != nil {
		return nil, nil, err
	}

	blindedMsg, st, err := Blind(c.variant, c.pub, rand.Reader, preparedMsg)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(blindedMsg))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrBlindingFailed, err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrSignatureFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("%w: issuer returned status %d", ErrSignatureFailed, resp.StatusCode)
	}

	blindSig, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrSignatureFailed, err)
	}

	finalSig, err := Finalize(c.variant, c.pub, blindSig, st)
	if err != nil {
		return nil, nil, err
	}

	return finalSig, st, nil
}
