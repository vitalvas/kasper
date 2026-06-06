package e2ee

import (
	"bytes"
	"net/http"
	"strconv"

	"github.com/vitalvas/kasper/mux"
)

// MiddlewareConfig configures the server-side E2EE middleware.
type MiddlewareConfig struct {
	// Server configures request decryption and response encryption. KeySet is
	// required.
	Server ServerConfig

	// OnError is called when request decryption or validation fails. When nil,
	// an RFC 9457 Problem Details response is written via WriteProblem.
	OnError func(w http.ResponseWriter, r *http.Request, err error)
}

// Middleware returns a mux.MiddlewareFunc that decrypts incoming E2EE requests
// and encrypts handler responses per draft-vasylenko-e2ee-http. Only the body
// is protected; HTTP semantics (method, target, status, headers) are not bound
// into the AEAD (Section 7.5), so handlers and intermediaries see normal HTTP
// metadata.
//
// It returns ErrNoKeySet when Server.KeySet is nil. When Server.Replay is nil,
// a shared in-memory replay cache is installed so replay protection is active
// (Section 11.5).
func Middleware(cfg MiddlewareConfig) (mux.MiddlewareFunc, error) {
	if cfg.Server.KeySet == nil {
		return nil, ErrNoKeySet
	}

	if cfg.Server.Replay == nil {
		cfg.Server.Replay = NewMemoryReplayCache()
	}

	onError := cfg.OnError
	if onError == nil {
		onError = func(w http.ResponseWriter, _ *http.Request, err error) {
			WriteProblem(w, err)
		}
	}

	server := cfg.Server

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			enc, err := DecryptRequest(r, server)
			if err != nil {
				onError(w, r, err)

				return
			}

			buf := &bufferingWriter{header: make(http.Header), status: http.StatusOK}

			next.ServeHTTP(buf, r)

			resField, body, err := enc.Encrypt(buf.body.Bytes())
			if err != nil {
				onError(w, r, err)

				return
			}

			// Propagate handler-set headers except the body content type,
			// which is replaced by the E2EE media type.
			copyHeaders(w.Header(), buf.header)
			w.Header().Set(SessionHeader, resField)
			w.Header().Set("Content-Type", MediaType)
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(buf.status)

			_, _ = w.Write(body)
		})
	}, nil
}

// bufferingWriter captures a handler's response so it can be encrypted before
// being written to the real ResponseWriter.
type bufferingWriter struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func (b *bufferingWriter) Header() http.Header { return b.header }

func (b *bufferingWriter) WriteHeader(status int) {
	if b.wroteHeader {
		return
	}

	b.status = status
	b.wroteHeader = true
}

func (b *bufferingWriter) Write(p []byte) (int, error) {
	if !b.wroteHeader {
		b.WriteHeader(http.StatusOK)
	}

	return b.body.Write(p)
}

// copyHeaders copies all headers from src into dst, replacing existing values.
// The Content-Type and Content-Length are managed by the middleware and are
// not copied from the handler.
func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		if http.CanonicalHeaderKey(k) == "Content-Type" || http.CanonicalHeaderKey(k) == "Content-Length" {
			continue
		}

		dst[http.CanonicalHeaderKey(k)] = append([]string(nil), vv...)
	}
}
