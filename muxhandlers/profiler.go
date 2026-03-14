package muxhandlers

import (
	"net/http"
	"net/http/pprof"
)

// ProfilerHandler returns an http.Handler that serves the standard
// net/http/pprof endpoints. Mount it with a PathPrefix on the router:
//
//	r.PathPrefix("/debug/pprof").Handler(muxhandlers.ProfilerHandler())
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
// See: https://pkg.go.dev/net/http/pprof
func ProfilerHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return mux
}
