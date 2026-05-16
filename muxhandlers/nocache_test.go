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

func TestNoCachePreservesOptionalInterfaces(t *testing.T) {
	t.Run("Flush is forwarded and applies headers first", func(t *testing.T) {
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder()}
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.(http.Flusher).Flush()
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.True(t, full.flushed)
		assert.Equal(t, "no-store", full.Header().Get("Cache-Control"))
	})

	t.Run("ResponseController.Flush works via Unwrap", func(t *testing.T) {
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder()}
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			assert.NoError(t, http.NewResponseController(w).Flush())
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.True(t, full.flushed)
	})

	t.Run("Hijack is forwarded", func(t *testing.T) {
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder()}
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			assert.NoError(t, err)
			if conn != nil {
				_ = conn.Close()
			}
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.True(t, full.hijacked)
	})

	t.Run("Push is forwarded", func(t *testing.T) {
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder(), pushSupport: true}
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			assert.NoError(t, w.(http.Pusher).Push("/style.css", nil))
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, "/style.css", full.pushed)
	})

	t.Run("optional interfaces are advertised only when the underlying writer supports them", func(t *testing.T) {
		// A bare httptest.NewRecorder implements Flusher but not
		// Hijacker or Pusher. The wrapper must mirror that exactly:
		// type assertions for absent capabilities must return ok=false,
		// matching what the handler would see if no middleware were
		// applied.
		recorder := httptest.NewRecorder()
		_, recorderHasFlush := http.ResponseWriter(recorder).(http.Flusher)
		_, recorderHasHijack := http.ResponseWriter(recorder).(http.Hijacker)
		_, recorderHasPush := http.ResponseWriter(recorder).(http.Pusher)
		assert.True(t, recorderHasFlush, "httptest.NewRecorder should implement Flusher")
		assert.False(t, recorderHasHijack)
		assert.False(t, recorderHasPush)

		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, hasFlush := w.(http.Flusher)
			_, hasHijack := w.(http.Hijacker)
			_, hasPush := w.(http.Pusher)
			assert.Equal(t, recorderHasFlush, hasFlush, "Flusher capability must match baseline")
			assert.Equal(t, recorderHasHijack, hasHijack, "Hijacker must not be advertised when underlying writer lacks it")
			assert.Equal(t, recorderHasPush, hasPush, "Pusher must not be advertised when underlying writer lacks it")
		}))
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	})

	t.Run("Unwrap exposes the embedded writer", func(t *testing.T) {
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder()}
		mw := NoCacheMiddleware(mux.NewRouter(), NoCacheConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			type unwrapper interface{ Unwrap() http.ResponseWriter }
			u, ok := w.(unwrapper)
			if assert.True(t, ok, "wrapper must implement Unwrap") {
				assert.Same(t, http.ResponseWriter(full), u.Unwrap())
			}
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
	})
}

func TestNoCacheWrapVariants(t *testing.T) {
	// Drive noCacheWrap through every non-empty subset of
	// {Flusher, Hijacker, Pusher} plus the no-capability base case,
	// asserting the wrapper exposes exactly the same capabilities.
	cases := []struct {
		name                  string
		flush, hijack, push   bool
		wantFlush, wantHijack bool
		wantPush              bool
	}{
		{name: "none"},
		{name: "flush only", flush: true, wantFlush: true},
		{name: "hijack only", hijack: true, wantHijack: true},
		{name: "push only", push: true, wantPush: true},
		{name: "flush+hijack", flush: true, hijack: true, wantFlush: true, wantHijack: true},
		{name: "flush+push", flush: true, push: true, wantFlush: true, wantPush: true},
		{name: "hijack+push", hijack: true, push: true, wantHijack: true, wantPush: true},
		{
			name:       "all three",
			flush:      true,
			hijack:     true,
			push:       true,
			wantFlush:  true,
			wantHijack: true,
			wantPush:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := makeCapabilityWriter(tc.flush, tc.hijack, tc.push)
			_, wrapped := noCacheWrap(inner, func(http.Header) {})
			_, hasFlush := wrapped.(http.Flusher)
			_, hasHijack := wrapped.(http.Hijacker)
			_, hasPush := wrapped.(http.Pusher)
			assert.Equal(t, tc.wantFlush, hasFlush, "Flusher")
			assert.Equal(t, tc.wantHijack, hasHijack, "Hijacker")
			assert.Equal(t, tc.wantPush, hasPush, "Pusher")

			type unwrapper interface{ Unwrap() http.ResponseWriter }
			_, hasUnwrap := wrapped.(unwrapper)
			assert.True(t, hasUnwrap, "Unwrap must be exposed by every variant")
		})
	}
}
