package e2ee

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestProblemFromError(t *testing.T) {
	tests := []struct {
		err        error
		wantType   string
		wantStatus int
		wantTitle  string
	}{
		{ErrKeyUnknown, "urn:ietf:params:e2ee:error:key_unknown", http.StatusBadRequest, "Key Unknown"},
		{ErrKeyExpired, "urn:ietf:params:e2ee:error:key_expired", http.StatusBadRequest, "Key Expired"},
		{ErrAEADUnsupported, "urn:ietf:params:e2ee:error:aead_unsupported", http.StatusBadRequest, "AEAD Unsupported"},
		{ErrDecryptFailed, "urn:ietf:params:e2ee:error:decrypt_failed", http.StatusBadRequest, "Decryption Failed"},
		{ErrTimestampSkew, "urn:ietf:params:e2ee:error:timestamp_skew", http.StatusBadRequest, "Timestamp Out Of Range"},
		{ErrReplayDetected, "urn:ietf:params:e2ee:error:replay_detected", http.StatusTooEarly, "Replay Detected"},
		{ErrMalformed, "urn:ietf:params:e2ee:error:malformed", http.StatusBadRequest, "Malformed Request"},
	}

	for _, tt := range tests {
		t.Run(tt.wantTitle, func(t *testing.T) {
			prob := ProblemFromError(tt.err)
			assert.Equal(t, tt.wantType, prob.Type)
			assert.Equal(t, tt.wantStatus, prob.Status)
			assert.Equal(t, tt.wantTitle, prob.Title)
		})
	}
}

func TestProblemFromErrorWrapped(t *testing.T) {
	wrapped := fmt.Errorf("context: %w", ErrReplayDetected)
	prob := ProblemFromError(wrapped)
	assert.Equal(t, "urn:ietf:params:e2ee:error:replay_detected", prob.Type)
}

func TestProblemFromUnknownError(t *testing.T) {
	prob := ProblemFromError(fmt.Errorf("internal database failure"))
	assert.Equal(t, "urn:ietf:params:e2ee:error:malformed", prob.Type)
	assert.Equal(t, http.StatusBadRequest, prob.Status)
	// Internal details must not leak.
	assert.NotContains(t, prob.Title, "database")
}

func TestWriteProblem(t *testing.T) {
	rec := httptest.NewRecorder()

	prob := WriteProblem(rec, ErrKeyExpired)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, mux.ContentTypeApplicationProblemJSON, rec.Header().Get("Content-Type"))
	assert.Equal(t, http.StatusBadRequest, prob.Status)

	var decoded Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &decoded))
	assert.Equal(t, "urn:ietf:params:e2ee:error:key_expired", decoded.Type)
	assert.Equal(t, "Key Expired", decoded.Title)
}
