package muxhandlers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
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

// fullResponseWriter is a test double that implements every optional
// interface a real net/http server can provide, so we can assert that
// the middleware wrappers forward them to the underlying writer.
type fullResponseWriter struct {
	*httptest.ResponseRecorder
	flushed     bool
	hijacked    bool
	pushed      string
	pushSupport bool
}

func (f *fullResponseWriter) Flush() {
	f.flushed = true
	f.ResponseRecorder.Flush()
}

func (f *fullResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	f.hijacked = true
	server, client := net.Pipe()
	_ = client.Close()
	brw := bufio.NewReadWriter(bufio.NewReader(server), bufio.NewWriter(server))
	return server, brw, nil
}

func (f *fullResponseWriter) Push(target string, _ *http.PushOptions) error {
	if !f.pushSupport {
		return http.ErrNotSupported
	}
	f.pushed = target
	return nil
}

func TestAccessLogPreservesOptionalInterfaces(t *testing.T) {
	t.Run("Flush is forwarded to the underlying writer", func(t *testing.T) {
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder()}
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			f, ok := w.(http.Flusher)
			if assert.True(t, ok, "wrapper must implement http.Flusher") {
				f.Flush()
			}
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.True(t, full.flushed)
	})

	t.Run("ResponseController.Flush works via Unwrap", func(t *testing.T) {
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder()}
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			assert.NoError(t, http.NewResponseController(w).Flush())
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.True(t, full.flushed)
	})

	t.Run("Hijack is forwarded so WebSocket upgrades work", func(t *testing.T) {
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder()}
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if assert.True(t, ok, "wrapper must implement http.Hijacker") {
				conn, _, err := hj.Hijack()
				assert.NoError(t, err)
				if conn != nil {
					_ = conn.Close()
				}
			}
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.True(t, full.hijacked)
	})

	t.Run("hijacked requests log hijacked=true with zero status", func(t *testing.T) {
		// Regression: previously the wrapper reported Status=200 for
		// every hijacked connection because the upgrader writes raw
		// bytes (e.g. "HTTP/1.1 101 Switching Protocols") directly to
		// the net.Conn and the wrapper never observes a WriteHeader.
		// The wrapper must instead record Hijacked=true and emit an
		// "hijacked" attr in slog output instead of a fabricated 200.
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder()}
		logger, buf := newSlogCapture()
		var captured *AccessLogEntry
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger:  logger,
			LogFunc: func(e *AccessLogEntry) { captured = e },
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			assert.NoError(t, err)
			if conn != nil {
				_ = conn.Close()
			}
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/ws", nil))

		// LogFunc captures the entry; slog is bypassed.
		if assert.NotNil(t, captured) {
			assert.True(t, captured.Hijacked, "Hijacked must be set")
			assert.Equal(t, 0, captured.Status, "Status must be 0 when hijacked")
		}
		// And slog output (from a non-LogFunc run) should carry the
		// hijacked attr instead of status.
		captured = nil
		mw = AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{Logger: logger})
		full2 := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder()}
		h = mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			conn, _, _ := w.(http.Hijacker).Hijack()
			if conn != nil {
				_ = conn.Close()
			}
		}))
		h.ServeHTTP(full2, httptest.NewRequest(http.MethodGet, "/ws", nil))

		entry := decodeLogLine(t, buf)
		_, hasStatus := entry["status"]
		assert.False(t, hasStatus, "status attr must be absent for hijacked entry")
		assert.Equal(t, true, entry["hijacked"])
	})

	t.Run("Push is forwarded when supported", func(t *testing.T) {
		full := &fullResponseWriter{ResponseRecorder: httptest.NewRecorder(), pushSupport: true}
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			p, ok := w.(http.Pusher)
			if assert.True(t, ok) {
				assert.NoError(t, p.Push("/style.css", nil))
			}
		}))
		h.ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, "/style.css", full.pushed)
	})

	t.Run("optional interfaces are advertised only when the underlying writer supports them", func(t *testing.T) {
		// A bare httptest.NewRecorder implements Flusher but not
		// Hijacker or Pusher. The wrapper must mirror that exactly:
		// type assertions for absent capabilities must return ok=false,
		// matching what the handler would see if no middleware were
		// applied. Over-advertising would force a handler that does
		// feature detection (e.g. WebSocket upgraders, SSE writers)
		// into the wrong branch.
		recorder := httptest.NewRecorder()
		_, recorderHasFlush := http.ResponseWriter(recorder).(http.Flusher)
		_, recorderHasHijack := http.ResponseWriter(recorder).(http.Hijacker)
		_, recorderHasPush := http.ResponseWriter(recorder).(http.Pusher)
		// Sanity-check the baseline so the assertions below have meaning.
		assert.True(t, recorderHasFlush, "httptest.NewRecorder should implement Flusher")
		assert.False(t, recorderHasHijack)
		assert.False(t, recorderHasPush)

		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		})
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
		mw := AccessLogMiddleware(mux.NewRouter(), AccessLogConfig{
			Logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		})
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

// capabilityWriter is a minimal http.ResponseWriter used to probe
// every combination of optional interfaces accessLogWrap and
// noCacheWrap must produce. Each capability is gated by a flag so a
// single helper can synthesize all eight variants by composition.
type capabilityWriter struct {
	header http.Header
}

func (c *capabilityWriter) Header() http.Header       { return c.header }
func (*capabilityWriter) Write(b []byte) (int, error) { return len(b), nil }
func (*capabilityWriter) WriteHeader(int)             {}

type capWriterF struct{ *capabilityWriter }

func (*capWriterF) Flush() {}

type capWriterH struct{ *capabilityWriter }

func (*capWriterH) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}

type capWriterP struct{ *capabilityWriter }

func (*capWriterP) Push(string, *http.PushOptions) error { return http.ErrNotSupported }

type capWriterFH struct {
	*capabilityWriter
	*capWriterF
	*capWriterH
}

type capWriterFP struct {
	*capabilityWriter
	*capWriterF
	*capWriterP
}

type capWriterHP struct {
	*capabilityWriter
	*capWriterH
	*capWriterP
}

type capWriterFHP struct {
	*capabilityWriter
	*capWriterF
	*capWriterH
	*capWriterP
}

// makeCapabilityWriter returns an http.ResponseWriter that advertises
// exactly the supplied capabilities. It is used to drive
// accessLogWrap / noCacheWrap through every wrapper variant.
//
// Pointers are returned for the single- and combo-capability variants
// because their Flush/Hijack/Push methods use pointer receivers, so
// only the pointer types satisfy the corresponding http.* interfaces.
func makeCapabilityWriter(flush, hijack, push bool) http.ResponseWriter {
	base := &capabilityWriter{header: http.Header{}}
	switch {
	case flush && hijack && push:
		return &capWriterFHP{capabilityWriter: base, capWriterF: &capWriterF{base}, capWriterH: &capWriterH{base}, capWriterP: &capWriterP{base}}
	case flush && hijack:
		return &capWriterFH{capabilityWriter: base, capWriterF: &capWriterF{base}, capWriterH: &capWriterH{base}}
	case flush && push:
		return &capWriterFP{capabilityWriter: base, capWriterF: &capWriterF{base}, capWriterP: &capWriterP{base}}
	case hijack && push:
		return &capWriterHP{capabilityWriter: base, capWriterH: &capWriterH{base}, capWriterP: &capWriterP{base}}
	case flush:
		return &capWriterF{base}
	case hijack:
		return &capWriterH{base}
	case push:
		return &capWriterP{base}
	default:
		return base
	}
}

func TestAccessLogWrapVariants(t *testing.T) {
	// Drive accessLogWrap through every non-empty subset of
	// {Flusher, Hijacker, Pusher} plus the no-capability base case,
	// asserting the wrapper exposes exactly the same capabilities.
	cases := []struct {
		name                  string
		flush, hijack, push   bool
		wantFlush, wantHijack bool
		wantPush              bool
	}{
		{name: "none", flush: false, hijack: false, push: false},
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
			_, wrapped := accessLogWrap(inner)
			_, hasFlush := wrapped.(http.Flusher)
			_, hasHijack := wrapped.(http.Hijacker)
			_, hasPush := wrapped.(http.Pusher)
			assert.Equal(t, tc.wantFlush, hasFlush, "Flusher")
			assert.Equal(t, tc.wantHijack, hasHijack, "Hijacker")
			assert.Equal(t, tc.wantPush, hasPush, "Pusher")

			// Every variant must expose Unwrap so
			// http.ResponseController can reach the underlying writer.
			type unwrapper interface{ Unwrap() http.ResponseWriter }
			_, hasUnwrap := wrapped.(unwrapper)
			assert.True(t, hasUnwrap, "Unwrap must be exposed by every variant")
		})
	}
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
