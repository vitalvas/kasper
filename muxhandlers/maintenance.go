package muxhandlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// MaintenanceConfig configures the MaintenanceMode middleware.
type MaintenanceConfig struct {
	// Enabled reports whether the maintenance response should be sent
	// for the current request. The predicate is consulted on every
	// request, so callers can flip maintenance on and off by mutating
	// the data structure (atomic.Bool, file, env, scheduled window)
	// the predicate reads. When nil, the middleware is a no-op and
	// every request passes through.
	Enabled func(*http.Request) bool

	// Bypass, when non-nil, is consulted before Enabled is checked; if
	// it returns true the request bypasses maintenance entirely. The
	// router is supplied so the predicate can inspect matched-route
	// metadata, allow lists, header tokens, etc. Typical uses: admin
	// IP allow-list, internal health checks, deploy-pipeline tooling.
	Bypass func(*mux.Router, *http.Request) bool

	// Response, when non-nil, fully owns the maintenance response
	// body. The middleware sets Retry-After (when configured) and then
	// invokes the handler; StatusCode is ignored because the handler
	// controls its own status. Use this to render an HTML page, return
	// RFC 9457 ProblemDetails JSON, redirect to a static maintenance
	// page, or anything else.
	Response http.Handler

	// StatusCode is the HTTP status code for the default response when
	// Response is nil. Defaults to 503 Service Unavailable (RFC 9110
	// Section 15.6.4), which is the spec-correct signal for scheduled
	// maintenance; other codes are an escape hatch and should be used
	// with a clear reason.
	StatusCode int

	// RetryAfter, when greater than zero and RetryAt is the zero value,
	// emits a Retry-After header in delta-seconds form per RFC 9110
	// Section 10.2.3. Sub-second values are rounded down.
	RetryAfter time.Duration

	// RetryAt, when non-zero, emits a Retry-After header in HTTP-date
	// form per RFC 9110 Section 10.2.3, overriding RetryAfter. Use
	// when maintenance has a scheduled end time. Times are formatted
	// in UTC.
	RetryAt time.Time
}

// MaintenanceModeMiddleware short-circuits matching requests with a
// "service unavailable" response while a maintenance window is active.
// The Enabled predicate is the single source of truth; callers back it
// with whatever they like (atomic.Bool, file presence, env var, cron
// window) and the middleware reads it per request.
//
// When Enabled returns true and Bypass does not, the middleware sets
// Retry-After (if configured) and either invokes Response, when set,
// or writes a default plain-text body with StatusCode. When Enabled
// returns false, or Bypass returns true, the request flows through to
// the next handler unchanged.
//
// The router is accepted so the Bypass predicate can resolve route
// metadata. Pass the same *mux.Router the middleware is attached to
// via Use.
func MaintenanceModeMiddleware(router *mux.Router, cfg MaintenanceConfig) mux.MiddlewareFunc {
	enabled := cfg.Enabled
	bypass := cfg.Bypass
	response := cfg.Response

	statusCode := cfg.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusServiceUnavailable
	}

	retryAfterHeader := formatRetryAfter(cfg.RetryAfter, cfg.RetryAt)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if enabled == nil || !enabled(r) {
				next.ServeHTTP(w, r)
				return
			}
			if bypass != nil && bypass(router, r) {
				next.ServeHTTP(w, r)
				return
			}

			if retryAfterHeader != "" {
				w.Header().Set("Retry-After", retryAfterHeader)
			}

			if response != nil {
				response.ServeHTTP(w, r)
				return
			}

			writeDefaultMaintenanceResponse(w, statusCode)
		})
	}
}

// writeDefaultMaintenanceResponse writes a minimal plain-text 503-style
// response. Used only when MaintenanceConfig.Response is nil.
func writeDefaultMaintenanceResponse(w http.ResponseWriter, statusCode int) {
	body := http.StatusText(statusCode)
	if body == "" {
		body = http.StatusText(http.StatusServiceUnavailable)
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body))
}

// formatRetryAfter returns the Retry-After header value derived from
// the configured Duration and absolute time, honoring RFC 9110 Section
// 10.2.3 (either delta-seconds or HTTP-date). RetryAt wins when both
// are set. Returns empty string when neither is set.
func formatRetryAfter(after time.Duration, at time.Time) string {
	if !at.IsZero() {
		return at.UTC().Format(http.TimeFormat)
	}
	if after > 0 {
		seconds := max(int64(after/time.Second), 1)
		return strconv.FormatInt(seconds, 10)
	}
	return ""
}
