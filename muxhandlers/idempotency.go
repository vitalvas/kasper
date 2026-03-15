package muxhandlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// ErrNoIdempotencyStore is returned when IdempotencyConfig.Store is nil.
var ErrNoIdempotencyStore = errors.New("idempotency: Store must be set")

// IdempotencyStore is the interface for storing and retrieving cached
// responses keyed by idempotency key. Implementations must be safe for
// concurrent use.
type IdempotencyStore interface {
	// Get retrieves a cached response by key. Returns the serialized
	// response and true if found, or nil and false if not cached.
	Get(ctx context.Context, key string) ([]byte, bool)

	// Set stores a serialized response with the given key and TTL.
	// A zero TTL means the entry does not expire.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration)
}

// IdempotencyConfig configures the Idempotency middleware.
//
// Spec reference: https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/
type IdempotencyConfig struct {
	// Store is the backing store for cached responses. Required.
	Store IdempotencyStore

	// HeaderName overrides the header used to carry the idempotency key.
	// Defaults to "Idempotency-Key".
	HeaderName string

	// TTL is the time-to-live for cached responses. Defaults to 24 hours.
	// A zero value means entries do not expire.
	TTL time.Duration

	// Methods is the set of HTTP methods that require an idempotency key.
	// When nil, defaults to POST.
	Methods []string

	// EnforceKey, when true, returns 400 Bad Request if the idempotency
	// key header is missing on a matched method. When false (default),
	// requests without the header are passed through without caching.
	EnforceKey bool

	// CacheableStatusCodes is an optional allow list of HTTP status codes
	// that should be cached. When nil, all status codes are cached.
	// When set, only responses with a status code in this list are stored;
	// other responses (e.g. 500) are passed through without caching.
	CacheableStatusCodes []int
}

// idempotencyResponse is the serialized format for cached responses.
type idempotencyResponse struct {
	StatusCode int         `json:"status_code"`
	Header     http.Header `json:"header"`
	Body       []byte      `json:"body"`
}

// IdempotencyMiddleware returns a middleware that caches responses keyed by the
// Idempotency-Key header per draft-ietf-httpapi-idempotency-key-header. When a
// request carries a key that has been seen before, the cached response is
// replayed without invoking the downstream handler. The cached response
// includes the Idempotency-Key header in the replay.
//
// It returns ErrNoIdempotencyStore if Store is nil.
func IdempotencyMiddleware(cfg IdempotencyConfig) (mux.MiddlewareFunc, error) {
	if cfg.Store == nil {
		return nil, ErrNoIdempotencyStore
	}

	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = "Idempotency-Key"
	}

	ttl := cfg.TTL
	if ttl == 0 && cfg.TTL == 0 {
		ttl = 24 * time.Hour
	}

	methods := cfg.Methods
	if methods == nil {
		methods = []string{http.MethodPost}
	}

	methodSet := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		methodSet[m] = struct{}{}
	}

	var cacheableSet map[int]struct{}
	if len(cfg.CacheableStatusCodes) > 0 {
		cacheableSet = make(map[int]struct{}, len(cfg.CacheableStatusCodes))
		for _, code := range cfg.CacheableStatusCodes {
			cacheableSet[code] = struct{}{}
		}
	}

	store := cfg.Store
	enforceKey := cfg.EnforceKey

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := methodSet[r.Method]; !ok {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get(headerName)
			if key == "" {
				if enforceKey {
					http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if cached, ok := store.Get(r.Context(), key); ok {
				replayResponse(w, cached, headerName, key)
				return
			}

			rec := &responseRecorder{
				header: make(http.Header),
				body:   &bytes.Buffer{},
			}

			next.ServeHTTP(rec, r)

			cacheable := cacheableSet == nil
			if !cacheable {
				_, cacheable = cacheableSet[rec.statusCode]
			}

			if cacheable {
				resp := idempotencyResponse{
					StatusCode: rec.statusCode,
					Header:     rec.header,
					Body:       rec.body.Bytes(),
				}

				if data, err := json.Marshal(resp); err == nil {
					store.Set(r.Context(), key, data, ttl)
				}
			}

			writeRecordedResponse(w, rec, headerName, key)
		})
	}, nil
}

// replayResponse writes a cached response to the client.
func replayResponse(w http.ResponseWriter, data []byte, headerName, key string) {
	var resp idempotencyResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	h := w.Header()
	for k, vals := range resp.Header {
		for _, v := range vals {
			h.Add(k, v)
		}
	}
	h.Set(headerName, key)

	w.WriteHeader(resp.StatusCode)
	w.Write(resp.Body) //nolint:errcheck
}

// writeRecordedResponse writes the recorded response to the actual client.
func writeRecordedResponse(w http.ResponseWriter, rec *responseRecorder, headerName, key string) {
	h := w.Header()
	for k, vals := range rec.header {
		for _, v := range vals {
			h.Add(k, v)
		}
	}
	h.Set(headerName, key)

	if rec.statusCode == 0 {
		rec.statusCode = http.StatusOK
	}

	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes()) //nolint:errcheck
}

// responseRecorder captures the response from a handler for caching.
type responseRecorder struct {
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.statusCode == 0 {
		r.statusCode = code
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	return r.body.Write(b)
}
