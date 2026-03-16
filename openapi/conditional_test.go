package openapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestSchemaETag(t *testing.T) {
	t.Run("JSON endpoint returns ETag by default", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, []string{})
		spec.Handle(r, "/swagger", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/swagger/schema.json", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		etag := w.Header().Get("ETag")
		assert.NotEmpty(t, etag)
		assert.True(t, etag[0] == '"')
	})

	t.Run("YAML endpoint returns ETag by default", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, []string{})
		spec.Handle(r, "/swagger", &HandleConfig{YAMLFilename: "schema.yaml"})

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/swagger/schema.yaml", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Header().Get("ETag"))
	})

	t.Run("ETag is consistent across requests", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, []string{})
		spec.Handle(r, "/swagger", nil)

		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/swagger/schema.json", nil))

		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/swagger/schema.json", nil))

		assert.Equal(t, w1.Header().Get("ETag"), w2.Header().Get("ETag"))
	})

	t.Run("If-None-Match returns 304", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, []string{})
		spec.Handle(r, "/swagger", nil)

		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/swagger/schema.json", nil))
		etag := w1.Header().Get("ETag")
		require.NotEmpty(t, etag)

		req := httptest.NewRequest(http.MethodGet, "/swagger/schema.json", nil)
		req.Header.Set("If-None-Match", etag)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req)

		assert.Equal(t, http.StatusNotModified, w2.Code)
		assert.Empty(t, w2.Body.String())
		assert.Equal(t, etag, w2.Header().Get("ETag"))
	})

	t.Run("If-None-Match mismatch returns 200", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, []string{})
		spec.Handle(r, "/swagger", nil)

		req := httptest.NewRequest(http.MethodGet, "/swagger/schema.json", nil)
		req.Header.Set("If-None-Match", `"wrong"`)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Body.String())
	})

	t.Run("If-None-Match wildcard returns 304", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, []string{})
		spec.Handle(r, "/swagger", nil)

		req := httptest.NewRequest(http.MethodGet, "/swagger/schema.json", nil)
		req.Header.Set("If-None-Match", "*")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotModified, w.Code)
	})

	t.Run("DisableETag suppresses ETag header", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, []string{})
		spec.Handle(r, "/swagger", &HandleConfig{DisableETag: true})

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/swagger/schema.json", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, w.Header().Get("ETag"))
	})

	t.Run("YAML If-None-Match returns 304", func(t *testing.T) {
		r := mux.NewRouter()
		spec := NewSpec(Info{Title: "Test", Version: "1.0.0"})
		spec.Route(r.HandleFunc("/items", dummyHandler).Methods(http.MethodGet)).
			Response(http.StatusOK, []string{})
		spec.Handle(r, "/swagger", &HandleConfig{YAMLFilename: "schema.yaml"})

		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/swagger/schema.yaml", nil))
		etag := w1.Header().Get("ETag")
		require.NotEmpty(t, etag)

		req := httptest.NewRequest(http.MethodGet, "/swagger/schema.yaml", nil)
		req.Header.Set("If-None-Match", etag)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req)

		assert.Equal(t, http.StatusNotModified, w2.Code)
	})
}
