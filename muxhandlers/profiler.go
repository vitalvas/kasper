package muxhandlers

import (
	"expvar"
	"net/http"
	"net/http/pprof"
)

// ProfilerHandler returns an http.Handler that serves the standard
// net/http/pprof and expvar endpoints. Mount it with a PathPrefix on
// the router:
//
//	r.PathPrefix("/debug").Handler(muxhandlers.ProfilerHandler())
//
// Registered endpoints (relative to the mount path):
//
//	/              - pprof index page
//	/cmdline       - running program command line
//	/profile       - CPU profile (supports ?seconds=N)
//	/symbol        - symbol lookup
//	/trace         - execution trace (supports ?seconds=N)
//
// Named profiles (allocs, block, goroutine, heap, mutex, threadcreate)
// are served by the index handler.
//
// Additionally, /debug/vars serves exported variables via the expvar package.
//
// See: https://pkg.go.dev/net/http/pprof
// See: https://pkg.go.dev/expvar
func ProfilerHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/vars", expvar.Handler())

	return mux
}
