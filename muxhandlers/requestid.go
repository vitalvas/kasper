package muxhandlers

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/vitalvas/kasper/mux"
)

type requestIDKey struct{}

// RequestIDFromContext returns the request ID stored in the context by
// RequestIDMiddleware. Returns an empty string if no ID is present.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}

	return ""
}

// RequestIDConfig configures the Request ID middleware behaviour.
type RequestIDConfig struct {
	// HeaderName overrides the header used to propagate the request ID.
	// Defaults to "X-Request-ID" when empty.
	HeaderName string

	// GenerateFunc is an optional callback that returns a new unique ID.
	// It receives the current request, allowing ID generation based on
	// request context. Defaults to GenerateUUIDv4.
	GenerateFunc func(r *http.Request) string

	// TrustIncoming, when true, reuses an existing request ID from the
	// incoming request header instead of generating a new one.
	TrustIncoming bool
}

// RequestIDMiddleware returns a middleware that generates or propagates a
// request ID header. The ID is set on both the request (for downstream
// handlers) and the response (for the caller).
func RequestIDMiddleware(cfg RequestIDConfig) mux.MiddlewareFunc {
	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = "X-Request-ID"
	}

	generate := cfg.GenerateFunc
	if generate == nil {
		generate = GenerateUUIDv4
	}

	trustIncoming := cfg.TrustIncoming

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ""
			if trustIncoming {
				id = r.Header.Get(headerName)
			}

			if id == "" {
				id = generate(r)
			}

			if id != "" {
				r.Header.Set(headerName, id)
				w.Header().Set(headerName, id)
				r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GenerateUUIDv4 returns a new UUID v4 string.
//
// Spec reference: https://www.rfc-editor.org/rfc/rfc9562#section-5.4
func GenerateUUIDv4(_ *http.Request) string {
	return uuid.New().String()
}

// GenerateUUIDv7 returns a new UUID v7 string. UUIDs are time-ordered:
// IDs generated later sort lexicographically after earlier ones.
//
// Spec reference: https://www.rfc-editor.org/rfc/rfc9562#section-5.7
func GenerateUUIDv7(_ *http.Request) string {
	return uuid.Must(uuid.NewV7()).String()
}
