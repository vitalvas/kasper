package muxhandlers

import (
	"net/http"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// defaultAcceptPatchTypes is the set of patch content types advertised when
// AcceptPatchTypes is nil.
var defaultAcceptPatchTypes = []string{
	PatchTypeJSON,
	PatchTypeMergePatch,
	PatchTypeJSONPatch,
}

// AcceptPatchConfig configures the Accept-Patch middleware.
type AcceptPatchConfig struct {
	// AcceptPatchTypes is the list of Content-Type values advertised in
	// the Accept-Patch response header for OPTIONS requests. When nil,
	// defaults to application/json, application/merge-patch+json,
	// and application/json-patch+json.
	AcceptPatchTypes []string

	// StatusCode is the HTTP status code for OPTIONS responses.
	// Defaults to 204 No Content.
	StatusCode int
}

// AcceptPatchMiddleware returns a middleware that handles OPTIONS requests
// by responding with Allow and Accept-Patch headers per RFC 5789. The Allow
// header is auto-discovered from the router's registered methods for the
// matched path. Non-OPTIONS requests pass through unchanged.
//
// Because the router middleware only runs for matched routes, this function
// also sets the router's MethodNotAllowedHandler to intercept OPTIONS
// requests that would otherwise receive a 405 response.
//
// Spec reference: https://www.rfc-editor.org/rfc/rfc5789#section-3.1
func AcceptPatchMiddleware(router *mux.Router, cfg AcceptPatchConfig) mux.MiddlewareFunc {
	types := cfg.AcceptPatchTypes
	if types == nil {
		types = defaultAcceptPatchTypes
	}

	acceptPatch := strings.Join(types, ", ")

	statusCode := cfg.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusNoContent
	}

	prevHandler := router.MethodNotAllowedHandler

	router.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			if methods, err := getAllMethodsForRoute(router, r); err == nil {
				w.Header().Set("Allow", strings.Join(methods, ", "))
			}

			w.Header().Set("Accept-Patch", acceptPatch)
			w.WriteHeader(statusCode)

			return
		}

		if prevHandler != nil {
			prevHandler.ServeHTTP(w, r)
			return
		}

		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				if methods, err := getAllMethodsForRoute(router, r); err == nil {
					w.Header().Set("Allow", strings.Join(methods, ", "))
				}

				w.Header().Set("Accept-Patch", acceptPatch)
				w.WriteHeader(statusCode)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
