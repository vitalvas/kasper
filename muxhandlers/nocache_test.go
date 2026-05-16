package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vitalvas/kasper/mux"
)

func TestNoCacheMiddleware(t *testing.T) {
	t.Run("modern preset sets only Cache-Control: no-store", func(t *testing.T) {
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Empty(t, w.Header().Get("Pragma"))
		assert.Empty(t, w.Header().Get("Expires"))
	})

	t.Run("strict preset sets Cache-Control, Pragma, and Expires", func(t *testing.T) {
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{Preset: NoCachePresetStrict})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, noCacheStrictValue, w.Header().Get("Cache-Control"))
		assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
		assert.Equal(t, "0", w.Header().Get("Expires"))
	})

	t.Run("strips ETag and Last-Modified set by handler", func(t *testing.T) {
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("ETag", `"abc123"`)
			w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Empty(t, w.Header().Get("ETag"))
		assert.Empty(t, w.Header().Get("Last-Modified"))
	})

	t.Run("overrides handler-set Cache-Control", func(t *testing.T) {
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=3600")
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	})

	t.Run("strict preset overrides handler-set Pragma and Expires", func(t *testing.T) {
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{Preset: NoCachePresetStrict})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Pragma", "public")
			w.Header().Set("Expires", "Wed, 21 Oct 2099 07:28:00 GMT")
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
		assert.Equal(t, "0", w.Header().Get("Expires"))
	})

	t.Run("applies headers when handler writes body without WriteHeader", func(t *testing.T) {
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("hello"))
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Equal(t, "hello", w.Body.String())
	})

	t.Run("applies headers when handler writes nothing", func(t *testing.T) {
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			// no write, no WriteHeader
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	})

	t.Run("Skip predicate leaves handler headers untouched", func(t *testing.T) {
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{
			Skip: func(_ *mux.Router, r *http.Request) bool { return r.URL.Path == "/assets/style.css" },
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			w.Header().Set("ETag", `"v1"`)
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/style.css", nil))

		// Untouched because the predicate matched.
		assert.Equal(t, "public, max-age=86400", w.Header().Get("Cache-Control"))
		assert.Equal(t, `"v1"`, w.Header().Get("ETag"))
	})

	t.Run("Skip predicate can read matched route metadata", func(t *testing.T) {
		const allowCacheKey = "allow_cache"

		r := mux.NewRouter()
		r.Use(NoCacheMiddleware(r, NoCacheConfig{
			Skip: func(_ *mux.Router, req *http.Request) bool {
				route := mux.CurrentRoute(req)
				if route == nil {
					return false
				}
				allow, _ := route.GetMetadataValueOr(allowCacheKey, false).(bool)
				return allow
			},
		}))
		r.HandleFunc("/cached", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=60")
			w.WriteHeader(http.StatusOK)
		}).Metadata(allowCacheKey, true)
		r.HandleFunc("/dynamic", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=60")
			w.WriteHeader(http.StatusOK)
		})

		cached := httptest.NewRecorder()
		r.ServeHTTP(cached, httptest.NewRequest(http.MethodGet, "/cached", nil))
		assert.Equal(t, "public, max-age=60", cached.Header().Get("Cache-Control"))

		dynamic := httptest.NewRecorder()
		r.ServeHTTP(dynamic, httptest.NewRequest(http.MethodGet, "/dynamic", nil))
		assert.Equal(t, "no-store", dynamic.Header().Get("Cache-Control"))
	})

	t.Run("WriteHeader called twice does not re-apply headers", func(t *testing.T) {
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.WriteHeader(http.StatusAccepted) // stdlib ignores
			_, _ = w.Write([]byte("body"))
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "body", w.Body.String())
	})
}
