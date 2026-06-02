package muxhandlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthHandlerLiveness(t *testing.T) {
	h := HealthHandler(HealthConfig{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok\n", w.Body.String())
	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestHealthHandlerAllChecksPass(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "db", Check: func(context.Context) error { return nil }},
			{Name: "cache", Check: func(context.Context) error { return nil }},
		},
		IncludeNames: true,
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok:")
	assert.Contains(t, w.Body.String(), "db")
	assert.Contains(t, w.Body.String(), "cache")
}

func TestHealthHandlerAnyCheckFails(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "db", Check: func(context.Context) error { return errors.New("connection refused") }},
			{Name: "cache", Check: func(context.Context) error { return nil }},
		},
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "db: connection refused")
	assert.NotContains(t, body, "cache:")
}

func TestHealthHandlerJSON(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "db", Check: func(context.Context) error { return errors.New("boom") }},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `"status":"degraded"`)
	assert.Contains(t, w.Body.String(), `"db":{"status":"fail","error":"boom"}`)
}

func TestHealthHandlerTimeout(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{
				Name: "slow",
				Check: func(ctx context.Context) error {
					select {
					case <-time.After(100 * time.Millisecond):
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				},
			},
		},
		Timeout: 10 * time.Millisecond,
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "slow:")
}

func TestHealthHandlerTimeoutDoesNotWaitForIgnoredContext(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{
				Name: "runaway",
				Check: func(context.Context) error {
					close(started)
					select {
					case <-release:
					case <-time.After(200 * time.Millisecond):
					}
					return nil
				},
			},
		},
		Timeout: 10 * time.Millisecond,
	})

	start := time.Now()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	elapsed := time.Since(start)
	close(release)

	<-started
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "runaway: context deadline exceeded")
	assert.Less(t, elapsed, 100*time.Millisecond,
		"handler must return on timeout even when a check ignores ctx (took %s)", elapsed)
}

func TestHealthHandlerAlreadyCancelledContext(t *testing.T) {
	// When the request context is already cancelled (e.g. client gone)
	// and no per-check Timeout narrows it, the check is reported as failed
	// without ever invoking the Check function.
	called := false
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{
				Name: "db",
				Check: func(context.Context) error {
					called = true
					return nil
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "db: context canceled")
	assert.False(t, called, "check must be skipped when context is already cancelled")
}

func TestHealthHandlerMethodNotAllowed(t *testing.T) {
	h := HealthHandler(HealthConfig{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/healthz", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "GET, HEAD", w.Header().Get("Allow"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}

func TestHealthHandlerHEAD(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		h := HealthHandler(HealthConfig{})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/healthz", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, w.Body.String())
	})

	t.Run("json", func(t *testing.T) {
		h := HealthHandler(HealthConfig{})
		req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Empty(t, w.Body.String())
	})
}

func TestHealthHandlerRunsChecksInParallel(t *testing.T) {
	start := time.Now()
	delay := 50 * time.Millisecond
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "a", Check: func(context.Context) error { time.Sleep(delay); return nil }},
			{Name: "b", Check: func(context.Context) error { time.Sleep(delay); return nil }},
			{Name: "c", Check: func(context.Context) error { time.Sleep(delay); return nil }},
		},
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	elapsed := time.Since(start)

	require.Equal(t, http.StatusOK, w.Code)
	// Sequential would be ~150ms; parallel must be well under 100ms.
	assert.Less(t, elapsed, 100*time.Millisecond,
		"checks must run concurrently (took %s)", elapsed)
}

func TestHealthHandlerCustomOKBody(t *testing.T) {
	h := HealthHandler(HealthConfig{OKBody: "alive"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, "alive", w.Body.String())
}

func TestHealthHandlerSetsVaryAccept(t *testing.T) {
	h := HealthHandler(HealthConfig{})

	t.Run("text", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		assert.Equal(t, "Accept", w.Header().Get("Vary"))
	})

	t.Run("json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, "Accept", w.Header().Get("Vary"))
	})
}

// panickingError is an error whose Error() method panics, exercising the
// path where serialization (not the check itself) is the source of a
// panic. The reason string is extracted under the per-check recover, so
// this must not escape the handler.
type panickingError struct{}

func (panickingError) Error() string { panic("error string panic") }

func TestHealthHandlerRecoversFromPanickingErrorString(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "db", Check: func(context.Context) error { return panickingError{} }},
		},
	})
	w := httptest.NewRecorder()
	// No RecoveryMiddleware: the handler itself must contain the panic.
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "db: panic: error string panic")
}

func TestHealthHandlerRecoversFromPanickingCheck(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "boom", Check: func(context.Context) error { panic("kaboom") }},
			{Name: "ok", Check: func(context.Context) error { return nil }},
		},
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "boom: panic: kaboom")
	assert.NotContains(t, body, "ok:")
}

func TestHealthHandlerIncludeNamesWithNoChecks(t *testing.T) {
	// IncludeNames on a liveness handler (no checks) must fall back to
	// OKBody, not emit an empty "ok: " line.
	h := HealthHandler(HealthConfig{IncludeNames: true})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok\n", w.Body.String())
}

func TestHealthHandlerJSONSuccess(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "db", Check: func(context.Context) error { return nil }},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"ok"`)
	assert.Contains(t, w.Body.String(), `"db":{"status":"ok"}`)
}

func TestHealthHandlerJSONLivenessOmitsChecks(t *testing.T) {
	// Liveness JSON (no checks) must omit the checks object entirely.
	h := HealthHandler(HealthConfig{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()
	assert.Contains(t, body, `"status":"ok"`)
	assert.NotContains(t, body, `"checks"`)
}

func TestHealthHandlerNilCheckFunc(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "db"}, // Check is nil
		},
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "db: check not configured")
}

func TestHealthHandlerDuplicateNamesJSON(t *testing.T) {
	// Duplicate names must never collapse in the JSON map: each check
	// gets a distinct key. The expected suffixing depends on declaration
	// order and on any explicitly-supplied "#N" names.
	tests := []struct {
		name    string
		inputs  []string // check names, each backed by a check failing with that name
		wantKey map[string]string
	}{
		{
			name:   "two duplicates",
			inputs: []string{"db", "db"},
			wantKey: map[string]string{
				"db":   "db",
				"db#2": "db",
			},
		},
		{
			name:   "explicit suffix before duplicates",
			inputs: []string{"db#2", "db", "db"},
			wantKey: map[string]string{
				"db#2": "db#2",
				"db":   "db",
				"db#3": "db",
			},
		},
		{
			name:   "generated suffix collides with later explicit",
			inputs: []string{"db", "db", "db#2"},
			wantKey: map[string]string{
				"db":     "db",
				"db#2":   "db",
				"db#2#2": "db#2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks := make([]HealthCheck, len(tt.inputs))
			for i, in := range tt.inputs {
				checks[i] = HealthCheck{Name: in, Check: func(context.Context) error { return errors.New(in) }}
			}
			h := HealthHandler(HealthConfig{Checks: checks})
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			var report HealthReport
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &report))
			require.Len(t, report.Checks, len(tt.inputs), "every check must appear under a distinct key")
			for key, wantErr := range tt.wantKey {
				c, ok := report.Checks[key]
				require.True(t, ok, "missing key %q", key)
				assert.Equal(t, wantErr, c.Error)
			}
		})
	}
}

func TestHealthHandlerEmptyName(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Check: func(context.Context) error { return errors.New("boom") }},
		},
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	// No leading ": boom"; the unnamed check gets a stable name.
	assert.Contains(t, w.Body.String(), "check-0: boom")
}

func TestHealthHandlerMultilineError(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "db", Check: func(context.Context) error { return errors.New("connection refused\ncontext deadline exceeded") }},
			{Name: "cache", Check: func(context.Context) error { return errors.New("down") }},
		},
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	body := w.Body.String()
	// The multi-line error is collapsed onto a single line.
	assert.Contains(t, body, "db: connection refused context deadline exceeded\n")
	// And does not split the line count: each failed check is one line.
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	assert.Len(t, lines, 2)
}

func TestHealthHandlerMultilineNameTextOutput(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		h := HealthHandler(HealthConfig{
			Checks: []HealthCheck{
				{Name: "db\nprimary", Check: func(context.Context) error { return errors.New("down") }},
			},
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		body := w.Body.String()
		assert.Contains(t, body, "db primary: down\n")
		assert.Len(t, strings.Split(strings.TrimRight(body, "\n"), "\n"), 1)
	})

	t.Run("success include names", func(t *testing.T) {
		h := HealthHandler(HealthConfig{
			Checks: []HealthCheck{
				{Name: "db\nprimary", Check: func(context.Context) error { return nil }},
			},
			IncludeNames: true,
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		body := w.Body.String()
		assert.Contains(t, body, "ok: db primary\n")
		assert.Len(t, strings.Split(strings.TrimRight(body, "\n"), "\n"), 1)
	})
}

func TestHealthHandlerMultipleChecksFail(t *testing.T) {
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{Name: "db", Check: func(context.Context) error { return errors.New("down") }},
			{Name: "cache", Check: func(context.Context) error { return nil }},
			{Name: "broker", Check: func(context.Context) error { return errors.New("timeout") }},
		},
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	body := w.Body.String()
	// Both failing checks are reported; the passing one is not.
	assert.Contains(t, body, "db: down")
	assert.Contains(t, body, "broker: timeout")
	assert.NotContains(t, body, "cache:")
	// One line per failing check, no extra lines.
	assert.Len(t, strings.Split(strings.TrimRight(body, "\n"), "\n"), 2)
}

func TestHealthHandlerMixedTimeoutAndFastPass(t *testing.T) {
	// A slow check that honors ctx times out (503), while a fast sibling
	// passes. The overall result must be 503 and name the slow check.
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{
				Name: "slow",
				Check: func(ctx context.Context) error {
					select {
					case <-time.After(time.Second):
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				},
			},
			{Name: "fast", Check: func(context.Context) error { return nil }},
		},
		Timeout: 10 * time.Millisecond,
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "slow:")
	assert.NotContains(t, body, "fast:")
}

func TestHealthHandlerReturnsPromptlyOnSlowCheck(t *testing.T) {
	// A check sleeps far longer than its timeout but honors ctx. The
	// handler must return once the per-check timeout fires, bounded by
	// the timeout rather than the check's sleep duration.
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{
				Name: "runaway",
				Check: func(ctx context.Context) error {
					select {
					case <-time.After(time.Second):
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				},
			},
		},
		Timeout: 10 * time.Millisecond,
	})

	start := time.Now()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "runaway:")
	// Bounded by the per-check timeout, not the 1s the check would sleep.
	assert.Less(t, elapsed, 500*time.Millisecond,
		"handler must return once the per-check timeout fires (took %s)", elapsed)
}

func TestHealthHandlerNegotiatesPlainTextWhenRequested(t *testing.T) {
	h := HealthHandler(HealthConfig{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Accept", "text/plain")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "ok\n", w.Body.String())
}

func TestHealthHandlerPrefersPlainTextForWildcardAccept(t *testing.T) {
	h := HealthHandler(HealthConfig{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Accept", "*/*")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
}

func TestHealthHandlerDoesNotLeakAcceptCaseSensitivity(t *testing.T) {
	h := HealthHandler(HealthConfig{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Accept", "APPLICATION/JSON")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.True(t, strings.Contains(w.Body.String(), `"status":"ok"`),
		"Accept matching must be case-insensitive")
}

func TestHealthHandlerUnsupportedAccept(t *testing.T) {
	called := false
	h := HealthHandler(HealthConfig{
		Checks: []HealthCheck{
			{
				Name: "db",
				Check: func(context.Context) error {
					called = true
					return nil
				},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req.Header.Set("Accept", "application/xml")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotAcceptable, w.Code)
	assert.False(t, called, "unsupported Accept should be rejected before running checks")
	assert.Equal(t, "Accept", w.Header().Get("Vary"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestHealthHandlerHEADUnsupportedAcceptHasNoBody(t *testing.T) {
	h := HealthHandler(HealthConfig{})
	req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
	req.Header.Set("Accept", "application/xml")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotAcceptable, w.Code)
	assert.Empty(t, w.Body.String())
}

func TestHealthHandlerPreservesExistingVary(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		h := HealthHandler(HealthConfig{})
		w := httptest.NewRecorder()
		w.Header().Add("Vary", "Origin")
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

		assert.Contains(t, w.Header().Values("Vary"), "Origin")
		assert.Contains(t, w.Header().Values("Vary"), "Accept")
	})

	t.Run("json", func(t *testing.T) {
		h := HealthHandler(HealthConfig{})
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		w.Header().Add("Vary", "Origin")
		h.ServeHTTP(w, req)

		assert.Contains(t, w.Header().Values("Vary"), "Origin")
		assert.Contains(t, w.Header().Values("Vary"), "Accept")
	})
}
