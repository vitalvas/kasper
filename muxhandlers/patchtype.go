package muxhandlers

import (
	"context"
	"mime"
	"net/http"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// Patch content type constants for the supported PATCH formats.
const (
	// PatchTypeJSON is the implicit merge patch using standard JSON.
	PatchTypeJSON = "application/json"

	// PatchTypeMergePatch is the JSON Merge Patch format per RFC 7396.
	PatchTypeMergePatch = "application/merge-patch+json"

	// PatchTypeJSONPatch is the JSON Patch format per RFC 6902.
	PatchTypeJSONPatch = "application/json-patch+json"
)

// patchTypeCtxKey is the context key for the resolved patch content type.
type patchTypeCtxKey struct{}

// PatchContentType returns the patch content type stored in the request
// context by PatchRoutingMiddleware. Returns an empty string for non-PATCH
// requests or when the middleware is not applied.
func PatchContentType(r *http.Request) string {
	if v, ok := r.Context().Value(patchTypeCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// defaultPatchTypes is the set of accepted Content-Type values for PATCH
// requests when AllowedTypes is nil.
var defaultPatchTypes = []string{
	PatchTypeJSON,
	PatchTypeMergePatch,
	PatchTypeJSONPatch,
}

// PatchRoutingConfig configures the Patch Routing middleware.
type PatchRoutingConfig struct {
	// AllowedTypes is the set of accepted Content-Type values for PATCH
	// requests. Matching is case-insensitive and ignores parameters.
	// When nil, defaults to application/json, application/merge-patch+json,
	// and application/json-patch+json.
	AllowedTypes []string
}

// PatchRoutingMiddleware returns a middleware that validates the Content-Type
// of PATCH requests against a set of allowed patch formats and stores the
// resolved type in the request context. Non-PATCH requests pass through
// unchanged.
//
// The resolved type is retrievable via PatchContentType.
//
// Returns 415 Unsupported Media Type when the Content-Type is missing or
// does not match any allowed type.
//
// Spec references:
//   - https://www.rfc-editor.org/rfc/rfc7396 (JSON Merge Patch)
//   - https://www.rfc-editor.org/rfc/rfc6902 (JSON Patch)
func PatchRoutingMiddleware(cfg PatchRoutingConfig) mux.MiddlewareFunc {
	types := cfg.AllowedTypes
	if types == nil {
		types = defaultPatchTypes
	}

	allowedSet := make(map[string]struct{}, len(types))
	for _, t := range types {
		allowedSet[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			ct := r.Header.Get("Content-Type")
			if ct == "" {
				http.Error(w, http.StatusText(http.StatusUnsupportedMediaType), http.StatusUnsupportedMediaType)
				return
			}

			mediaType, _, err := mime.ParseMediaType(ct)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusUnsupportedMediaType), http.StatusUnsupportedMediaType)
				return
			}

			mediaType = strings.ToLower(mediaType)
			if _, ok := allowedSet[mediaType]; !ok {
				http.Error(w, http.StatusText(http.StatusUnsupportedMediaType), http.StatusUnsupportedMediaType)
				return
			}

			ctx := context.WithValue(r.Context(), patchTypeCtxKey{}, mediaType)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
