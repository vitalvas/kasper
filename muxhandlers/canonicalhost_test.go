package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalHostMiddleware(t *testing.T) {
	t.Run("empty URL returns error", func(t *testing.T) {
		_, err := CanonicalHostMiddleware(CanonicalHostConfig{})
		assert.ErrorIs(t, err, ErrCanonicalHostEmpty)
	})

	t.Run("invalid URL returns error", func(t *testing.T) {
		_, err := CanonicalHostMiddleware(CanonicalHostConfig{URL: "://bad"})
		assert.ErrorIs(t, err, ErrCanonicalHostInvalid)
	})

	t.Run("URL without scheme returns error", func(t *testing.T) {
		_, err := CanonicalHostMiddleware(CanonicalHostConfig{URL: "www.example.com"})
		assert.ErrorIs(t, err, ErrCanonicalHostInvalid)
	})

	t.Run("URL without host returns error", func(t *testing.T) {
		_, err := CanonicalHostMiddleware(CanonicalHostConfig{URL: "https://"})
		assert.ErrorIs(t, err, ErrCanonicalHostInvalid)
	})

	t.Run("redirects non-matching host", func(t *testing.T) {
		mw, err := CanonicalHostMiddleware(CanonicalHostConfig{
			URL: "https://www.example.com",
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example.com/page?q=1", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusMovedPermanently, rec.Code)
		assert.Equal(t, "https://www.example.com/page?q=1", rec.Header().Get("Location"))
	})

	t.Run("redirects non-matching scheme", func(t *testing.T) {
		mw, err := CanonicalHostMiddleware(CanonicalHostConfig{
			URL: "https://example.com",
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusMovedPermanently, rec.Code)
		assert.Equal(t, "https://example.com/path", rec.Header().Get("Location"))
	})

	t.Run("passes through matching request", func(t *testing.T) {
		mw, err := CanonicalHostMiddleware(CanonicalHostConfig{
			URL: "https://www.example.com",
		})
		require.NoError(t, err)

		var called bool
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://www.example.com/page", nil)
		req.Host = "www.example.com"
		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("custom status code", func(t *testing.T) {
		mw, err := CanonicalHostMiddleware(CanonicalHostConfig{
			URL:        "https://www.example.com",
			StatusCode: http.StatusPermanentRedirect,
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusPermanentRedirect, rec.Code)
	})

	t.Run("preserves path and query", func(t *testing.T) {
		mw, err := CanonicalHostMiddleware(CanonicalHostConfig{
			URL: "https://www.example.com",
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://old.example.com/api/v1/users?page=2&limit=10", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusMovedPermanently, rec.Code)
		assert.Equal(t, "https://www.example.com/api/v1/users?page=2&limit=10", rec.Header().Get("Location"))
	})

	t.Run("canonical host with port", func(t *testing.T) {
		mw, err := CanonicalHostMiddleware(CanonicalHostConfig{
			URL: "https://www.example.com:8443",
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusMovedPermanently, rec.Code)
		assert.Equal(t, "https://www.example.com:8443/", rec.Header().Get("Location"))
	})
}

func BenchmarkCanonicalHostMiddleware(b *testing.B) {
	mw, err := CanonicalHostMiddleware(CanonicalHostConfig{
		URL: "https://www.example.com",
	})
	if err != nil {
		b.Fatal(err)
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/page", nil)

	b.ResetTimer()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}
