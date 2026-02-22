package muxhandlers

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// ErrNoAllowedTypes is returned when ContentTypeCheckConfig.AllowedTypes is
// empty.
var ErrNoAllowedTypes = errors.New("content type check: at least one allowed content type is required")

// ContentTypeCheckConfig configures the Content-Type Check middleware behaviour.
type ContentTypeCheckConfig struct {
	// AllowedTypes is the set of acceptable Content-Type values.
	// Matching is case-insensitive and ignores parameters
	// (e.g. "application/json" matches "application/json; charset=utf-8").
	// Required; at least one must be provided.
	AllowedTypes []string

	// Methods is the set of HTTP methods that require Content-Type
	// validation. When nil, defaults to POST, PUT, PATCH.
	Methods []string
}

// defaultCheckedMethods is the set of HTTP methods that require Content-Type
// validation when Methods is nil.
var defaultCheckedMethods = []string{
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
}

// ContentTypeCheckMiddleware returns a middleware that validates the
// Content-Type header on requests with matching methods. It returns 415
// Unsupported Media Type when the Content-Type is missing or does not match
// any of the allowed types.
//
// It returns ErrNoAllowedTypes if AllowedTypes is empty.
func ContentTypeCheckMiddleware(cfg ContentTypeCheckConfig) (mux.MiddlewareFunc, error) {
	if len(cfg.AllowedTypes) == 0 {
		return nil, ErrNoAllowedTypes
	}

	methods := cfg.Methods
	if methods == nil {
		methods = defaultCheckedMethods
	}

	methodSet := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		methodSet[m] = struct{}{}
	}

	allowedSet := make(map[string]struct{}, len(cfg.AllowedTypes))
	for _, t := range cfg.AllowedTypes {
		allowedSet[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, check := methodSet[r.Method]; check {
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

				if _, ok := allowedSet[strings.ToLower(mediaType)]; !ok {
					http.Error(w, http.StatusText(http.StatusUnsupportedMediaType), http.StatusUnsupportedMediaType)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}
