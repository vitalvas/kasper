package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestSunsetMiddleware(t *testing.T) {
	sunsetTime := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
	deprecationTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	t.Run("sunset header only", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/v1/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SunsetMiddleware(SunsetConfig{
			Sunset: sunsetTime,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "Wed, 31 Dec 2025 23:59:59 GMT", w.Header().Get("Sunset"))
		assert.Empty(t, w.Header().Get("Deprecation"))
		assert.Empty(t, w.Header().Get("Link"))
	})

	t.Run("sunset with deprecation", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/v1/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SunsetMiddleware(SunsetConfig{
			Sunset:      sunsetTime,
			Deprecation: deprecationTime,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "Wed, 31 Dec 2025 23:59:59 GMT", w.Header().Get("Sunset"))
		assert.Equal(t, "Sun, 01 Jun 2025 00:00:00 GMT", w.Header().Get("Deprecation"))
	})

	t.Run("sunset with link", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/v1/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SunsetMiddleware(SunsetConfig{
			Sunset: sunsetTime,
			Link:   "https://example.com/docs/migration",
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "Wed, 31 Dec 2025 23:59:59 GMT", w.Header().Get("Sunset"))
		assert.Equal(t, `<https://example.com/docs/migration>; rel="sunset"`, w.Header().Get("Link"))
	})

	t.Run("all fields", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/v1/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SunsetMiddleware(SunsetConfig{
			Sunset:      sunsetTime,
			Deprecation: deprecationTime,
			Link:        "https://example.com/docs/v2",
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "Wed, 31 Dec 2025 23:59:59 GMT", w.Header().Get("Sunset"))
		assert.Equal(t, "Sun, 01 Jun 2025 00:00:00 GMT", w.Header().Get("Deprecation"))
		assert.Equal(t, `<https://example.com/docs/v2>; rel="sunset"`, w.Header().Get("Link"))
	})

	t.Run("zero sunset time returns error", func(t *testing.T) {
		_, err := SunsetMiddleware(SunsetConfig{})
		assert.ErrorIs(t, err, ErrSunsetZeroTime)
	})

	t.Run("non-UTC time is converted to UTC", func(t *testing.T) {
		loc, err := time.LoadLocation("America/New_York")
		require.NoError(t, err)

		localTime := time.Date(2025, 12, 31, 18, 59, 59, 0, loc)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SunsetMiddleware(SunsetConfig{
			Sunset: localTime,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "Wed, 31 Dec 2025 23:59:59 GMT", w.Header().Get("Sunset"))
	})

	t.Run("link does not overwrite existing link header", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Add("Link", `<https://example.com/next>; rel="next"`)
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := SunsetMiddleware(SunsetConfig{
			Sunset: sunsetTime,
			Link:   "https://example.com/docs/migration",
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		links := w.Header().Values("Link")
		assert.Len(t, links, 2)
		assert.Contains(t, links, `<https://example.com/docs/migration>; rel="sunset"`)
		assert.Contains(t, links, `<https://example.com/next>; rel="next"`)
	})

	t.Run("headers set on every response", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet, http.MethodPost)

		mw, err := SunsetMiddleware(SunsetConfig{
			Sunset: sunsetTime,
		})
		require.NoError(t, err)
		r.Use(mw)

		for _, method := range []string{http.MethodGet, http.MethodPost} {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, "Wed, 31 Dec 2025 23:59:59 GMT", w.Header().Get("Sunset"))
		}
	})
}

func BenchmarkSunsetMiddleware(b *testing.B) {
	r := mux.NewRouter()
	r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	mw, err := SunsetMiddleware(SunsetConfig{
		Sunset:      time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
		Deprecation: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		Link:        "https://example.com/docs/v2",
	})
	if err != nil {
		b.Fatal(err)
	}
	r.Use(mw)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}
