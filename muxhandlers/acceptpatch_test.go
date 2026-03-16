package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vitalvas/kasper/mux"
)

func TestAcceptPatchMiddleware(t *testing.T) {
	t.Run("non-OPTIONS request passes through", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet, http.MethodPatch)

		mw := AcceptPatchMiddleware(r, AcceptPatchConfig{})
		r.Use(mw)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Header().Get("Accept-Patch"))
		assert.Empty(t, rec.Header().Get("Allow"))
	})

	t.Run("OPTIONS returns default Accept-Patch and Allow", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet, http.MethodPatch)

		mw := AcceptPatchMiddleware(r, AcceptPatchConfig{})
		r.Use(mw)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users/1", nil)
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "application/json, application/merge-patch+json, application/json-patch+json", rec.Header().Get("Accept-Patch"))
		assert.Contains(t, rec.Header().Get("Allow"), http.MethodGet)
		assert.Contains(t, rec.Header().Get("Allow"), http.MethodPatch)
	})

	t.Run("custom Accept-Patch types", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodPatch)

		mw := AcceptPatchMiddleware(r, AcceptPatchConfig{
			AcceptPatchTypes: []string{PatchTypeMergePatch},
		})
		r.Use(mw)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users/1", nil)
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "application/merge-patch+json", rec.Header().Get("Accept-Patch"))
	})

	t.Run("custom status code", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodPatch)

		mw := AcceptPatchMiddleware(r, AcceptPatchConfig{
			StatusCode: http.StatusOK,
		})
		r.Use(mw)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users/1", nil)
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.NotEmpty(t, rec.Header().Get("Accept-Patch"))
	})

	t.Run("OPTIONS on route without methods still returns Accept-Patch", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw := AcceptPatchMiddleware(r, AcceptPatchConfig{})
		r.Use(mw)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/health", nil)
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.NotEmpty(t, rec.Header().Get("Accept-Patch"))
		assert.Empty(t, rec.Header().Get("Allow"))
	})

	t.Run("multiple routes same path different methods", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodPatch)
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}).Methods(http.MethodDelete)

		mw := AcceptPatchMiddleware(r, AcceptPatchConfig{})
		r.Use(mw)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users/1", nil)
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		allow := rec.Header().Get("Allow")
		assert.Contains(t, allow, http.MethodGet)
		assert.Contains(t, allow, http.MethodPatch)
		assert.Contains(t, allow, http.MethodDelete)
	})
}

func BenchmarkAcceptPatchMiddleware(b *testing.B) {
	r := mux.NewRouter()
	r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet, http.MethodPatch, http.MethodDelete)

	mw := AcceptPatchMiddleware(r, AcceptPatchConfig{})
	r.Use(mw)

	req := httptest.NewRequest(http.MethodOptions, "/users/1", nil)

	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}
