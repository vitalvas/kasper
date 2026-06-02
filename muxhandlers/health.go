package muxhandlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// healthOffered lists the media types HealthHandler can produce, in
// preference order: plain text is the default, JSON is opt-in via the
// Accept header. Reused for proactive content negotiation per RFC 9110
// Section 12.5.1.
var healthOffered = []string{mux.ContentTypeTextPlain, mux.ContentTypeApplicationJSON}

// HealthCheck is a single named probe executed by HealthHandler. The
// check returns nil on success and a non-nil error describing the
// failure on degradation. Implementations should respect ctx (use it
// for the dependency call's timeout so a hung dependency does not
// stall the handler). Checks that ignore ctx after the handler times
// out can continue running until the underlying operation returns.
type HealthCheck struct {
	// Name identifies the check in the response. Use a short
	// lowercase token (e.g. "db", "redis", "upstream-idp"). An empty
	// name is replaced with "check-<index>"; duplicate names are
	// suffixed "#2", "#3", ... in declaration order so every check is
	// reported distinctly.
	Name string

	// Check is invoked per request with the request context, narrowed
	// by HealthConfig.Timeout when set. Return nil for healthy, error
	// for degraded. A nil Check is reported as a failed check ("check
	// not configured") rather than panicking. Implementations should
	// honor ctx so a check that overruns its timeout does not leak its
	// goroutine while the dependency call drags on.
	Check func(ctx context.Context) error
}

// HealthConfig configures HealthHandler.
type HealthConfig struct {
	// Checks are the per-request probes. An empty slice yields a
	// liveness-only handler that always returns 200 - appropriate for
	// /healthz where the only assertion is "process is up". Populate
	// for readiness endpoints.
	Checks []HealthCheck

	// Timeout caps the time the handler waits for a single Check to
	// return. Zero means no per-check cap; the request context governs.
	Timeout time.Duration

	// OKBody is the response body on success. Defaults to "ok\n".
	// Ignored when the client requested application/json via Accept.
	OKBody string

	// IncludeNames, when true, lists the passed check names in the
	// plain-text 200 body. Has no effect on the JSON variant, which
	// always lists checks. Useful when one endpoint covers several
	// dependencies so operators can confirm the right set is wired.
	IncludeNames bool
}

// HealthReport is the JSON body emitted when the client sends
// Accept: application/json. It is exported so consumers can register
// it as a response schema with the openapi package.
type HealthReport struct {
	Status string                       `json:"status"`
	Checks map[string]HealthCheckReport `json:"checks,omitempty"`
}

// HealthCheckReport is the per-check entry in a HealthReport.
type HealthCheckReport struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// HealthHandler returns an http.Handler that runs the configured
// Checks and answers 200 when all pass or 503 with the failing
// check(s) named in the body when any fails.
//
// Liveness use:
//
//	r.Handle("/healthz", muxhandlers.HealthHandler(muxhandlers.HealthConfig{}))
//
// Readiness use:
//
//	r.Handle("/readyz", muxhandlers.HealthHandler(muxhandlers.HealthConfig{
//	    Checks: []muxhandlers.HealthCheck{
//	        {Name: "db", Check: func(ctx context.Context) error {
//	            return store.Ping(ctx)
//	        }},
//	    },
//	    Timeout: 2 * time.Second,
//	}))
//
// The handler answers GET and HEAD; other methods return 405 with the
// Allow header set per RFC 9110 Section 15.5.6.
func HealthHandler(cfg HealthConfig) http.Handler {
	okBody := cfg.OKBody
	if okBody == "" {
		okBody = "ok\n"
	}

	// Normalize check names once, at construction: assign a stable name
	// to unnamed checks and disambiguate collisions so no check is
	// dropped from the JSON map or rendered as a nameless text line.
	checks := normalizeHealthChecks(cfg.Checks)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			setHealthCommonHeaders(w.Header())
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		selected := negotiate(r.Header.Get("Accept"), healthOffered)
		if selected == "" {
			writeHealthNotAcceptable(w, r.Method == http.MethodHead)
			return
		}

		results := runHealthChecks(r.Context(), checks, cfg.Timeout)
		anyFailed := false
		for _, res := range results {
			if res.failed {
				anyFailed = true
				break
			}
		}

		status := http.StatusOK
		if anyFailed {
			status = http.StatusServiceUnavailable
		}

		head := r.Method == http.MethodHead
		if selected == mux.ContentTypeApplicationJSON {
			writeHealthJSON(w, status, results, head)
			return
		}

		writeHealthText(w, status, results, okBody, cfg.IncludeNames, head)
	})
}

// healthResult holds the outcome of a single check. The failure reason
// is captured as a string (not an error) at the point of the check, so
// that a misbehaving error value whose Error() method panics is contained
// by the per-check recover rather than escaping into the response writer.
type healthResult struct {
	name   string
	failed bool
	reason string
}

type indexedHealthResult struct {
	index  int
	result healthResult
}

// errCheckNotConfigured is the failure reason reported for a check whose
// Check function is nil. Surfacing it as a failed check (rather than a
// recovered panic) gives operators a clear, actionable message.
var errCheckNotConfigured = errors.New("check not configured")

// normalizeHealthChecks returns a copy of checks with stable, unique
// names. Empty names become "check-<index>"; duplicate names are
// suffixed "#2", "#3", ... in declaration order. The Check functions are
// carried through unchanged (including nil, handled at run time). The
// result is computed once per handler, not per request.
func normalizeHealthChecks(checks []HealthCheck) []HealthCheck {
	if len(checks) == 0 {
		return nil
	}

	out := make([]HealthCheck, len(checks))
	used := make(map[string]bool, len(checks))
	// nextSuffix[base] is the next "#N" suffix to try for a base name,
	// remembered across collisions so the search stays O(checks) overall
	// rather than rescanning from #2 each time.
	nextSuffix := make(map[string]int, len(checks))
	for i, c := range checks {
		base := c.Name
		if base == "" {
			base = fmt.Sprintf("check-%d", i)
		}

		name := base
		// Resolve collisions (with earlier checks or with an explicitly
		// supplied "base#N") by walking suffixes until an unused name is
		// found. The candidate itself may be taken by an explicit name,
		// so the loop keeps incrementing.
		for used[name] {
			suffix := max(nextSuffix[base], 2)
			name = fmt.Sprintf("%s#%d", base, suffix)
			nextSuffix[base] = suffix + 1
		}

		used[name] = true
		out[i] = HealthCheck{Name: name, Check: c.Check}
	}
	return out
}

// runHealthChecks invokes every check in parallel. Each check runs in
// its own goroutine; the slowest dominates the wall-clock cost. The
// per-check timeout (when set) caps how long the handler waits for that
// check, even if the check ignores ctx.
func runHealthChecks(ctx context.Context, checks []HealthCheck, timeout time.Duration) []healthResult {
	results := make([]healthResult, len(checks))
	if len(checks) == 0 {
		return results
	}

	done := make(chan indexedHealthResult, len(checks))
	for i, c := range checks {
		go func(i int, c HealthCheck) {
			done <- indexedHealthResult{
				index:  i,
				result: runHealthCheck(ctx, c, timeout),
			}
		}(i, c)
	}

	for range checks {
		completed := <-done
		results[completed.index] = completed.result
	}
	return results
}

func runHealthCheck(ctx context.Context, c HealthCheck, timeout time.Duration) healthResult {
	if c.Check == nil {
		return healthResult{name: c.Name, failed: true, reason: errCheckNotConfigured.Error()}
	}

	cctx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if err := cctx.Err(); err != nil {
		return healthResult{name: c.Name, failed: true, reason: err.Error()}
	}

	// outcome carries the resolved failure state. The reason string is
	// extracted inside this goroutine, under the recover, so a panic from
	// either Check or a misbehaving error's Error() method is contained
	// here and never reaches the response writer.
	type outcome struct {
		failed bool
		reason string
	}
	resCh := make(chan outcome, 1)
	go func() {
		var res outcome
		defer func() {
			if rec := recover(); rec != nil {
				res = outcome{failed: true, reason: fmt.Sprintf("panic: %v", rec)}
			}
			resCh <- res
		}()
		if err := c.Check(cctx); err != nil {
			res = outcome{failed: true, reason: err.Error()}
		}
	}()

	select {
	case res := <-resCh:
		return healthResult{name: c.Name, failed: res.failed, reason: res.reason}
	case <-cctx.Done():
		return healthResult{name: c.Name, failed: true, reason: cctx.Err().Error()}
	}
}

func setHealthCommonHeaders(h http.Header) {
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
}

func writeHealthText(w http.ResponseWriter, status int, results []healthResult, okBody string, includeNames bool, head bool) {
	h := w.Header()
	setHealthCommonHeaders(h)
	// The response body depends on the Accept header (text vs JSON), so
	// downstream caches must key on it per RFC 9110 Section 12.5.5.
	h.Add("Vary", "Accept")
	h.Set("Content-Type", mux.ContentTypeTextPlainUTF8)
	w.WriteHeader(status)
	if head {
		return
	}

	if status == http.StatusOK {
		if includeNames && len(results) > 0 {
			names := make([]string, 0, len(results))
			for _, res := range results {
				names = append(names, collapseLines(res.name))
			}
			fmt.Fprintf(w, "ok: %s\n", strings.Join(names, ","))
			return
		}
		_, _ = w.Write([]byte(okBody))
		return
	}

	for _, res := range results {
		if res.failed {
			fmt.Fprintf(w, "%s: %s\n", collapseLines(res.name), collapseLines(res.reason))
		}
	}
}

func writeHealthNotAcceptable(w http.ResponseWriter, head bool) {
	h := w.Header()
	setHealthCommonHeaders(h)
	h.Add("Vary", "Accept")
	h.Set("Content-Type", mux.ContentTypeTextPlainUTF8)
	w.WriteHeader(http.StatusNotAcceptable)
	if head {
		return
	}
	fmt.Fprintln(w, http.StatusText(http.StatusNotAcceptable))
}

// collapseLines folds CR and LF into single spaces so a multi-line error
// stays on one "name: reason" line and cannot be misread as a separate
// check by a line-oriented scraper. The JSON variant needs no such
// treatment because the encoder escapes control characters.
func collapseLines(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r == '\r' || r == '\n'
	}), " ")
}

func writeHealthJSON(w http.ResponseWriter, status int, results []healthResult, head bool) {
	report := HealthReport{Status: "ok"}
	if status != http.StatusOK {
		report.Status = "degraded"
	}
	if len(results) > 0 {
		report.Checks = make(map[string]HealthCheckReport, len(results))
		for _, res := range results {
			c := HealthCheckReport{Status: "ok"}
			if res.failed {
				c.Status = "fail"
				c.Error = res.reason
			}
			report.Checks[res.name] = c
		}
	}

	h := w.Header()
	setHealthCommonHeaders(h)
	h.Add("Vary", "Accept")
	if head {
		h.Set("Content-Type", mux.ContentTypeApplicationJSON)
		w.WriteHeader(status)
		return
	}
	mux.ResponseJSON(w, status, report)
}
