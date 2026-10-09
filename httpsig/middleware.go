package httpsig

import (
	"bytes"
	"io"
	"net/http"

	"github.com/vitalvas/kasper/mux"
)

// MiddlewareConfig configures the server-side signature verification
// middleware.
type MiddlewareConfig struct {
	// Verify configures how signatures are verified.
	Verify VerifyConfig

	// OnError is called when verification fails. When nil, a plain 401
	// Unauthorized response is sent.
	OnError func(w http.ResponseWriter, r *http.Request, err error)
}

// Middleware returns a mux.MiddlewareFunc that verifies HTTP message
// signatures on incoming requests per RFC 9421.
//
// It returns ErrNoResolver if VerifyConfig.Resolver is nil.
func Middleware(cfg MiddlewareConfig) (mux.MiddlewareFunc, error) {
	if cfg.Verify.Resolver == nil {
		return nil, ErrNoResolver
	}

	onError := cfg.OnError
	if onError == nil {
		onError = defaultOnError
	}

	verifyCfg := cfg.Verify

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := VerifyRequest(r, verifyCfg); err != nil {
				onError(w, r, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// defaultOnError writes a 401 Unauthorized response with no body.
func defaultOnError(w http.ResponseWriter, _ *http.Request, _ error) {
	w.WriteHeader(http.StatusUnauthorized)
}

// SignMiddleware returns a mux.MiddlewareFunc that signs outgoing responses
// per RFC 9421 via SignResponse. The handler's response is buffered so the
// signature (and the optional Content-Digest) covers the final status code,
// headers, and complete body.
//
// Covered components may carry the ";req" parameter (for example
// "@authority;req" or "@path;req") to bind the response signature to the
// incoming request. When signing fails, a plain 500 Internal Server Error
// response is sent instead of the handler's response.
//
// It returns ErrNoSigner if SignConfig.Signer is nil.
func SignMiddleware(cfg SignConfig) (mux.MiddlewareFunc, error) {
	if cfg.Signer == nil {
		return nil, ErrNoSigner
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &responseRecorder{header: w.Header(), writer: w}
			next.ServeHTTP(rec, r)

			if rec.status == 0 {
				rec.status = http.StatusOK
			}

			resp := &http.Response{
				StatusCode: rec.status,
				Header:     rec.header,
				Body:       io.NopCloser(bytes.NewReader(rec.body.Bytes())),
			}

			if err := SignResponse(resp, r, cfg); err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			w.WriteHeader(rec.status)
			if rec.body.Len() > 0 {
				_, _ = w.Write(rec.body.Bytes())
			}
		})
	}, nil
}

// responseRecorder buffers a handler's response so it can be signed before
// anything is written to the client. It shares the real ResponseWriter's
// header map, so headers set by the handler and by SignResponse land on the
// outgoing response. 1xx informational responses pass through unbuffered.
type responseRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
	writer http.ResponseWriter
}

func (r *responseRecorder) Header() http.Header { return r.header }

func (r *responseRecorder) WriteHeader(code int) {
	if code >= 100 && code < 200 {
		r.writer.WriteHeader(code)
		return
	}

	if r.status == 0 {
		r.status = code
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}

	return r.body.Write(b)
}
