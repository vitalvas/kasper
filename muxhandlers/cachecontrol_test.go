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

func TestCacheControlMiddleware(t *testing.T) {
	t.Run("empty rules returns error", func(t *testing.T) {
		_, err := CacheControlMiddleware(CacheControlConfig{})
		require.ErrorIs(t, err, ErrNoCacheControlRules)
	})

	t.Run("exact content type match sets header", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "application/json", Value: "no-cache", Expires: 0},
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		before := time.Now().UTC()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
		assertExpiresInRange(t, w.Header().Get("Expires"), before, 0)
	})

	t.Run("prefix match", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "image/", Value: "public, max-age=86400", Expires: 24 * time.Hour},
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		before := time.Now().UTC()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "public, max-age=86400", w.Header().Get("Cache-Control"))
		assertExpiresInRange(t, w.Header().Get("Expires"), before, 24*time.Hour)
	})

	t.Run("first matching rule wins", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "image/svg", Value: "public, max-age=3600"},
				{ContentType: "image/", Value: "public, max-age=86400"},
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
	})

	t.Run("unmatched type with default value", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "image/", Value: "public, max-age=86400"},
			},
			DefaultValue:   "no-store",
			DefaultExpires: 0,
		})
		require.NoError(t, err)
		r.Use(mw)

		before := time.Now().UTC()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assertExpiresInRange(t, w.Header().Get("Expires"), before, 0)
	})

	t.Run("unmatched type without default value sets no header", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "image/", Value: "public, max-age=86400", Expires: -1},
			},
			DefaultExpires: -1,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Cache-Control"))
		assert.Empty(t, w.Header().Get("Expires"))
	})

	t.Run("handler-set headers are not overwritten", func(t *testing.T) {
		tests := []struct {
			name           string
			handlerCC      string
			handlerExpires string
			wantCC         string
			wantExact      bool
			wantExpires    string
		}{
			{
				name:      "cache-control preset",
				handlerCC: "private, max-age=60",
				wantCC:    "private, max-age=60",
			},
			{
				name:           "expires preset",
				handlerExpires: "Thu, 01 Jan 2026 00:00:00 GMT",
				wantCC:         "no-cache",
				wantExact:      true,
				wantExpires:    "Thu, 01 Jan 2026 00:00:00 GMT",
			},
			{
				name:           "both preset",
				handlerCC:      "private",
				handlerExpires: "Thu, 01 Jan 2026 00:00:00 GMT",
				wantCC:         "private",
				wantExact:      true,
				wantExpires:    "Thu, 01 Jan 2026 00:00:00 GMT",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				r := mux.NewRouter()
				r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if tt.handlerCC != "" {
						w.Header().Set("Cache-Control", tt.handlerCC)
					}
					if tt.handlerExpires != "" {
						w.Header().Set("Expires", tt.handlerExpires)
					}
					w.WriteHeader(http.StatusOK)
				}).Methods(http.MethodGet)

				mw, err := CacheControlMiddleware(CacheControlConfig{
					Rules: []CacheControlRule{
						{ContentType: "application/json", Value: "no-cache", Expires: 0},
					},
				})
				require.NoError(t, err)
				r.Use(mw)

				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				r.ServeHTTP(w, req)

				assert.Equal(t, tt.wantCC, w.Header().Get("Cache-Control"))

				if tt.wantExact {
					assert.Equal(t, tt.wantExpires, w.Header().Get("Expires"))
				}
			})
		}
	})

	t.Run("negative expires sets no expires header", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "application/json", Value: "no-cache", Expires: -1},
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
		assert.Empty(t, w.Header().Get("Expires"))
	})

	t.Run("case-insensitive matching", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "Application/JSON")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "application/json", Value: "no-cache", Expires: -1},
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	})

	t.Run("content type with parameters", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "application/json", Value: "no-cache", Expires: -1},
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	})

	t.Run("implicit write header on write", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<h1>hello</h1>"))
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "text/html", Value: "no-store", Expires: -1},
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	})
}

// assertExpiresInRange parses the Expires header as an HTTP-date and checks
// that it falls within the expected range: [before+offset, before+offset+2s].
func assertExpiresInRange(t *testing.T, header string, before time.Time, offset time.Duration) {
	t.Helper()

	require.NotEmpty(t, header, "Expires header must be set")

	got, err := time.Parse(http.TimeFormat, header)
	require.NoError(t, err, "Expires header must be a valid HTTP-date")

	earliest := before.Add(offset).Truncate(time.Second)
	latest := earliest.Add(2 * time.Second)

	assert.False(t, got.Before(earliest), "Expires %v is before earliest %v", got, earliest)
	assert.False(t, got.After(latest), "Expires %v is after latest %v", got, latest)
}

func BenchmarkCacheControlMiddleware(b *testing.B) {
	b.Run("matching rule", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "application/json", Value: "no-cache", Expires: 0},
			},
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
	})

	b.Run("no match", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := CacheControlMiddleware(CacheControlConfig{
			Rules: []CacheControlRule{
				{ContentType: "image/", Value: "public, max-age=86400", Expires: -1},
			},
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
	})
}
