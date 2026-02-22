package muxhandlers

import (
	"net/http"
	"os"

	"github.com/vitalvas/kasper/mux"
)

// ServerConfig configures the Server middleware behaviour.
type ServerConfig struct {
	// Hostname is the value written to the X-Server-Hostname response
	// header. Resolution order: Hostname field, then HostnameEnv
	// environment variable, then os.Hostname.
	Hostname string

	// HostnameEnv is a list of environment variable names checked in
	// order (e.g. ["POD_NAME", "HOSTNAME"]). The first non-empty
	// value is used. Only consulted when Hostname is empty. When all
	// variables are unset or empty, os.Hostname is used as a fallback.
	HostnameEnv []string
}

// ServerMiddleware returns a middleware that sets server identification
// response headers. The hostname is resolved once when the middleware is
// created. It returns an error if the hostname cannot be determined.
func ServerMiddleware(cfg ServerConfig) (mux.MiddlewareFunc, error) {
	hostname := cfg.Hostname

	if hostname == "" {
		for _, env := range cfg.HostnameEnv {
			if v, ok := os.LookupEnv(env); ok && v != "" {
				hostname = v
				break
			}
		}
	}

	if hostname == "" {
		h, err := os.Hostname()
		if err != nil {
			return nil, err
		}

		hostname = h
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Server-Hostname", hostname)
			next.ServeHTTP(w, r)
		})
	}, nil
}
