package muxhandlers

import (
	"errors"
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// ErrNoLinks is returned when EarlyHintsConfig has neither Links nor LinksFunc configured.
var ErrNoLinks = errors.New("early hints: at least one Link must be set")

// EarlyHintsConfig configures the Early Hints middleware.
//
// Spec reference: https://www.rfc-editor.org/rfc/rfc8297
type EarlyHintsConfig struct {
	// Links is the list of Link header values to send with the 103 Early
	// Hints response. Each entry should follow the format defined in
	// RFC 8288, e.g. `</style.css>; rel=preload; as=style`.
	Links []string

	// LinksFunc is called per request to compute Link header values
	// dynamically. Its results are sent alongside the static Links list.
	// Either Links or LinksFunc (or both) must be set.
	LinksFunc func(r *http.Request) []string
}

// EarlyHintsMiddleware returns a middleware that sends a 103 Early Hints
// informational response per RFC 8297 before the final response. This allows
// clients to begin preloading resources (stylesheets, scripts, fonts) while
// the server is still processing the request.
//
// The middleware sets the configured Link headers and writes a 103 status code.
// The downstream handler then writes the final response as usual. Link headers
// from the 103 response are not carried over to the final response.
//
// It returns ErrNoLinks if both Links is empty and LinksFunc is nil.
func EarlyHintsMiddleware(cfg EarlyHintsConfig) (mux.MiddlewareFunc, error) {
	if len(cfg.Links) == 0 && cfg.LinksFunc == nil {
		return nil, ErrNoLinks
	}

	links := make([]string, len(cfg.Links))
	copy(links, cfg.Links)

	linksFunc := cfg.LinksFunc

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var dynamic []string
			if linksFunc != nil {
				dynamic = linksFunc(r)
			}

			if len(links) == 0 && len(dynamic) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			for _, link := range links {
				h.Add("Link", link)
			}
			for _, link := range dynamic {
				h.Add("Link", link)
			}

			w.WriteHeader(http.StatusEarlyHints)

			next.ServeHTTP(w, r)
		})
	}, nil
}
