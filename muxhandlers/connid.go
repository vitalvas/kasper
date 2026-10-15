package muxhandlers

import (
	"context"
	"net"
	"net/http"
	"uuid"

	"go.vitalvas.com/kasper/mux"
)

type connIDKey struct{}

// ConnIDFromContext returns the connection ID stored in the context by
// ConnContext. Returns an empty string if no ID is present.
func ConnIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(connIDKey{}).(string); ok {
		return id
	}

	return ""
}

// ConnIDConfig configures connection ID tracking.
type ConnIDConfig struct {
	// HeaderName overrides the response header used to surface the
	// connection ID. Defaults to "X-Connection-ID" when empty.
	HeaderName string

	// ExposeHeader, when true, writes the connection ID to the response
	// header. When false (the default) the ID is still propagated via the
	// request context (ConnIDFromContext) and the access log, but is not
	// leaked to the client.
	ExposeHeader bool

	// GenerateFunc is an optional callback returning a new unique ID for a
	// connection. Defaults to a UUID v4. Unlike the request ID, it is
	// invoked once per TCP connection, not once per request.
	GenerateFunc func() string
}

// ConnContext returns a function suitable for http.Server.ConnContext. It
// stamps every accepted connection with a unique ID stored in the connection's
// context, so that all requests served over the same keep-alive connection
// share one ID. Pair it with ConnIDMiddleware to surface the ID on responses.
//
//	cfg := muxhandlers.ConnIDConfig{}
//	srv := &http.Server{
//	    Handler:     r,
//	    ConnContext: muxhandlers.ConnContext(cfg),
//	}
//	r.Use(muxhandlers.ConnIDMiddleware(cfg))
func ConnContext(cfg ConnIDConfig) func(ctx context.Context, _ net.Conn) context.Context {
	generate := cfg.GenerateFunc
	if generate == nil {
		generate = func() string { return uuid.New().String() }
	}

	return func(ctx context.Context, _ net.Conn) context.Context {
		return context.WithValue(ctx, connIDKey{}, generate())
	}
}

// ConnIDMiddleware surfaces the connection ID stamped by ConnContext. The ID
// already lives in the request context (propagated from the connection), so
// downstream handlers can always read it with ConnIDFromContext; set
// ExposeHeader to also write it to the response header. When no connection ID
// is present (e.g. the server was configured without ConnContext), the
// middleware is a no-op.
func ConnIDMiddleware(cfg ConnIDConfig) mux.MiddlewareFunc {
	if !cfg.ExposeHeader {
		// Nothing to write; the ID already rides the request context from
		// ConnContext. Skip wrapping entirely.
		return func(next http.Handler) http.Handler { return next }
	}

	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = "X-Connection-ID"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id := ConnIDFromContext(r.Context()); id != "" {
				w.Header().Set(headerName, id)
			}

			next.ServeHTTP(w, r)
		})
	}
}
