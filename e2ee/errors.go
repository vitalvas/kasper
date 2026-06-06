package e2ee

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/vitalvas/kasper/muxhandlers"
)

// Sentinel errors. These map onto the protocol error codes defined in the
// draft and are surfaced to callers via errors.Is. Server-side they are
// translated into RFC 9457 Problem Details responses by ProblemFromError.
var (
	// ErrKeyUnknown is returned when the requested kid is not recognized.
	ErrKeyUnknown = errors.New("e2ee: key unknown")

	// ErrKeyExpired is returned when the key is outside its validity window.
	ErrKeyExpired = errors.New("e2ee: key expired")

	// ErrAEADUnsupported is returned when the requested AEAD is not advertised
	// for the resolved kid.
	ErrAEADUnsupported = errors.New("e2ee: aead unsupported")

	// ErrDecryptFailed is returned when AES-GCM authentication fails.
	ErrDecryptFailed = errors.New("e2ee: decrypt failed")

	// ErrTimestampSkew is returned when the message timestamp is outside the
	// acceptable window.
	ErrTimestampSkew = errors.New("e2ee: timestamp skew")

	// ErrReplayDetected is returned when the nid was already processed.
	ErrReplayDetected = errors.New("e2ee: replay detected")

	// ErrMalformed is returned when the E2EE-Session field or message body
	// fails parsing or structural validation.
	ErrMalformed = errors.New("e2ee: malformed")
)

// Configuration and key-material errors. These are not protocol error codes;
// they signal misuse of the API.
var (
	// ErrInvalidKey is returned when key material is invalid (wrong length,
	// nil, or not on the expected curve).
	ErrInvalidKey = errors.New("e2ee: invalid key material")

	// ErrNoKeys is returned when a key set contains no usable keys.
	ErrNoKeys = errors.New("e2ee: no keys")

	// ErrNoKeySet is returned when a client is configured without a key set
	// source.
	ErrNoKeySet = errors.New("e2ee: no key set")
)

// errorCode returns the protocol error code string for a sentinel error,
// along with the HTTP status code defined by the draft. It returns ok=false
// for errors that are not protocol error codes.
func errorCode(err error) (code string, status int, ok bool) {
	switch {
	case errors.Is(err, ErrKeyUnknown):
		return "key_unknown", http.StatusBadRequest, true
	case errors.Is(err, ErrKeyExpired):
		return "key_expired", http.StatusBadRequest, true
	case errors.Is(err, ErrAEADUnsupported):
		return "aead_unsupported", http.StatusBadRequest, true
	case errors.Is(err, ErrDecryptFailed):
		return "decrypt_failed", http.StatusBadRequest, true
	case errors.Is(err, ErrTimestampSkew):
		return "timestamp_skew", http.StatusBadRequest, true
	case errors.Is(err, ErrReplayDetected):
		return "replay_detected", http.StatusTooEarly, true
	case errors.Is(err, ErrMalformed):
		return "malformed", http.StatusBadRequest, true
	default:
		return "", 0, false
	}
}

// problemTitle maps each protocol error code to its fixed human-readable
// title per the draft.
var problemTitle = map[string]string{
	"key_unknown":      "Key Unknown",
	"key_expired":      "Key Expired",
	"aead_unsupported": "AEAD Unsupported",
	"decrypt_failed":   "Decryption Failed",
	"timestamp_skew":   "Timestamp Out Of Range",
	"replay_detected":  "Replay Detected",
	"malformed":        "Malformed Request",
}

// Problem is an RFC 9457 Problem Details object describing an E2EE protocol
// error. It is an alias for [muxhandlers.ProblemDetails], reusing the shared
// kasper implementation (extension members, media type, and writer).
type Problem = muxhandlers.ProblemDetails

// ProblemFromError builds a [Problem] from a protocol error. The Type member
// uses the URN form urn:ietf:params:e2ee:error:<code>. When err is not a
// recognized protocol error code, a generic malformed problem is returned so
// that internal details are never leaked to the peer.
func ProblemFromError(err error) Problem {
	code, status, ok := errorCode(err)
	if !ok {
		code, status = "malformed", http.StatusBadRequest
	}

	return Problem{
		Type:   fmt.Sprintf("urn:ietf:params:e2ee:error:%s", code),
		Status: status,
		Title:  problemTitle[code],
	}
}

// WriteProblem writes an RFC 9457 Problem Details response for err to w via
// [muxhandlers.WriteProblemDetails]. It sets the Content-Type to
// application/problem+json and the status code from the problem. It returns
// the Problem that was written.
func WriteProblem(w http.ResponseWriter, err error) Problem {
	prob := ProblemFromError(err)

	muxhandlers.WriteProblemDetails(w, prob)

	return prob
}
