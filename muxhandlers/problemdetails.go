package muxhandlers

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// ProblemDetails represents an RFC 9457 Problem Details object.
//
// Spec reference: https://www.rfc-editor.org/rfc/rfc9457
type ProblemDetails struct {
	// Type is a URI reference that identifies the problem type. When
	// dereferenced, it should provide human-readable documentation.
	// Defaults to "about:blank" when empty, per RFC 9457 Section 3.1.3.
	Type string `json:"type"`

	// Title is a short, human-readable summary of the problem type.
	// It should not change from occurrence to occurrence of the same
	// problem type, per RFC 9457 Section 3.1.4.
	Title string `json:"title"`

	// Status is the HTTP status code for this occurrence of the problem.
	// Per RFC 9457 Section 3.1.1.
	Status int `json:"status"`

	// Detail is a human-readable explanation specific to this occurrence
	// of the problem, per RFC 9457 Section 3.1.5.
	Detail string `json:"detail,omitempty"`

	// Instance is a URI reference that identifies the specific occurrence
	// of the problem, per RFC 9457 Section 3.1.2.
	Instance string `json:"instance,omitempty"`

	// Extensions contains additional members beyond the standard fields.
	// Per RFC 9457 Section 3.2, problem types may extend the object with
	// additional members that provide further context.
	Extensions map[string]any `json:"-"`
}

// MarshalJSON implements json.Marshaler. It serializes the standard fields
// and merges any extension members into the top-level JSON object, per
// RFC 9457 Section 3.2.
func (p ProblemDetails) MarshalJSON() ([]byte, error) {
	typ := p.Type
	if typ == "" {
		typ = "about:blank"
	}

	base := problemDetailsJSON{
		Type:     typ,
		Title:    p.Title,
		Status:   p.Status,
		Detail:   p.Detail,
		Instance: p.Instance,
	}

	if len(p.Extensions) == 0 {
		return json.Marshal(base)
	}

	baseBytes, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}

	extBytes, err := json.Marshal(p.Extensions)
	if err != nil {
		return nil, err
	}

	// Merge: strip trailing '}' from base and leading '{' from extensions,
	// then join with a comma.
	merged := make([]byte, 0, len(baseBytes)+len(extBytes))
	merged = append(merged, baseBytes[:len(baseBytes)-1]...)
	merged = append(merged, ',')
	merged = append(merged, extBytes[1:]...)

	return merged, nil
}

// problemDetailsJSON is the internal struct used for standard field serialization.
type problemDetailsJSON struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// WriteProblemDetails writes an RFC 9457 Problem Details JSON response.
// It sets Content-Type to "application/problem+json" and writes the
// status code from the ProblemDetails struct.
// If encoding fails, an HTTP 500 Internal Server Error is written instead.
func WriteProblemDetails(w http.ResponseWriter, problem ProblemDetails) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(problem); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", mux.ContentTypeApplicationProblemJSON)
	w.WriteHeader(problem.Status)
	w.Write(buf.Bytes()) //nolint:errcheck
}

// NewProblemDetails creates a ProblemDetails with the given status code
// and the standard status text as title. Type defaults to "about:blank"
// per RFC 9457 Section 4.2.
func NewProblemDetails(status int) ProblemDetails {
	return ProblemDetails{
		Status: status,
		Title:  http.StatusText(status),
	}
}
