package httpsig

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DigestAlgorithm identifies the hash algorithm for Content-Digest
// per RFC 9530.
type DigestAlgorithm string

const (
	// DigestSHA256 uses SHA-256 for content digest.
	DigestSHA256 DigestAlgorithm = "sha-256"

	// DigestSHA512 uses SHA-512 for content digest.
	DigestSHA512 DigestAlgorithm = "sha-512"
)

// SetContentDigest reads the request body, computes the digest using the
// specified algorithm, sets the Content-Digest header per RFC 9530, and
// replaces the body so it can be read again.
func SetContentDigest(r *http.Request, alg DigestAlgorithm) error {
	body, err := readAndRestoreBody(r)
	if err != nil {
		return err
	}

	digest, err := computeDigest(body, alg)
	if err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(digest)
	r.Header.Set("Content-Digest", fmt.Sprintf("%s=:%s:", alg, encoded))

	return nil
}

// VerifyContentDigest verifies the Content-Digest header against the request
// body per RFC 9530. It supports multiple digest values in the header
// and verifies the first recognized algorithm.
func VerifyContentDigest(r *http.Request) error {
	header := r.Header.Get("Content-Digest")
	if header == "" {
		return ErrDigestNotFound
	}

	body, err := readAndRestoreBody(r)
	if err != nil {
		return err
	}

	// Parse the dictionary of digest values.
	for entry := range strings.SplitSeq(header, ",") {
		entry = strings.TrimSpace(entry)
		alg, encodedDigest, ok := parseDigestEntry(entry)
		if !ok {
			continue
		}

		expected, err := computeDigest(body, alg)
		if err != nil {
			return err
		}

		actual, err := base64.StdEncoding.DecodeString(encodedDigest)
		if err != nil {
			return fmt.Errorf("%w: invalid base64 in digest", ErrMalformedHeader)
		}

		if !bytes.Equal(expected, actual) {
			return ErrDigestMismatch
		}

		return nil
	}

	return ErrUnsupportedDigest
}

// parseDigestEntry parses a single "alg=:base64:" entry from the
// Content-Digest header.
func parseDigestEntry(entry string) (DigestAlgorithm, string, bool) {
	algStr, value, ok := strings.Cut(entry, "=")
	if !ok {
		return "", "", false
	}

	alg := DigestAlgorithm(strings.TrimSpace(algStr))
	value = strings.TrimSpace(value)

	// Value must be :base64:
	if len(value) < 2 || value[0] != ':' || value[len(value)-1] != ':' {
		return "", "", false
	}

	encoded := value[1 : len(value)-1]

	switch alg {
	case DigestSHA256, DigestSHA512:
		return alg, encoded, true
	default:
		return "", "", false
	}
}

// computeDigest computes the hash of data using the specified algorithm.
func computeDigest(data []byte, alg DigestAlgorithm) ([]byte, error) {
	switch alg {
	case DigestSHA256:
		h := sha256.Sum256(data)
		return h[:], nil
	case DigestSHA512:
		h := sha512.Sum512(data)
		return h[:], nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedDigest, alg)
	}
}

// readAndRestoreBody reads the entire request body and replaces it with a
// new reader so the body can be consumed again by downstream handlers.
func readAndRestoreBody(r *http.Request) ([]byte, error) {
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
