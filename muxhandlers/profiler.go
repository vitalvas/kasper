package muxhandlers

import (
	"expvar"
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// RegisterProfiler registers the standard net/http/pprof and expvar
// endpoints on the given router. Mount using Route or PathPrefix:
//
//	r.Route("/debug", muxhandlers.RegisterProfiler)
//	r.Route("/_internal", muxhandlers.RegisterProfiler)
//
// Registered endpoints (relative to the mount path):
//
//	/debug/pprof/        - pprof index page
//	/debug/pprof/cmdline - running program command line
//	/debug/pprof/profile - CPU profile (supports ?seconds=N)
//	/debug/pprof/symbol  - symbol lookup
//	/debug/pprof/trace   - execution trace (supports ?seconds=N)
//	/debug/vars          - exported variables via the expvar package
//
// Named profiles (allocs, block, goroutine, heap, mutex, threadcreate)
// are served by the index handler.
//
// See: https://pkg.go.dev/net/http/pprof
// See: https://pkg.go.dev/expvar
func RegisterProfiler(r *mux.Router) {
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline).Methods(http.MethodGet)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile).Methods(http.MethodGet)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol).Methods(http.MethodGet, http.MethodPost)
	r.HandleFunc("/debug/pprof/trace", pprof.Trace).Methods(http.MethodGet)
	r.Handle("/debug/vars", expvar.Handler()).Methods(http.MethodGet)
	r.PathPrefix("/debug/pprof/").HandlerFunc(pprofIndex).Methods(http.MethodGet)
}

// pprofIndex wraps pprof.Index by rewriting r.URL.Path to the
// "/debug/pprof/" prefix that the stdlib handler expects internally.
func pprofIndex(w http.ResponseWriter, r *http.Request) {
	const marker = "/debug/pprof/"

	path := r.URL.Path
	if idx := strings.Index(path, marker); idx >= 0 {
		path = path[idx:]
	}

	r2 := new(http.Request)
	*r2 = *r
	u := *r.URL
	u.Path = path
	r2.URL = &u

	pprof.Index(w, r2)
}
