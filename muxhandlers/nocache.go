package muxhandlers

import (
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

			rewriter := &noCacheResponseWriter{ResponseWriter: w, apply: apply}
			next.ServeHTTP(rewriter, r)

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

// noCacheResponseWriter rewrites caching headers exactly once, at the
// moment the response is committed (WriteHeader or the first Write).
// It does not buffer the body — only header values are touched.
type noCacheResponseWriter struct {
	http.ResponseWriter
	apply   func(http.Header)
	applied bool
}

func (w *noCacheResponseWriter) WriteHeader(code int) {
	if !w.applied {
		w.apply(w.Header())
		w.applied = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *noCacheResponseWriter) Write(b []byte) (int, error) {
	if !w.applied {
		w.apply(w.Header())
		w.applied = true
	}
	return w.ResponseWriter.Write(b)
}
