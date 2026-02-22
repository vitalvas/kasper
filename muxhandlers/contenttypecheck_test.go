package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestContentTypeCheckMiddleware(t *testing.T) {
	t.Run("config validation", func(t *testing.T) {
		t.Run("empty allowed types", func(t *testing.T) {
			_, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{})
			assert.ErrorIs(t, err, ErrNoAllowedTypes)
		})

		t.Run("valid config", func(t *testing.T) {
			_, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
				AllowedTypes: []string{"application/json"},
			})
			assert.NoError(t, err)
		})
	})

	t.Run("matching type passes through", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("matching type with charset params passes through", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("non-matching type returns 415", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Content-Type", "text/plain")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	})

	t.Run("missing Content-Type on checked method returns 415", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	})

	t.Run("GET request skips check", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("custom methods list", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json"},
			Methods:      []string{http.MethodDelete},
		})
		require.NoError(t, err)
		r.Use(mw)

		t.Run("POST skips check with custom methods", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("DELETE requires Content-Type with custom methods", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
		})
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"Application/JSON"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("multiple allowed types", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json", "application/xml"},
		})
		require.NoError(t, err)
		r.Use(mw)

		t.Run("first type matches", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("second type matches", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set("Content-Type", "application/xml")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	})

	t.Run("PUT and PATCH checked by default", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json"},
		})
		require.NoError(t, err)
		r.Use(mw)

		t.Run("PUT without Content-Type returns 415", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
		})

		t.Run("PATCH without Content-Type returns 415", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
		})
	})
}

func BenchmarkContentTypeCheckMiddleware(b *testing.B) {
	b.Run("matching type", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json"},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		b.ResetTimer()
		for b.Loop() {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("non-matching type", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := ContentTypeCheckMiddleware(ContentTypeCheckConfig{
			AllowedTypes: []string{"application/json"},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		b.ResetTimer()
		for b.Loop() {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set("Content-Type", "text/plain")
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
