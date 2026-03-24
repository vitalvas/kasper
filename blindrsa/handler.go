package blindrsa

import (
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
)

const defaultMaxBodySize = 4096

// IssuerConfig configures the server-side blind signature issuance handler.
type IssuerConfig struct {
	// Key is the RSA private key used for blind signing.
	Key *rsa.PrivateKey

	// Variant selects the RSABSSA variant.
	Variant Variant

	// OnError is called when issuance fails. When nil, a plain 400 Bad
	// Request response is sent.
	OnError func(w http.ResponseWriter, r *http.Request, err error)

	// MaxBodySize limits the request body size in bytes. When zero,
	// defaultMaxBodySize (4096) is used.
	MaxBodySize int
}

// IssueHandler returns an [http.Handler] that reads a blinded message from
// the request body, calls [BlindSign], and writes the blind signature as an
// application/octet-stream response.
func IssueHandler(cfg IssuerConfig) (http.Handler, error) {
	if cfg.Key == nil {
		return nil, ErrNoSigner
	}

	if err := validatePrivateKey(cfg.Key); err != nil {
		return nil, err
	}

	if _, err := validateVariant(cfg.Variant); err != nil {
		return nil, err
	}

	priv := cfg.Key
	variant := cfg.Variant

	onError := cfg.OnError
	if onError == nil {
		onError = defaultIssuerOnError
	}

	maxBody := cfg.MaxBodySize
	if maxBody <= 0 {
		maxBody = defaultMaxBodySize
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, int64(maxBody)+1))
		if err != nil {
			onError(w, r, fmt.Errorf("%w: %s", ErrInvalidInput, err))
			return
		}

		if len(body) > maxBody {
			onError(w, r, fmt.Errorf("%w: body exceeds maximum size of %d bytes", ErrInvalidInput, maxBody))
			return
		}

		blindSig, err := BlindSign(variant, priv, body)
		if err != nil {
			onError(w, r, err)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write(blindSig)
	}), nil
}

// defaultIssuerOnError writes a 400 Bad Request response with no body.
func defaultIssuerOnError(w http.ResponseWriter, _ *http.Request, _ error) {
	w.WriteHeader(http.StatusBadRequest)
}
