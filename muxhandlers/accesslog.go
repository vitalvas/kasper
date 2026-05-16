package muxhandlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// AccessLogEntry is the structured record produced for every request
// the AccessLog middleware observes. It is supplied to a user-provided
// callback (when LogFunc is set), and is also the source of fields the
// default slog sink emits.
type AccessLogEntry struct {
	// Time is when the request started processing.
	Time time.Time

	// Method is the HTTP request method (RFC 9110 Section 9).
	Method string

	// Proto is the request protocol as reported by r.Proto, e.g.
	// "HTTP/1.1", "HTTP/2.0".
	Proto string

	// Scheme is the resolved request scheme, "http" or "https". It
	// uses mux.Scheme(r), which infers https when r.URL.Scheme is set
	// (typically by ProxyHeadersMiddleware from a trusted
	// X-Forwarded-Proto) or when the connection is TLS.
	Scheme string

	// Host is r.Host (the Host header value, post-proxy resolution
	// when ProxyHeadersMiddleware is upstream). Useful for multi-vhost
	// deployments.
	Host string

	// Path is r.URL.Path as observed at handler entry; it reflects any
	// path normalization or rewriting performed by upstream middleware.
	Path string

	// Query is r.URL.RawQuery; empty when no query string was present.
	Query string

	// Status is the HTTP status code written by the handler. Defaults
	// to 200 when the handler completed without calling WriteHeader,
	// matching net/http behavior.
	Status int

	// Bytes is the total number of response body bytes written.
	Bytes int64

	// Duration is the wall-clock time spent in the handler chain.
	Duration time.Duration

	// RemoteAddr is r.RemoteAddr after any upstream proxy header
	// resolution. Use ProxyHeadersMiddleware to populate this from
	// trusted forwarded headers.
	RemoteAddr string

	// UserAgent is the request's User-Agent header value.
	UserAgent string

	// Referer is the request's Referer header value (RFC 9110 Section
	// 10.1.3, "Referer" preserves the original misspelling).
	Referer string

	// RouteName is the name set via mux.Route.Name, when the matched
	// route has one. Empty when the route is unnamed or no route was
	// matched.
	RouteName string

	// RequestID is the value returned by RequestIDFromContext, when the
	// request flowed through RequestIDMiddleware. Empty otherwise.
	RequestID string

	// Headers contains the request headers selected by IncludeHeaders,
	// with values for headers in RedactHeaders replaced by "[REDACTED]".
	// Nil when no headers are captured.
	Headers map[string]string

	// Err is set when ErrorFunc detects an application-level error
	// (e.g. 5xx status). Optional and informational.
	Err error
}

// AccessLogConfig configures the AccessLog middleware.
type AccessLogConfig struct {
	// Logger is the slog logger used when LogFunc is nil. It may be a
	// fully pre-configured logger: the middleware inherits whatever
	// handler, output sink, format, level, and pre-bound attributes
	// the caller has set (via slog.New, Logger.With, Logger.WithGroup,
	// etc.). Per-request access-log fields are appended to every
	// emitted record alongside those inherited attributes.
	// Defaults to slog.Default() when both Logger and LogFunc are nil.
	Logger *slog.Logger

	// LogFunc, when non-nil, fully takes over emission: the middleware
	// builds an AccessLogEntry and hands it to LogFunc instead of
	// touching Logger. Use this to integrate with non-slog sinks or to
	// suppress logging conditionally.
	LogFunc func(*AccessLogEntry)

	// Skip, when non-nil, is consulted before the handler runs; if it
	// returns true the request is processed without any log emission.
	// The first argument is the router this middleware was attached
	// to, so callers can resolve the matched route or its metadata
	// (e.g. mux.CurrentRoute(r).GetMetadataValueOr("skip_log", false))
	// to decide. Use it to silence health checks, metrics scrapes, or
	// routes tagged via metadata.
	Skip func(*mux.Router, *http.Request) bool

	// IncludeHeaders lists request header names to record into
	// AccessLogEntry.Headers. Names are matched case-insensitively.
	// When nil, no headers are captured.
	IncludeHeaders []string

	// RedactHeaders lists header names whose values are replaced with
	// "[REDACTED]" in AccessLogEntry.Headers. Authorization, Cookie,
	// Proxy-Authorization, and Set-Cookie are always redacted in
	// addition to anything supplied here. Names are case-insensitive.
	RedactHeaders []string

	// SlowThreshold, when greater than zero, raises the slog level of
	// requests whose duration exceeds it to Warn. Has no effect on
	// LogFunc, which always receives the full entry regardless of
	// duration.
	SlowThreshold time.Duration

	// Now overrides the clock source used for entry timestamps and
	// duration measurement. Defaults to time.Now. Intended for tests.
	Now func() time.Time
}

// alwaysRedactedHeaders is the baseline set of headers whose values are
// never emitted regardless of caller configuration. Each name is in
// canonical form so map lookups stay branch-free.
var alwaysRedactedHeaders = []string{
	"Authorization",
	"Cookie",
	"Proxy-Authorization",
	"Set-Cookie",
}

// AccessLogMiddleware records a structured entry for every request.
// The middleware wraps the response writer to capture the status code
// and response body byte count, runs the next handler, then emits an
// AccessLogEntry to LogFunc (when set) or to Logger (a slog logger).
// When neither is set, slog.Default() receives the entry.
//
// 5xx responses are logged at slog.LevelError; SlowThreshold escalates
// otherwise-Info requests to slog.LevelWarn when the handler runs
// longer than the threshold. Header capture is opt-in via
// IncludeHeaders; sensitive headers (Authorization, Cookie,
// Proxy-Authorization, Set-Cookie, plus anything in RedactHeaders) are
// always replaced by "[REDACTED]" when captured.
//
// The router is accepted so the Skip predicate can resolve route
// metadata or use route-aware filtering. Pass the same *mux.Router the
// middleware is attached to via Use.
func AccessLogMiddleware(router *mux.Router, cfg AccessLogConfig) mux.MiddlewareFunc {
	logger := cfg.Logger
	if logger == nil && cfg.LogFunc == nil {
		logger = slog.Default()
	}

	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	includeHeaders := canonicalHeaderList(cfg.IncludeHeaders)
	redactHeaders := canonicalHeaderSet(append(append([]string(nil), cfg.RedactHeaders...), alwaysRedactedHeaders...))

	skip := cfg.Skip
	slowThreshold := cfg.SlowThreshold
	logFunc := cfg.LogFunc

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip != nil && skip(router, r) {
				next.ServeHTTP(w, r)
				return
			}

			start := now()
			recorder := &accessLogResponseWriter{ResponseWriter: w}

			next.ServeHTTP(recorder, r)

			end := now()
			entry := buildAccessLogEntry(r, accessLogBuildArgs{
				recorder:       recorder,
				start:          start,
				end:            end,
				includeHeaders: includeHeaders,
				redactHeaders:  redactHeaders,
			})

			if logFunc != nil {
				logFunc(entry)
				return
			}
			emitSlog(logger, entry, slowThreshold)
		})
	}
}

// accessLogBuildArgs groups the inputs of buildAccessLogEntry so the
// signature stays within the project's parameter budget.
type accessLogBuildArgs struct {
	recorder       *accessLogResponseWriter
	start          time.Time
	end            time.Time
	includeHeaders []string
	redactHeaders  map[string]struct{}
}

// buildAccessLogEntry collects the per-request fields into a populated
// AccessLogEntry. Kept separate so tests can construct entries directly
// without standing up an http stack.
func buildAccessLogEntry(r *http.Request, args accessLogBuildArgs) *AccessLogEntry {
	entry := &AccessLogEntry{
		Time:       args.start,
		Method:     r.Method,
		Proto:      r.Proto,
		Scheme:     mux.Scheme(r),
		Host:       r.Host,
		Path:       r.URL.Path,
		Query:      r.URL.RawQuery,
		Status:     args.recorder.statusOrDefault(),
		Bytes:      args.recorder.bytes,
		Duration:   args.end.Sub(args.start),
		RemoteAddr: r.RemoteAddr,
		UserAgent:  r.UserAgent(),
		Referer:    r.Referer(),
		RequestID:  RequestIDFromContext(r.Context()),
	}

	if route := mux.CurrentRoute(r); route != nil {
		entry.RouteName = route.GetName()
	}

	if len(args.includeHeaders) > 0 {
		entry.Headers = captureHeaders(r.Header, args.includeHeaders, args.redactHeaders)
	}

	return entry
}

// captureHeaders extracts the requested header values from h, redacting
// any whose canonical name appears in redact.
func captureHeaders(h http.Header, include []string, redact map[string]struct{}) map[string]string {
	out := make(map[string]string, len(include))
	for _, name := range include {
		value := h.Get(name)
		if value == "" {
			continue
		}
		if _, ok := redact[name]; ok {
			value = "[REDACTED]"
		}
		out[name] = value
	}
	return out
}

// emitSlog writes the entry using a slog logger. Level selection:
// 5xx → Error, otherwise Info, escalated to Warn when SlowThreshold is
// set and the request exceeded it.
func emitSlog(logger *slog.Logger, entry *AccessLogEntry, slowThreshold time.Duration) {
	level := slog.LevelInfo
	switch {
	case entry.Status >= http.StatusInternalServerError:
		level = slog.LevelError
	case slowThreshold > 0 && entry.Duration >= slowThreshold:
		level = slog.LevelWarn
	}

	attrs := []slog.Attr{
		slog.String("method", entry.Method),
		slog.String("path", entry.Path),
		slog.Int("status", entry.Status),
		slog.Int64("bytes", entry.Bytes),
		slog.Duration("duration", entry.Duration),
		slog.String("remote_addr", entry.RemoteAddr),
	}
	if entry.Proto != "" {
		attrs = append(attrs, slog.String("proto", entry.Proto))
	}
	if entry.Scheme != "" {
		attrs = append(attrs, slog.String("scheme", entry.Scheme))
	}
	if entry.Host != "" {
		attrs = append(attrs, slog.String("host", entry.Host))
	}
	if entry.Query != "" {
		attrs = append(attrs, slog.String("query", entry.Query))
	}
	if entry.UserAgent != "" {
		attrs = append(attrs, slog.String("user_agent", entry.UserAgent))
	}
	if entry.Referer != "" {
		attrs = append(attrs, slog.String("referer", entry.Referer))
	}
	if entry.RouteName != "" {
		attrs = append(attrs, slog.String("route", entry.RouteName))
	}
	if entry.RequestID != "" {
		attrs = append(attrs, slog.String("request_id", entry.RequestID))
	}
	if len(entry.Headers) > 0 {
		headerAttrs := make([]any, 0, len(entry.Headers))
		for k, v := range entry.Headers {
			headerAttrs = append(headerAttrs, slog.String(k, v))
		}
		attrs = append(attrs, slog.Group("headers", headerAttrs...))
	}

	logger.LogAttrs(context.Background(), level, "http access", attrs...)
}

// accessLogResponseWriter wraps http.ResponseWriter to record the
// status code and response body byte count without otherwise altering
// the response. Implements only the surface required by access log
// observation; other interfaces (http.Flusher, http.Hijacker,
// http.Pusher) intentionally remain unimplemented because exposing
// them via embedding without explicit forwarding would falsely
// advertise capabilities the wrapper does not honor for status capture.
type accessLogResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (w *accessLogResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.status = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *accessLogResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		// net/http records 200 implicitly on the first Write; mirror
		// that so the recorded status matches the wire status.
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// statusOrDefault returns the recorded status code, falling back to
// 200 for handlers that completed without calling WriteHeader or Write
// (e.g. a handler that only sets headers and exits cleanly).
func (w *accessLogResponseWriter) statusOrDefault() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

// canonicalHeaderList returns the input list with each entry trimmed
// and canonicalized. Empty entries are dropped.
func canonicalHeaderList(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, http.CanonicalHeaderKey(name))
	}
	return out
}

// canonicalHeaderSet returns a lookup set of canonicalized header
// names, mirroring canonicalHeaderList but as a map.
func canonicalHeaderSet(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[http.CanonicalHeaderKey(name)] = struct{}{}
	}
	return out
}
