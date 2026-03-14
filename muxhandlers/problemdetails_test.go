package muxhandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestWriteProblemDetails(t *testing.T) {
	t.Run("basic problem details", func(t *testing.T) {
		w := httptest.NewRecorder()

		WriteProblemDetails(w, ProblemDetails{
			Type:   "https://example.com/errors/not-found",
			Title:  "Resource not found",
			Status: http.StatusNotFound,
			Detail: "User with ID 42 was not found",
		})

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, mux.ContentTypeApplicationProblemJSON, w.Header().Get("Content-Type"))

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "https://example.com/errors/not-found", body["type"])
		assert.Equal(t, "Resource not found", body["title"])
		assert.Equal(t, float64(http.StatusNotFound), body["status"])
		assert.Equal(t, "User with ID 42 was not found", body["detail"])
	})

	t.Run("defaults type to about:blank", func(t *testing.T) {
		w := httptest.NewRecorder()

		WriteProblemDetails(w, ProblemDetails{
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
		})

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "about:blank", body["type"])
	})

	t.Run("omits empty detail and instance", func(t *testing.T) {
		w := httptest.NewRecorder()

		WriteProblemDetails(w, ProblemDetails{
			Title:  "Internal Server Error",
			Status: http.StatusInternalServerError,
		})

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		_, hasDetail := body["detail"]
		_, hasInstance := body["instance"]
		assert.False(t, hasDetail)
		assert.False(t, hasInstance)
	})

	t.Run("includes instance", func(t *testing.T) {
		w := httptest.NewRecorder()

		WriteProblemDetails(w, ProblemDetails{
			Type:     "https://example.com/errors/conflict",
			Title:    "Conflict",
			Status:   http.StatusConflict,
			Instance: "/api/v1/users/42",
		})

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "/api/v1/users/42", body["instance"])
	})

	t.Run("includes extensions", func(t *testing.T) {
		w := httptest.NewRecorder()

		WriteProblemDetails(w, ProblemDetails{
			Type:   "https://example.com/errors/validation",
			Title:  "Validation Error",
			Status: http.StatusUnprocessableEntity,
			Detail: "One or more fields are invalid",
			Extensions: map[string]any{
				"errors": []map[string]string{
					{"field": "email", "message": "invalid format"},
					{"field": "age", "message": "must be positive"},
				},
				"traceId": "abc-123",
			},
		})

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "https://example.com/errors/validation", body["type"])
		assert.Equal(t, "Validation Error", body["title"])
		assert.Equal(t, "abc-123", body["traceId"])

		errors, ok := body["errors"].([]any)
		require.True(t, ok)
		assert.Len(t, errors, 2)
	})

	t.Run("extensions do not override standard fields", func(t *testing.T) {
		w := httptest.NewRecorder()

		WriteProblemDetails(w, ProblemDetails{
			Type:   "https://example.com/errors/test",
			Title:  "Original Title",
			Status: http.StatusBadRequest,
			Extensions: map[string]any{
				"extra": "value",
			},
		})

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "Original Title", body["title"])
		assert.Equal(t, "value", body["extra"])
	})
}

func TestNewProblemDetails(t *testing.T) {
	t.Run("creates with status and title", func(t *testing.T) {
		p := NewProblemDetails(http.StatusNotFound)
		assert.Equal(t, http.StatusNotFound, p.Status)
		assert.Equal(t, "Not Found", p.Title)
		assert.Empty(t, p.Type)
		assert.Empty(t, p.Detail)
		assert.Empty(t, p.Instance)
		assert.Nil(t, p.Extensions)
	})

	t.Run("serializes with about:blank type", func(t *testing.T) {
		w := httptest.NewRecorder()
		WriteProblemDetails(w, NewProblemDetails(http.StatusForbidden))

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "about:blank", body["type"])
		assert.Equal(t, "Forbidden", body["title"])
		assert.Equal(t, float64(http.StatusForbidden), body["status"])
	})
}

func TestProblemDetailsMarshalJSON(t *testing.T) {
	t.Run("without extensions", func(t *testing.T) {
		p := ProblemDetails{
			Type:   "https://example.com/test",
			Title:  "Test",
			Status: 400,
		}

		data, err := json.Marshal(p)
		require.NoError(t, err)

		var body map[string]any
		require.NoError(t, json.Unmarshal(data, &body))
		assert.Equal(t, "https://example.com/test", body["type"])
		assert.Len(t, body, 3) // type, title, status
	})

	t.Run("with extensions", func(t *testing.T) {
		p := ProblemDetails{
			Type:   "https://example.com/test",
			Title:  "Test",
			Status: 400,
			Extensions: map[string]any{
				"retryAfter": 30,
			},
		}

		data, err := json.Marshal(p)
		require.NoError(t, err)

		var body map[string]any
		require.NoError(t, json.Unmarshal(data, &body))
		assert.Equal(t, float64(30), body["retryAfter"])
		assert.Len(t, body, 4)
	})

	t.Run("empty type defaults to about:blank", func(t *testing.T) {
		p := ProblemDetails{
			Title:  "Test",
			Status: 400,
		}

		data, err := json.Marshal(p)
		require.NoError(t, err)

		var body map[string]any
		require.NoError(t, json.Unmarshal(data, &body))
		assert.Equal(t, "about:blank", body["type"])
	})
}

func BenchmarkWriteProblemDetails(b *testing.B) {
	b.Run("without extensions", func(b *testing.B) {
		problem := ProblemDetails{
			Type:   "https://example.com/errors/not-found",
			Title:  "Not Found",
			Status: http.StatusNotFound,
			Detail: "Resource not found",
		}

		b.ResetTimer()
		for b.Loop() {
			WriteProblemDetails(httptest.NewRecorder(), problem)
		}
	})

	b.Run("with extensions", func(b *testing.B) {
		problem := ProblemDetails{
			Type:   "https://example.com/errors/validation",
			Title:  "Validation Error",
			Status: http.StatusUnprocessableEntity,
			Extensions: map[string]any{
				"errors":  []string{"field1", "field2"},
				"traceId": "abc-123",
			},
		}

		b.ResetTimer()
		for b.Loop() {
			WriteProblemDetails(httptest.NewRecorder(), problem)
		}
	})
}
