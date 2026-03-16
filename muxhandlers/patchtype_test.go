package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPatchRoutingMiddleware(t *testing.T) {
	t.Run("non-PATCH request passes through", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{})

		var called bool
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			assert.Empty(t, PatchContentType(r))
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("PATCH with application/json", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, PatchTypeJSON, PatchContentType(r))
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("PATCH with merge-patch+json", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, PatchTypeMergePatch, PatchContentType(r))
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		req.Header.Set("Content-Type", "application/merge-patch+json")
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("PATCH with json-patch+json", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, PatchTypeJSONPatch, PatchContentType(r))
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		req.Header.Set("Content-Type", "application/json-patch+json")
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("PATCH with charset parameter is accepted", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, PatchTypeMergePatch, PatchContentType(r))
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		req.Header.Set("Content-Type", "application/merge-patch+json; charset=utf-8")
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("PATCH with unsupported content type returns 415", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		req.Header.Set("Content-Type", "text/plain")
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	})

	t.Run("PATCH with missing content type returns 415", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	})

	t.Run("PATCH with malformed content type returns 415", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		req.Header.Set("Content-Type", ";;;")
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	})

	t.Run("custom allowed types", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{
			AllowedTypes: []string{PatchTypeMergePatch},
		})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		mw := PatchRoutingMiddleware(PatchRoutingConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, PatchTypeMergePatch, PatchContentType(r))
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		req.Header.Set("Content-Type", "Application/Merge-Patch+JSON")
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("PatchContentType returns empty without middleware", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/", nil)
		assert.Empty(t, PatchContentType(req))
	})
}

func BenchmarkPatchRoutingMiddleware(b *testing.B) {
	mw := PatchRoutingMiddleware(PatchRoutingConfig{})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPatch, "/", nil)
	req.Header.Set("Content-Type", "application/merge-patch+json")

	b.ResetTimer()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}
