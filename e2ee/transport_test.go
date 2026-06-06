package e2ee

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransportNilBaseUsesDefault(t *testing.T) {
	tr := NewTransport(nil, ClientConfig{})
	assert.NotNil(t, tr.base)
}

func TestTransportEncryptError(t *testing.T) {
	// No key set in the config makes EncryptRequest fail inside RoundTrip.
	tr := NewTransport(http.DefaultTransport, ClientConfig{})

	req, _ := http.NewRequest(http.MethodGet, "https://x/api", nil)

	_, err := tr.RoundTrip(req)
	require.ErrorIs(t, err, ErrNoKeySet)
}

func TestTransportGetBodyError(t *testing.T) {
	cs := clientKeySetFor(t)

	tr := NewTransport(http.DefaultTransport, ClientConfig{KeySet: cs})

	req, _ := http.NewRequest(http.MethodPost, "https://x/api", strings.NewReader("body"))
	req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("getbody failed") }

	_, err := tr.RoundTrip(req)
	require.Error(t, err)
}

func TestTransportBaseError(t *testing.T) {
	cs := clientKeySetFor(t)

	tr := NewTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("base transport failed")
	}), ClientConfig{KeySet: cs})

	req, _ := http.NewRequest(http.MethodPost, "https://x/api", strings.NewReader("body"))

	_, err := tr.RoundTrip(req)
	require.Error(t, err)
}

func TestTransportDecryptError(t *testing.T) {
	cs := clientKeySetFor(t)

	// Base returns a 200 with no E2EE-Session header, so Decrypt fails and the
	// transport closes the body and returns the error.
	tr := NewTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("plaintext")),
		}, nil
	}), ClientConfig{KeySet: cs})

	req, _ := http.NewRequest(http.MethodPost, "https://x/api", strings.NewReader("body"))

	_, err := tr.RoundTrip(req)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestTransportErrorWithMalformedContentType(t *testing.T) {
	cs := clientKeySetFor(t)

	// A non-success response whose Content-Type cannot be parsed is not treated
	// as a Problem Details body, so the transport attempts to decrypt it and
	// surfaces the resulting decryption error.
	tr := NewTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("not encrypted")),
		}
		resp.Header.Set("Content-Type", "garbage/;;;")

		return resp, nil
	}), ClientConfig{KeySet: cs})

	req, _ := http.NewRequest(http.MethodPost, "https://x/api", strings.NewReader("body"))

	_, err := tr.RoundTrip(req)
	require.Error(t, err)
}
