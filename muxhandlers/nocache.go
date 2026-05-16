package muxhandlers

import (
	"bufio"
	"net"
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// NoCachePreset selects the set of response headers NoCacheMiddleware
// writes on each response.
type NoCachePreset int

const (
	// NoCachePresetModern emits a single Cache-Control: no-store header
	// per RFC 9111 Section 5.2.2.5, which instructs shared and private
	// caches not to store any part of the response. Sufficient for any
	// client and intermediary that respects RFC 9111.
	NoCachePresetModern NoCachePreset = iota

	// NoCachePresetStrict emits the legacy "no-cache combo" expected by
	// caches and clients predating RFC 7234 / RFC 9111:
	//
	//   Cache-Control: no-store, no-cache, must-revalidate, max-age=0,
	//                  private
	//   Pragma:        no-cache
	//   Expires:       0
	//
	// Pragma comes from RFC 1945; Expires: 0 is the HTTP/1.0 convention
	// for "already expired". Use when downstream caches may be ancient
	// or non-conformant.
	NoCachePresetStrict
)

// NoCacheConfig configures the NoCache middleware.
type NoCacheConfig struct {
	// Preset selects which header set the middleware emits. Defaults to
	// NoCachePresetModern (Cache-Control: no-store only).
	Preset NoCachePreset

	// Skip, when non-nil, is consulted before each response is flushed;
	// returning true forwards the handler's response unchanged, leaving
	// any handler-set caching headers intact. The router is supplied so
	// callers can inspect matched-route metadata (e.g. opt specific
	// routes out of the no-cache policy).
	Skip func(*mux.Router, *http.Request) bool
}

// Header values emitted by the no-cache presets, kept as constants so
// tests can assert exact values and callers can read what the
// middleware actually writes.
const (
	noCacheModernValue    = "no-store"
	noCacheStrictValue    = "no-store, no-cache, must-revalidate, max-age=0, private"
	noCachePragmaValue    = "no-cache"
	noCacheExpiresEpoch   = "0"
	noCacheHeaderControl  = "Cache-Control"
	noCacheHeaderPragma   = "Pragma"
	noCacheHeaderExpires  = "Expires"
	noCacheHeaderETag     = "Etag"
	noCacheHeaderModified = "Last-Modified"
)

// NoCacheMiddleware forces responses to be uncacheable. It rewrites
// caching headers on the response writer at the moment the handler
// flushes its status line, overriding any Cache-Control, Pragma, or
// Expires the handler may have set, and removes ETag and Last-Modified
// so downstream caches cannot perform conditional revalidation.
//
// The Modern preset emits Cache-Control: no-store per RFC 9111
// Section 5.2.2.5; Strict adds the legacy Pragma and Expires header
// combo expected by HTTP/1.0-era intermediaries.
//
// The router argument is accepted so the Skip predicate can resolve
// route metadata. Pass the same *mux.Router the middleware is attached
// to via Use.
func NoCacheMiddleware(router *mux.Router, cfg NoCacheConfig) mux.MiddlewareFunc {
	skip := cfg.Skip
	apply := noCacheApplier(cfg.Preset)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip != nil && skip(router, r) {
				next.ServeHTTP(w, r)
				return
			}

			rewriter, wrapped := noCacheWrap(w, apply)
			next.ServeHTTP(wrapped, r)

			// If the handler returned without calling WriteHeader or
			// Write, stdlib will treat it as 200 OK at flush time; make
			// sure the headers are applied to that implicit response.
			if !rewriter.applied {
				rewriter.apply(rewriter.Header())
			}
		})
	}
}

// noCacheApplier returns a function that rewrites the header map per
// the selected preset. Strict overlays Pragma + Expires; both presets
// strip ETag and Last-Modified.
func noCacheApplier(preset NoCachePreset) func(http.Header) {
	switch preset {
	case NoCachePresetStrict:
		return func(h http.Header) {
			h.Set(noCacheHeaderControl, noCacheStrictValue)
			h.Set(noCacheHeaderPragma, noCachePragmaValue)
			h.Set(noCacheHeaderExpires, noCacheExpiresEpoch)
			h.Del(noCacheHeaderETag)
			h.Del(noCacheHeaderModified)
		}
	default:
		return func(h http.Header) {
			h.Set(noCacheHeaderControl, noCacheModernValue)
			// Pragma and Expires may have been set by the handler; the
			// Modern preset leaves them alone unless they would conflict
			// with no-store. Strip ETag and Last-Modified so caches do
			// not attempt conditional revalidation.
			h.Del(noCacheHeaderETag)
			h.Del(noCacheHeaderModified)
		}
	}
}

// noCacheResponseWriter is the base wrapper that rewrites caching
// headers exactly once, at the moment the response is committed
// (WriteHeader or the first Write). It does not buffer the body —
// only header values are touched. The base always exposes Unwrap so
// http.ResponseController can reach optional methods on the inner
// writer; Flusher, Hijacker, and Pusher are exposed through
// noCacheWrap, which picks a wrapper variant matching only the
// capabilities the inner writer actually advertises so handler-side
// type assertions like w.(http.Hijacker) keep observing the right
// ok value.
type noCacheResponseWriter struct {
	http.ResponseWriter
	apply   func(http.Header)
	applied bool
}

func (w *noCacheResponseWriter) WriteHeader(code int) {
	w.ensureApplied()
	w.ResponseWriter.WriteHeader(code)
}

func (w *noCacheResponseWriter) Write(b []byte) (int, error) {
	w.ensureApplied()
	return w.ResponseWriter.Write(b)
}

func (w *noCacheResponseWriter) ensureApplied() {
	if !w.applied {
		w.apply(w.Header())
		w.applied = true
	}
}

// Unwrap returns the underlying http.ResponseWriter so
// http.ResponseController can reach optional methods the embedded
// writer implements.
func (w *noCacheResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// The seven no-cache wrapper variants cover every non-empty subset
// of {Flusher, Hijacker, Pusher}. noCacheWrap picks the variant whose
// method set matches the capabilities of the wrapped writer, so
// handler-side type assertions like w.(http.Pusher) observe the same
// ok value they would on the bare net/http writer.

type noCacheFW struct{ *noCacheResponseWriter }

func (w noCacheFW) Flush() {
	w.ensureApplied()
	w.ResponseWriter.(http.Flusher).Flush()
}

type noCacheHW struct{ *noCacheResponseWriter }

func (w noCacheHW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.ensureApplied()
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

type noCachePW struct{ *noCacheResponseWriter }

func (w noCachePW) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}

type noCacheFHW struct {
	*noCacheResponseWriter
	noCacheFW
	noCacheHW
}

type noCacheFPW struct {
	*noCacheResponseWriter
	noCacheFW
	noCachePW
}

type noCacheHPW struct {
	*noCacheResponseWriter
	noCacheHW
	noCachePW
}

type noCacheFHPW struct {
	*noCacheResponseWriter
	noCacheFW
	noCacheHW
	noCachePW
}

// noCacheWrap returns an http.ResponseWriter that wraps inner with
// the no-cache header rewrite and exposes exactly the optional
// interfaces the inner writer supports. The base rewriter is returned
// alongside so callers can inspect rewriter.applied after the handler
// has run.
func noCacheWrap(inner http.ResponseWriter, apply func(http.Header)) (*noCacheResponseWriter, http.ResponseWriter) {
	base := &noCacheResponseWriter{ResponseWriter: inner, apply: apply}
	_, flush := inner.(http.Flusher)
	_, hijack := inner.(http.Hijacker)
	_, push := inner.(http.Pusher)
	switch {
	case flush && hijack && push:
		return base, noCacheFHPW{noCacheResponseWriter: base, noCacheFW: noCacheFW{base}, noCacheHW: noCacheHW{base}, noCachePW: noCachePW{base}}
	case flush && hijack:
		return base, noCacheFHW{noCacheResponseWriter: base, noCacheFW: noCacheFW{base}, noCacheHW: noCacheHW{base}}
	case flush && push:
		return base, noCacheFPW{noCacheResponseWriter: base, noCacheFW: noCacheFW{base}, noCachePW: noCachePW{base}}
	case hijack && push:
		return base, noCacheHPW{noCacheResponseWriter: base, noCacheHW: noCacheHW{base}, noCachePW: noCachePW{base}}
	case flush:
		return base, noCacheFW{base}
	case hijack:
		return base, noCacheHW{base}
	case push:
		return base, noCachePW{base}
	default:
		return base, base
	}
}
