package muxhandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vitalvas/kasper/mux"
)

var (
	uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	uuidV7Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

func TestRequestIDMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		config         RequestIDConfig
		incomingHeader string
		wantHeader     string
		wantGenerated  bool
	}{
		{
			name:          "generates UUID v4 by default",
			config:        RequestIDConfig{},
			wantGenerated: true,
		},
		{
			name:           "does not trust incoming by default",
			config:         RequestIDConfig{},
			incomingHeader: "existing-id",
			wantGenerated:  true,
		},
		{
			name:           "trusts incoming when configured",
			config:         RequestIDConfig{TrustIncoming: true},
			incomingHeader: "existing-id",
			wantHeader:     "existing-id",
		},
		{
			name:          "generates when trust incoming but no header",
			config:        RequestIDConfig{TrustIncoming: true},
			wantGenerated: true,
		},
		{
			name:       "custom generate func",
			config:     RequestIDConfig{GenerateFunc: func(_ *http.Request) string { return "custom-id" }},
			wantHeader: "custom-id",
		},
		{
			name:       "custom header name",
			config:     RequestIDConfig{HeaderName: "X-Trace-ID", GenerateFunc: func(_ *http.Request) string { return "trace-123" }},
			wantHeader: "trace-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedRequestHeader string

			headerName := tt.config.HeaderName
			if headerName == "" {
				headerName = "X-Request-ID"
			}

			r := mux.NewRouter()
			r.HandleFunc("/test", func(_ http.ResponseWriter, req *http.Request) {
				capturedRequestHeader = req.Header.Get(headerName)
			}).Methods(http.MethodGet)
			r.Use(RequestIDMiddleware(tt.config))

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.incomingHeader != "" {
				req.Header.Set(headerName, tt.incomingHeader)
			}
			r.ServeHTTP(w, req)

			responseHeader := w.Header().Get(headerName)

			if tt.wantGenerated {
				assert.Regexp(t, uuidV4Regex, responseHeader)
				assert.Regexp(t, uuidV4Regex, capturedRequestHeader)
			} else {
				assert.Equal(t, tt.wantHeader, responseHeader)
				assert.Equal(t, tt.wantHeader, capturedRequestHeader)
			}

			assert.Equal(t, capturedRequestHeader, responseHeader)
		})
	}

	t.Run("each request gets unique ID", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(RequestIDMiddleware(RequestIDConfig{}))

		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/test", nil))

		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/test", nil))

		id1 := w1.Header().Get("X-Request-ID")
		id2 := w2.Header().Get("X-Request-ID")

		assert.NotEmpty(t, id1)
		assert.NotEmpty(t, id2)
		assert.NotEqual(t, id1, id2)
	})

	t.Run("generate func receives request", func(t *testing.T) {
		var capturedPath string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(RequestIDMiddleware(RequestIDConfig{
			GenerateFunc: func(r *http.Request) string {
				capturedPath = r.URL.Path
				return "path-based-id"
			},
		}))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		assert.Equal(t, "/test", capturedPath)
		assert.Equal(t, "path-based-id", w.Header().Get("X-Request-ID"))
	})

	t.Run("empty id does not set headers", func(t *testing.T) {
		var capturedRequestHeader string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, req *http.Request) {
			capturedRequestHeader = req.Header.Get("X-Request-ID")
		}).Methods(http.MethodGet)
		r.Use(RequestIDMiddleware(RequestIDConfig{
			GenerateFunc: func(_ *http.Request) string { return "" },
		}))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		assert.Empty(t, capturedRequestHeader)
		assert.Empty(t, w.Header().Get("X-Request-ID"))
	})

	t.Run("id available via context", func(t *testing.T) {
		var capturedCtxID string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, req *http.Request) {
			capturedCtxID = RequestIDFromContext(req.Context())
		}).Methods(http.MethodGet)
		r.Use(RequestIDMiddleware(RequestIDConfig{}))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		assert.NotEmpty(t, capturedCtxID)
		assert.Equal(t, w.Header().Get("X-Request-ID"), capturedCtxID)
	})

	t.Run("empty id not in context", func(t *testing.T) {
		var capturedCtxID string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, req *http.Request) {
			capturedCtxID = RequestIDFromContext(req.Context())
		}).Methods(http.MethodGet)
		r.Use(RequestIDMiddleware(RequestIDConfig{
			GenerateFunc: func(_ *http.Request) string { return "" },
		}))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		assert.Empty(t, capturedCtxID)
	})
}

func TestRequestIDFromContext(t *testing.T) {
	t.Run("returns empty for bare context", func(t *testing.T) {
		assert.Empty(t, RequestIDFromContext(context.Background()))
	})
}

func TestGenerateUUIDv4(t *testing.T) {
	t.Run("format", func(t *testing.T) {
		id := GenerateUUIDv4(nil)
		assert.Regexp(t, uuidV4Regex, id)
		assert.Len(t, id, 36)
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := make(map[string]struct{}, 100)
		for range 100 {
			id := GenerateUUIDv4(nil)
			_, exists := seen[id]
			assert.False(t, exists, "duplicate UUID generated: %s", id)
			seen[id] = struct{}{}
		}
	})
}

func TestGenerateUUIDv7(t *testing.T) {
	t.Run("format", func(t *testing.T) {
		id := GenerateUUIDv7(nil)
		assert.Regexp(t, uuidV7Regex, id)
		assert.Len(t, id, 36)
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := make(map[string]struct{}, 100)
		for range 100 {
			id := GenerateUUIDv7(nil)
			_, exists := seen[id]
			assert.False(t, exists, "duplicate UUID generated: %s", id)
			seen[id] = struct{}{}
		}
	})

	t.Run("time ordered", func(t *testing.T) {
		id1 := GenerateUUIDv7(nil)
		time.Sleep(2 * time.Millisecond)
		id2 := GenerateUUIDv7(nil)

		assert.Less(t, id1, id2)
	})

	t.Run("middleware with GenerateUUIDv7", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(RequestIDMiddleware(RequestIDConfig{
			GenerateFunc: GenerateUUIDv7,
		}))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		assert.Regexp(t, uuidV7Regex, w.Header().Get("X-Request-ID"))
	})
}

func BenchmarkRequestIDMiddleware(b *testing.B) {
	b.Run("default generator", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(RequestIDMiddleware(RequestIDConfig{}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("uuid v7 generator", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(RequestIDMiddleware(RequestIDConfig{GenerateFunc: GenerateUUIDv7}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("trust incoming", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(RequestIDMiddleware(RequestIDConfig{TrustIncoming: true}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-ID", "pre-existing-id")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
