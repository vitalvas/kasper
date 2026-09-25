package muxhandlers

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"

	"github.com/vitalvas/kasper/mux"
	"github.com/vitalvas/kasper/sfv"
)

// DigestAlgorithm identifies a Content-Digest hash algorithm per RFC 9530.
type DigestAlgorithm string

const (
	// DigestSHA256 selects SHA-256.
	DigestSHA256 DigestAlgorithm = "sha-256"
	// DigestSHA512 selects SHA-512.
	DigestSHA512 DigestAlgorithm = "sha-512"
)

// Sentinel errors for the Content-Digest middleware.
var (
	// ErrNoDigestAlgorithm is returned when Algorithm is empty or unsupported.
	ErrNoDigestAlgorithm = errors.New("content digest: a supported algorithm is required")
	// ErrDigestMissing is returned during request verification when the
	// Content-Digest header is absent.
	ErrDigestMissing = errors.New("content digest: header missing")
	// ErrDigestMismatch is returned during request verification when no digest
	// value matches the request body.
	ErrDigestMismatch = errors.New("content digest: body does not match header")
	// ErrDigestMalformed is returned when the Content-Digest header cannot be
	// parsed as an RFC 9651 dictionary of byte sequences.
	ErrDigestMalformed = errors.New("content digest: malformed header")
	// ErrDigestUnsupported is returned when the Content-Digest header contains
	// no supported algorithm (only entries such as md5 or sha-1).
	ErrDigestUnsupported = errors.New("content digest: no supported algorithm")
)

// ContentDigestConfig configures the Content-Digest middleware (RFC 9530).
type ContentDigestConfig struct {
	// Algorithm is the hash used to emit Content-Digest on responses.
	// Must be DigestSHA256 or DigestSHA512. Required.
	Algorithm DigestAlgorithm

	// VerifyRequests, when true, verifies an incoming Content-Digest header
	// against the request body and rejects a mismatch before the request
	// reaches the handler. Off by default. Requests without a Content-Digest
	// header are rejected only when RequireRequestDigest is also set.
	VerifyRequests bool

	// RequireRequestDigest, when true together with VerifyRequests, rejects
	// requests that carry no Content-Digest header. When false, a missing
	// header is allowed and only a present-but-wrong digest is rejected.
	RequireRequestDigest bool

	// OnError handles request verification failure. When nil, the middleware
	// writes 400 Bad Request with no body.
	OnError func(w http.ResponseWriter, r *http.Request, err error)
}

// ContentDigestMiddleware returns a middleware that adds a Content-Digest
// response header (RFC 9530) computed over the response body, and optionally
// verifies the Content-Digest of incoming requests.
//
// It returns ErrNoDigestAlgorithm if Algorithm is not a supported value.
func ContentDigestMiddleware(cfg ContentDigestConfig) (mux.MiddlewareFunc, error) {
	if !supportedDigest(cfg.Algorithm) {
		return nil, ErrNoDigestAlgorithm
	}

	onError := cfg.OnError
	if onError == nil {
		onError = func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.VerifyRequests {
				if err := verifyRequestDigest(r, cfg.RequireRequestDigest); err != nil {
					onError(w, r, err)
					return
				}
			}

			rec := &digestRecorder{header: http.Header{}, body: &bytes.Buffer{}}
			next.ServeHTTP(rec, r)

			digest := contentDigestField(rec.body.Bytes(), cfg.Algorithm)
			copyHeader(w.Header(), rec.header)
			w.Header().Set("Content-Digest", digest)

			status := rec.statusCode
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			_, _ = w.Write(rec.body.Bytes())
		})
	}, nil
}

// SetContentDigest computes the digest of the request body with alg, sets the
// Content-Digest header (RFC 9530) built via the sfv serializer, and restores
// the body so it can be read again. It returns ErrDigestUnsupported when alg is
// not a supported algorithm.
func SetContentDigest(r *http.Request, alg DigestAlgorithm) error {
	if !supportedDigest(alg) {
		return ErrDigestUnsupported
	}
	body, err := readAndRestoreRequestBody(r)
	if err != nil {
		return err
	}
	r.Header.Set("Content-Digest", contentDigestField(body, alg))
	return nil
}

// VerifyContentDigest verifies the Content-Digest header (RFC 9530) against the
// request body, reading and restoring the body. It returns:
//   - ErrDigestMissing when the header is absent;
//   - ErrDigestMalformed when the header is not a valid dictionary of byte
//     sequences;
//   - ErrDigestUnsupported when no entry uses a supported algorithm;
//   - ErrDigestMismatch when a supported entry does not match the body.
//
// Verification succeeds as soon as one supported entry matches.
func VerifyContentDigest(r *http.Request) error {
	header := r.Header.Get("Content-Digest")
	if header == "" {
		return ErrDigestMissing
	}

	dict, err := sfv.ParseDictionary(header)
	if err != nil {
		return ErrDigestMalformed
	}

	body, err := readAndRestoreRequestBody(r)
	if err != nil {
		return err
	}

	sawSupported := false
	for _, entry := range dict {
		alg := DigestAlgorithm(entry.Key)
		if !supportedDigest(alg) {
			continue
		}
		if entry.Member.IsInnerList || entry.Member.Item.Value.Kind != sfv.KindByteSequence {
			return ErrDigestMalformed
		}
		sawSupported = true
		want := entry.Member.Item.Value.Bytes
		got := hashBody(body, alg)
		if subtle.ConstantTimeCompare(want, got) == 1 {
			return nil
		}
	}
	if !sawSupported {
		return ErrDigestUnsupported
	}
	return ErrDigestMismatch
}

// verifyRequestDigest is the middleware wrapper: a missing header is allowed
// unless requireHeader is set.
func verifyRequestDigest(r *http.Request, requireHeader bool) error {
	err := VerifyContentDigest(r)
	if errors.Is(err, ErrDigestMissing) && !requireHeader {
		return nil
	}
	return err
}

// contentDigestField returns the RFC 9530 Content-Digest field value for data,
// e.g. "sha-256=:<base64>:", built via the sfv serializer.
func contentDigestField(data []byte, alg DigestAlgorithm) string {
	sum := hashBody(data, alg)
	dict := sfv.Dictionary{
		{
			Key:    string(alg),
			Member: sfv.Member{Item: sfv.Item{Value: sfv.ByteSequence(sum)}},
		},
	}
	return dict.String()
}

func hashBody(data []byte, alg DigestAlgorithm) []byte {
	switch alg {
	case DigestSHA512:
		h := sha512.Sum512(data)
		return h[:]
	default:
		h := sha256.Sum256(data)
		return h[:]
	}
}

func supportedDigest(alg DigestAlgorithm) bool {
	return alg == DigestSHA256 || alg == DigestSHA512
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// readAndRestoreRequestBody reads the request body fully and replaces it so
// downstream handlers can read it again.
func readAndRestoreRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// digestRecorder buffers a handler's response so the Content-Digest can be
// computed over the complete body before anything is written to the client.
type digestRecorder struct {
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func (d *digestRecorder) Header() http.Header { return d.header }

func (d *digestRecorder) WriteHeader(code int) {
	if d.statusCode == 0 {
		d.statusCode = code
	}
}

func (d *digestRecorder) Write(b []byte) (int, error) {
	if d.statusCode == 0 {
		d.statusCode = http.StatusOK
	}
	return d.body.Write(b)
}
