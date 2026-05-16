package muxhandlers

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// GracefulShutdownConfig configures the GracefulShutdown middleware.
type GracefulShutdownConfig struct {
	// Bypass, when non-nil, is consulted for every request; returning
	// true forwards the request to the next handler even while the
	// drain is in progress. The router is supplied so the predicate
	// can inspect matched-route metadata. Typical uses: keep k8s
	// liveness/readiness probes and /metrics reachable so the
	// orchestrator can observe the drain.
	Bypass func(*mux.Router, *http.Request) bool

	// Response, when non-nil, fully owns the response written to
	// requests that arrive after Drain() has been called. The
	// middleware sets the default headers (Connection: close,
	// Cache-Control: no-store, optional Retry-After) before invoking
	// the handler so the handler can override them or write its own.
	// StatusCode is ignored when Response is set because the handler
	// controls its own status.
	Response http.Handler

	// StatusCode is the HTTP status code for the default drain
	// response when Response is nil. Defaults to 503 Service
	// Unavailable (RFC 9110 Section 15.6.4), which is the spec-correct
	// signal that the server is intentionally rejecting new work.
	StatusCode int

	// RetryAfter, when greater than zero, emits a Retry-After header
	// in delta-seconds form per RFC 9110 Section 10.2.3 on drain
	// responses. Sub-second values round up to 1. Defaults to no
	// header.
	RetryAfter time.Duration
}

// Drainer is the control surface returned by GracefulShutdownMiddleware.
// Callers invoke Drain() from a signal handler to start rejecting new
// requests, then use Wait() to block until in-flight requests have
// completed (typically just before http.Server.Shutdown).
type Drainer struct {
	draining atomic.Bool
	inFlight atomic.Int64
}

// Drain marks the server as draining. After this call returns, every
// request that enters the middleware is rejected with the configured
// drain response unless Bypass forwards it. Idempotent.
func (d *Drainer) Drain() {
	d.draining.Store(true)
}

// IsDraining reports whether Drain has been called.
func (d *Drainer) IsDraining() bool {
	return d.draining.Load()
}

// InFlight returns the number of requests currently inside the
// middleware chain. Useful for metrics and tests; counts only requests
// the middleware decided to forward to next (i.e. not drained, not
// bypassed-without-incrementing).
func (d *Drainer) InFlight() int64 {
	return d.inFlight.Load()
}

// Wait blocks until the in-flight count reaches zero or the context is
// cancelled. Returns nil on a clean drain or ctx.Err() if the deadline
// fires first. Wait is safe to call before, during, or after Drain;
// when no requests have ever been observed it returns immediately.
// The implementation polls inFlight at a 20ms cadence, which is
// invisible against typical shutdown deadlines and avoids per-request
// signalling overhead on the hot path.
//
// The typical usage is:
//
//	drainer.Drain()
//	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	_ = drainer.Wait(shutdownCtx)
//	_ = srv.Shutdown(shutdownCtx)
func (d *Drainer) Wait(ctx context.Context) error {
	if d.inFlight.Load() == 0 {
		return nil
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if d.inFlight.Load() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// GracefulShutdownMiddleware returns a middleware that intercepts
// requests once Drain has been called and a Drainer the caller uses to
// trigger and observe the drain. Requests arriving before Drain() flow
// through unchanged; requests arriving after receive the configured
// drain response unless Bypass forwards them. In-flight requests are
// tracked via Drainer.InFlight and waited on via Drainer.Wait.
//
// Pair this with http.Server.Shutdown: call Drainer.Drain on SIGTERM,
// Drainer.Wait to let active requests complete, then Server.Shutdown
// to close listeners.
//
// The router is accepted so the Bypass predicate can resolve
// matched-route metadata. Pass the same *mux.Router the middleware is
// attached to via Use.
func GracefulShutdownMiddleware(router *mux.Router, cfg GracefulShutdownConfig) (mux.MiddlewareFunc, *Drainer) {
	drainer := &Drainer{}

	bypass := cfg.Bypass
	response := cfg.Response

	statusCode := cfg.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusServiceUnavailable
	}

	retryAfter := ""
	if cfg.RetryAfter > 0 {
		seconds := max(int64(cfg.RetryAfter/time.Second), 1)
		retryAfter = strconv.FormatInt(seconds, 10)
	}

	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if drainer.IsDraining() && (bypass == nil || !bypass(router, r)) {
				writeDrainResponse(w, r, response, statusCode, retryAfter)
				return
			}

			drainer.inFlight.Add(1)
			defer drainer.inFlight.Add(-1)
			next.ServeHTTP(w, r)
		})
	}
	return mw, drainer
}

// writeDrainResponse emits the response sent to requests that arrive
// after Drain. When response is nil a minimal plain-text 503-style
// body is written; otherwise the handler owns the body and is invoked
// after the default drain headers are set.
func writeDrainResponse(w http.ResponseWriter, r *http.Request, response http.Handler, statusCode int, retryAfter string) {
	h := w.Header()
	h.Set("Connection", "close")
	h.Set("Cache-Control", "no-store")
	if retryAfter != "" {
		h.Set("Retry-After", retryAfter)
	}

	if response != nil {
		response.ServeHTTP(w, r)
		return
	}

	body := http.StatusText(statusCode)
	if body == "" {
		body = http.StatusText(http.StatusServiceUnavailable)
	}
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body))
}
