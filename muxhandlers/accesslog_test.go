package muxhandlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vitalvas/kasper/mux"
)

// newSlogCapture builds an slog logger whose JSON output is captured
// into the returned buffer, so tests can assert on emitted fields.
func newSlogCapture() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, buf
}

// decodeLogLine parses the first slog JSON line into a map for
// field-level assertions. Fails the test if no line is present.
func decodeLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatalf("expected slog output, got none")
	}
	// Take only the first record in case multiple were emitted.
	if i := strings.Index(line, "\n"); i >= 0 {
		line = line[:i]
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("decoding log line %q: %v", line, err)
	}
	return out
}

func TestAccessLogMiddleware(t *testing.T) {
	t.Run("emits structured slog entry with default fields", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{Logger: logger})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, "ok")
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/v1/users?page=2", nil)
		req.RemoteAddr = "10.0.0.1:54321"
		req.Host = "api.example.com"
		req.Header.Set("User-Agent", "test-agent/1.0")
		req.Header.Set("Referer", "https://example.com/source")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		entry := decodeLogLine(t, buf)
		assert.Equal(t, "http access", entry["msg"])
		assert.Equal(t, "INFO", entry["level"])
		assert.Equal(t, http.MethodPost, entry["method"])
		assert.Equal(t, "/api/v1/users", entry["path"])
		assert.Equal(t, float64(http.StatusCreated), entry["status"])
		assert.Equal(t, float64(2), entry["bytes"])
		assert.Equal(t, "10.0.0.1:54321", entry["remote_addr"])
		assert.Equal(t, "test-agent/1.0", entry["user_agent"])
		assert.Equal(t, "https://example.com/source", entry["referer"])
		assert.Equal(t, "page=2", entry["query"])
		assert.Equal(t, "HTTP/1.1", entry["proto"])
		assert.Equal(t, "http", entry["scheme"])
		assert.Equal(t, "api.example.com", entry["host"])
	})

	t.Run("resolves https scheme when TLS info is present", func(t *testing.T) {
		var captured *AccessLogEntry
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			LogFunc: func(e *AccessLogEntry) { captured = e },
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "https://api.example.com/secure", nil)
		req.TLS = &tls.ConnectionState{}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if assert.NotNil(t, captured) {
			assert.Equal(t, "https", captured.Scheme)
			assert.Equal(t, "api.example.com", captured.Host)
		}
	})

	t.Run("defaults status to 200 when handler does not write header", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{Logger: logger})
		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			// Handler completes silently.
		}))

		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/silent", nil))
		entry := decodeLogLine(t, buf)
		assert.Equal(t, float64(http.StatusOK), entry["status"])
		assert.Equal(t, float64(0), entry["bytes"])
	})

	t.Run("records implicit 200 when handler writes without WriteHeader", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{Logger: logger})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "hello")
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		entry := decodeLogLine(t, buf)
		assert.Equal(t, float64(http.StatusOK), entry["status"])
		assert.Equal(t, float64(5), entry["bytes"])
	})

	t.Run("LogFunc bypasses slog and receives full entry", func(t *testing.T) {
		var captured *AccessLogEntry
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			LogFunc: func(e *AccessLogEntry) { captured = e },
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		req := httptest.NewRequest(http.MethodGet, "/brew", nil)
		h.ServeHTTP(httptest.NewRecorder(), req)

		if assert.NotNil(t, captured) {
			assert.Equal(t, http.StatusTeapot, captured.Status)
			assert.Equal(t, http.MethodGet, captured.Method)
			assert.Equal(t, "/brew", captured.Path)
		}
	})

	t.Run("Skip predicate suppresses logging by path", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger: logger,
			Skip:   func(_ *mux.Router, r *http.Request) bool { return r.URL.Path == "/healthz" },
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
		assert.Empty(t, buf.String())
	})

	t.Run("Skip predicate can inspect route metadata via router", func(t *testing.T) {
		logger, buf := newSlogCapture()

		r := mux.NewRouter()
		const skipKey = "access_log_skip"
		r.Use(AccessLogMiddleware(r, AccessLogConfig{
			Logger: logger,
			Skip: func(_ *mux.Router, req *http.Request) bool {
				route := mux.CurrentRoute(req)
				if route == nil {
					return false
				}
				skip, _ := route.GetMetadataValueOr(skipKey, false).(bool)
				return skip
			},
		}))
		r.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Metadata(skipKey, true)
		r.HandleFunc("/api/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/metrics", nil))
		assert.Empty(t, buf.String(), "metadata-marked route should be skipped")

		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/users", nil))
		entry := decodeLogLine(t, buf)
		assert.Equal(t, "/api/users", entry["path"])
	})

	t.Run("5xx responses log at error level", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{Logger: logger})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		entry := decodeLogLine(t, buf)
		assert.Equal(t, "ERROR", entry["level"])
		assert.Equal(t, float64(http.StatusInternalServerError), entry["status"])
	})

	t.Run("SlowThreshold escalates Info to Warn", func(t *testing.T) {
		logger, buf := newSlogCapture()
		// Use injected clock so the handler appears to take 200ms.
		fixed := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
		var call int
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger:        logger,
			SlowThreshold: 100 * time.Millisecond,
			Now: func() time.Time {
				call++
				if call == 1 {
					return fixed
				}
				return fixed.Add(200 * time.Millisecond)
			},
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))
		entry := decodeLogLine(t, buf)
		assert.Equal(t, "WARN", entry["level"])
	})

	t.Run("SlowThreshold does not affect 5xx escalation", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger:        logger,
			SlowThreshold: time.Nanosecond, // any duration qualifies
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "fail", http.StatusBadGateway)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		entry := decodeLogLine(t, buf)
		assert.Equal(t, "ERROR", entry["level"])
	})

	t.Run("captures included headers and redacts sensitive ones", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger:         logger,
			IncludeHeaders: []string{"X-Tenant", "authorization", "x-custom-token"},
			RedactHeaders:  []string{"X-Custom-Token"},
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Tenant", "acme")
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("X-Custom-Token", "sensitive")
		h.ServeHTTP(httptest.NewRecorder(), req)

		entry := decodeLogLine(t, buf)
		headers, ok := entry["headers"].(map[string]any)
		if assert.True(t, ok, "headers group should be present") {
			assert.Equal(t, "acme", headers["X-Tenant"])
			assert.Equal(t, "[REDACTED]", headers["Authorization"])
			assert.Equal(t, "[REDACTED]", headers["X-Custom-Token"])
		}
	})

	t.Run("missing included header is omitted from output", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger:         logger,
			IncludeHeaders: []string{"X-Tenant"},
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		// "headers" group must be absent because no included header was present.
		entry := decodeLogLine(t, buf)
		_, has := entry["headers"]
		assert.False(t, has, "empty headers map should not be logged")
	})

	t.Run("route name is recorded when matched route is named", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{Logger: logger})

		r := mux.NewRouter()
		r.Use(mw)
		r.HandleFunc("/api/v1/users/{id:int}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Name("user-get")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/42", nil)
		r.ServeHTTP(httptest.NewRecorder(), req)

		entry := decodeLogLine(t, buf)
		assert.Equal(t, "user-get", entry["route"])
	})

	t.Run("request ID propagates into the log entry", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{Logger: logger})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := context.WithValue(req.Context(), requestIDKey{}, "abc-123")
		req = req.WithContext(ctx)
		h.ServeHTTP(httptest.NewRecorder(), req)
		entry := decodeLogLine(t, buf)
		assert.Equal(t, "abc-123", entry["request_id"])
	})

	t.Run("default logger is slog.Default when nothing is configured", func(t *testing.T) {
		// Replace slog.Default for the test, ensuring the middleware
		// reaches it when both Logger and LogFunc are nil.
		original := slog.Default()
		defer slog.SetDefault(original)

		buf := &bytes.Buffer{}
		slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))

		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		assert.NotEmpty(t, buf.String())
	})

	t.Run("WriteHeader called twice does not panic and keeps first status", func(t *testing.T) {
		logger, buf := newSlogCapture()
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{Logger: logger})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.WriteHeader(http.StatusAccepted) // ignored by stdlib
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		entry := decodeLogLine(t, buf)
		assert.Equal(t, float64(http.StatusCreated), entry["status"])
	})
}

func TestAccessLogCanonicalHeaderHelpers(t *testing.T) {
	t.Run("canonicalHeaderList drops empties and canonicalizes", func(t *testing.T) {
		got := canonicalHeaderList([]string{" x-tenant ", "", "AUTHORIZATION"})
		assert.Equal(t, []string{"X-Tenant", "Authorization"}, got)
	})

	t.Run("canonicalHeaderSet always contains baseline redacted headers", func(t *testing.T) {
		set := canonicalHeaderSet(alwaysRedactedHeaders)
		for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Set-Cookie"} {
			_, ok := set[name]
			assert.True(t, ok, "%s should be in redacted set", name)
		}
	})

	t.Run("canonicalHeaderSet drops empty entries", func(t *testing.T) {
		set := canonicalHeaderSet([]string{"  ", "", "X-Foo"})
		_, hasFoo := set["X-Foo"]
		assert.True(t, hasFoo)
		_, hasEmpty := set[""]
		assert.False(t, hasEmpty)
	})
}
