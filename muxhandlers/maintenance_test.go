package muxhandlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vitalvas/kasper/mux"
)

func newMaintenanceHandler(cfg MaintenanceConfig) http.Handler {
	mw := MaintenanceModeMiddleware(mux.NewRouter(), cfg)
	return mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Pass", "through")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("downstream"))
	}))
}

func TestMaintenanceModeMiddleware(t *testing.T) {
	t.Run("nil Enabled is a no-op", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "through", w.Header().Get("X-Pass"))
	})

	t.Run("Enabled returning false passes through", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled: func(_ *http.Request) bool { return false },
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Enabled returning true emits default 503 plain-text body", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled: func(_ *http.Request) bool { return true },
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Equal(t, "Service Unavailable", w.Body.String())
	})

	t.Run("custom StatusCode is honored", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled:    func(_ *http.Request) bool { return true },
			StatusCode: http.StatusBadGateway,
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusBadGateway, w.Code)
		assert.Equal(t, "Bad Gateway", w.Body.String())
	})

	t.Run("unknown StatusCode falls back to Service Unavailable text", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled:    func(_ *http.Request) bool { return true },
			StatusCode: 599, // not in http.StatusText
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, 599, w.Code)
		assert.Equal(t, "Service Unavailable", w.Body.String())
	})

	t.Run("custom Response handler fully owns the response", func(t *testing.T) {
		htmlBody := `<!doctype html><title>Down</title><h1>We'll be right back.</h1>`
		response := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(htmlBody))
		})

		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled:    func(_ *http.Request) bool { return true },
			Response:   response,
			StatusCode: http.StatusTeapot, // must be ignored when Response is set
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, htmlBody, w.Body.String())
	})

	t.Run("RetryAfter Duration emits delta-seconds", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled:    func(_ *http.Request) bool { return true },
			RetryAfter: 90 * time.Second,
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, "90", w.Header().Get("Retry-After"))
	})

	t.Run("sub-second RetryAfter rounds up to 1", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled:    func(_ *http.Request) bool { return true },
			RetryAfter: 250 * time.Millisecond,
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, "1", w.Header().Get("Retry-After"))
	})

	t.Run("RetryAt emits HTTP-date in UTC", func(t *testing.T) {
		at := time.Date(2025, time.January, 2, 15, 4, 5, 0, time.FixedZone("CET", 3600))
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled: func(_ *http.Request) bool { return true },
			RetryAt: at,
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		// 15:04:05 CET == 14:04:05 UTC.
		assert.Equal(t, "Thu, 02 Jan 2025 14:04:05 GMT", w.Header().Get("Retry-After"))
	})

	t.Run("RetryAt takes precedence over RetryAfter", func(t *testing.T) {
		at := time.Date(2030, time.June, 15, 0, 0, 0, 0, time.UTC)
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled:    func(_ *http.Request) bool { return true },
			RetryAfter: 30 * time.Second,
			RetryAt:    at,
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, "Sat, 15 Jun 2030 00:00:00 GMT", w.Header().Get("Retry-After"))
	})

	t.Run("neither RetryAfter nor RetryAt omits the header", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled: func(_ *http.Request) bool { return true },
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Empty(t, w.Header().Get("Retry-After"))
	})

	t.Run("Bypass predicate forwards request to next handler", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled: func(_ *http.Request) bool { return true },
			Bypass: func(_ *mux.Router, r *http.Request) bool {
				return r.Header.Get("X-Admin") == "yes"
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Admin", "yes")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "through", w.Header().Get("X-Pass"))
	})

	t.Run("Bypass returning false still emits maintenance response", func(t *testing.T) {
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled: func(_ *http.Request) bool { return true },
			Bypass:  func(_ *mux.Router, _ *http.Request) bool { return false },
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("Enabled predicate can be flipped at runtime via atomic.Bool", func(t *testing.T) {
		var inMaintenance atomic.Bool
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled: func(_ *http.Request) bool { return inMaintenance.Load() },
		})

		// Off → through.
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusOK, w.Code)

		// On → 503.
		inMaintenance.Store(true)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		// Off again → through.
		inMaintenance.Store(false)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Bypass can read matched route metadata", func(t *testing.T) {
		const exemptKey = "exempt_from_maintenance"

		r := mux.NewRouter()
		r.Use(MaintenanceModeMiddleware(r, MaintenanceConfig{
			Enabled: func(_ *http.Request) bool { return true },
			Bypass: func(_ *mux.Router, req *http.Request) bool {
				route := mux.CurrentRoute(req)
				if route == nil {
					return false
				}
				exempt, _ := route.GetMetadataValueOr(exemptKey, false).(bool)
				return exempt
			},
		}))
		r.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		}).Metadata(exemptKey, true)
		r.HandleFunc("/api/users", func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("should not be called during maintenance")
		})

		health := httptest.NewRecorder()
		r.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		assert.Equal(t, http.StatusOK, health.Code)
		assert.Equal(t, "ok", health.Body.String())

		users := httptest.NewRecorder()
		r.ServeHTTP(users, httptest.NewRequest(http.MethodGet, "/api/users", nil))
		assert.Equal(t, http.StatusServiceUnavailable, users.Code)
	})

	t.Run("Retry-After is set before custom Response runs", func(t *testing.T) {
		var seen string
		response := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			seen = w.Header().Get("Retry-After")
			w.WriteHeader(http.StatusServiceUnavailable)
		})
		h := newMaintenanceHandler(MaintenanceConfig{
			Enabled:    func(_ *http.Request) bool { return true },
			Response:   response,
			RetryAfter: 60 * time.Second,
		})
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, "60", seen)
	})
}

func TestFormatRetryAfter(t *testing.T) {
	t.Run("zero values return empty", func(t *testing.T) {
		assert.Empty(t, formatRetryAfter(0, time.Time{}))
	})

	t.Run("negative duration returns empty", func(t *testing.T) {
		assert.Empty(t, formatRetryAfter(-time.Second, time.Time{}))
	})
}
